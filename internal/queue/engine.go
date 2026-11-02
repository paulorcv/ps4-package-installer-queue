// Package queue schedules package installs on the PS4, a limited number at a
// time, and decides when each download has finished from what the NAS
// monitor observes.
package queue

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/paulorcv/ps4-package-installer-queue/internal/monitor"
)

// State of a queue item.
type State int

const (
	Pending     State = iota // waiting for a free slot
	Sending                  // install request being sent to ezRemote
	Downloading              // PS4 accepted it; waiting for the download to end
	Done
	Failed
)

func (s State) String() string {
	switch s {
	case Pending:
		return "pendente"
	case Sending:
		return "enviando"
	case Downloading:
		return "baixando"
	case Done:
		return "concluído"
	case Failed:
		return "falhou"
	}
	return "?"
}

// Active reports whether the item occupies a download slot.
func (s State) Active() bool { return s == Sending || s == Downloading }

// Finished reports whether the item reached a terminal state.
func (s State) Finished() bool { return s == Done || s == Failed }

// Item is one package in the queue. Snapshot returns copies.
type Item struct {
	ID    int
	Name  string
	URL   string
	Path  string // decoded URL path, matched against access log lines
	Size  int64  // bytes, or <0 if unknown
	State State

	Served int64 // bytes the NAS reported as sent to the PS4
	Conns  int   // PS4 connections open at the last poll
	Err    string

	AddedAt      time.Time
	SentAt       time.Time
	LastActivity time.Time
	FinishedAt   time.Time
}

// NewItem describes a package to enqueue.
type NewItem struct {
	Name string
	URL  string
	Size int64
}

// Installer starts an install on the PS4.
type Installer interface {
	InstallURL(ctx context.Context, pkgURL string) error
}

// Activity reports the PS4's live connections to the NAS.
type Activity interface {
	Connections(ctx context.Context) (int, error)
}

// Options configure the engine.
type Options struct {
	PS4Host       string
	MaxConcurrent int
	StartTimeout  time.Duration // no sign of the PS4 after sending => failed
	IdleDone      time.Duration // quiet period after the whole file was served
	StallTimeout  time.Duration // quiet period without finishing => failed
	PollInterval  time.Duration
	SendTimeout   time.Duration
	// RetryMin and RetryMax bound the backoff used while the installer is
	// unreachable. The queue pauses instead of failing items.
	RetryMin time.Duration
	RetryMax time.Duration
	// IsTransient reports whether a send error means "try again later"
	// (e.g. the PS4 or ezRemote is not answering). Defaults to network
	// errors and timeouts.
	IsTransient func(error) bool
	Now         func() time.Time
}

// Status describes the engine as a whole.
type Status struct {
	// RetryAt is when sending resumes after the installer was unreachable;
	// zero when not paused.
	RetryAt time.Time
	LastErr string
}

// Engine owns the queue. All methods are safe for concurrent use.
type Engine struct {
	opts Options
	inst Installer
	act  Activity

	mu     sync.Mutex
	items  []*Item
	nextID int
	monErr string // last monitor error, to avoid repeating it

	retryAt time.Time     // sending is paused until then
	backoff time.Duration // next pause length
	sendErr string        // last transient send error

	changed chan struct{}
	wake    chan struct{}
	logs    chan string
}

// New creates an engine. Call Run to start processing.
func New(opts Options, inst Installer, act Activity) *Engine {
	if opts.MaxConcurrent < 1 {
		opts.MaxConcurrent = 1
	}
	if opts.SendTimeout <= 0 {
		opts.SendTimeout = 60 * time.Second
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.RetryMin <= 0 {
		opts.RetryMin = 15 * time.Second
	}
	if opts.RetryMax < opts.RetryMin {
		opts.RetryMax = 2 * time.Minute
	}
	if opts.IsTransient == nil {
		opts.IsTransient = IsNetworkError
	}
	return &Engine{
		opts:    opts,
		inst:    inst,
		act:     act,
		nextID:  1,
		backoff: opts.RetryMin,
		changed: make(chan struct{}, 1),
		wake:    make(chan struct{}, 1),
		logs:    make(chan string, 256),
	}
}

// Changes signals (coalesced) whenever the queue changes.
func (e *Engine) Changes() <-chan struct{} { return e.changed }

// Logs delivers human readable messages about what the engine did.
func (e *Engine) Logs() <-chan string { return e.logs }

// Add appends packages to the end of the queue. Packages already pending or
// in progress are skipped. It returns how many were added.
func (e *Engine) Add(items ...NewItem) int {
	e.mu.Lock()
	added := 0
	var skipped []string
	for _, it := range items {
		if e.inProgressLocked(it.URL) {
			skipped = append(skipped, it.Name)
			continue
		}
		e.items = append(e.items, &Item{
			ID:      e.nextID,
			Name:    it.Name,
			URL:     it.URL,
			Path:    urlPath(it.URL),
			Size:    it.Size,
			State:   Pending,
			AddedAt: e.opts.Now(),
		})
		e.nextID++
		added++
	}
	e.mu.Unlock()

	for _, name := range skipped {
		e.log("já está na fila: %s", name)
	}
	if added > 0 {
		e.log("%d pacote(s) adicionado(s) à fila", added)
		e.notify()
		e.kick()
	}
	return added
}

// Remove drops an item that is not in progress.
func (e *Engine) Remove(id int) bool {
	e.mu.Lock()
	i := e.indexLocked(id)
	if i < 0 || e.items[i].State.Active() {
		e.mu.Unlock()
		return false
	}
	e.items = append(e.items[:i], e.items[i+1:]...)
	e.mu.Unlock()
	e.notify()
	return true
}

// Retry puts a failed item back to pending.
func (e *Engine) Retry(id int) bool {
	e.mu.Lock()
	it := e.findLocked(id)
	if it == nil || it.State != Failed {
		e.mu.Unlock()
		return false
	}
	resetForRetry(it)
	name := it.Name
	e.mu.Unlock()
	e.log("nova tentativa: %s", name)
	e.notify()
	e.kick()
	return true
}

// MarkDone finishes an item by hand, e.g. when the PS4 shows it installed but
// the NAS log did not account for every byte.
func (e *Engine) MarkDone(id int) bool {
	e.mu.Lock()
	it := e.findLocked(id)
	if it == nil || it.State == Done || it.State == Pending || it.State == Sending {
		e.mu.Unlock()
		return false
	}
	it.State = Done
	it.Err = ""
	it.FinishedAt = e.opts.Now()
	name := it.Name
	e.mu.Unlock()
	e.log("marcado como concluído: %s", name)
	e.notify()
	e.kick()
	return true
}

// Move shifts a pending item up (delta<0) or down (delta>0) among the other
// pending items.
func (e *Engine) Move(id, delta int) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	i := e.indexLocked(id)
	if i < 0 || e.items[i].State != Pending {
		return false
	}
	j := i
	for steps := abs(delta); steps > 0; steps-- {
		k := j + sign(delta)
		for k >= 0 && k < len(e.items) && e.items[k].State != Pending {
			k += sign(delta)
		}
		if k < 0 || k >= len(e.items) {
			break
		}
		e.items[j], e.items[k] = e.items[k], e.items[j]
		j = k
	}
	if j == i {
		return false
	}
	e.notifyLocked()
	return true
}

// ClearFinished removes every item that is done (failed items are kept so
// they can be retried).
func (e *Engine) ClearFinished() int {
	e.mu.Lock()
	kept := e.items[:0]
	removed := 0
	for _, it := range e.items {
		if it.State == Done {
			removed++
			continue
		}
		kept = append(kept, it)
	}
	e.items = kept
	e.mu.Unlock()
	if removed > 0 {
		e.notify()
	}
	return removed
}

// Status reports whether sending is paused because the installer was
// unreachable.
func (e *Engine) Status() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.opts.Now().Before(e.retryAt) {
		return Status{}
	}
	return Status{RetryAt: e.retryAt, LastErr: e.sendErr}
}

// Snapshot returns a copy of every item, in queue order.
func (e *Engine) Snapshot() []Item {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]Item, len(e.items))
	for i, it := range e.items {
		out[i] = *it
	}
	return out
}

// Run processes the queue until ctx is cancelled.
func (e *Engine) Run(ctx context.Context, events <-chan monitor.Event) {
	poll := e.opts.PollInterval
	if poll <= 0 {
		poll = 5 * time.Second
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()

	for {
		e.dispatch(ctx)
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			e.OnEvent(ev)
		case <-ticker.C:
			e.Poll(ctx)
		case <-e.wake:
		}
	}
}

// dispatch starts pending items while there are free slots.
func (e *Engine) dispatch(ctx context.Context) {
	for {
		e.mu.Lock()
		if e.opts.Now().Before(e.retryAt) || e.activeLocked() >= e.opts.MaxConcurrent {
			e.mu.Unlock()
			return
		}
		it := e.nextPendingLocked()
		if it == nil {
			e.mu.Unlock()
			return
		}
		it.State = Sending
		it.SentAt = e.opts.Now()
		id, name, u := it.ID, it.Name, it.URL
		e.notifyLocked()
		e.mu.Unlock()

		e.log("enviando para o PS4: %s", name)
		go e.send(ctx, id, name, u)
	}
}

func (e *Engine) send(ctx context.Context, id int, name, u string) {
	sctx, cancel := context.WithTimeout(ctx, e.opts.SendTimeout)
	err := e.inst.InstallURL(sctx, u)
	cancel()

	e.mu.Lock()
	it := e.findLocked(id)
	if it == nil || it.State != Sending {
		e.mu.Unlock()
		return
	}
	now := e.opts.Now()
	var msg string
	switch {
	case err == nil:
		it.State = Downloading
		it.SentAt = now
		e.retryAt, e.sendErr, e.backoff = time.Time{}, "", e.opts.RetryMin
		msg = fmt.Sprintf("PS4 aceitou: %s", name)
	case e.opts.IsTransient(err):
		// Keep the item at the head of the queue and pause sending, so a
		// short outage does not fail the whole queue.
		wait := e.backoff
		it.State = Pending
		it.SentAt = time.Time{}
		e.retryAt = now.Add(wait)
		e.sendErr = err.Error()
		e.backoff = min(e.backoff*2, e.opts.RetryMax)
		msg = fmt.Sprintf("ezRemote não respondeu (%v); nova tentativa em %s", err, wait)
	default:
		it.State = Failed
		it.Err = err.Error()
		it.FinishedAt = now
		msg = fmt.Sprintf("falha ao enviar %s: %v", name, err)
	}
	e.notifyLocked()
	e.mu.Unlock()

	e.log("%s", msg)
	e.kick()
}

// IsNetworkError reports whether err is a connection problem or timeout, as
// opposed to an answer from the installer.
func IsNetworkError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne)
}

// OnEvent accounts for a request the NAS served.
func (e *Engine) OnEvent(ev monitor.Event) {
	if ev.Host != e.opts.PS4Host || ev.Method != "GET" {
		return
	}
	if ev.Status != 200 && ev.Status != 206 {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, it := range e.items {
		if it.State.Active() && it.Path == ev.Path {
			it.Served += ev.Bytes
			it.LastActivity = e.opts.Now()
			e.notifyLocked()
			return
		}
	}
}

// Poll checks live connections and finishes or fails downloads.
func (e *Engine) Poll(ctx context.Context) {
	e.mu.Lock()
	downloading := false
	for _, it := range e.items {
		if it.State == Downloading {
			downloading = true
			break
		}
	}
	e.mu.Unlock()
	if !downloading {
		return
	}

	pctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	conns, err := e.act.Connections(pctx)
	cancel()

	e.mu.Lock()
	if err != nil {
		// Without the monitor we cannot tell anything; keep waiting.
		msg := err.Error()
		repeat := msg == e.monErr
		e.monErr = msg
		e.mu.Unlock()
		if !repeat {
			e.log("monitor indisponível: %v", err)
		}
		return
	}
	e.monErr = ""

	now := e.opts.Now()
	var msgs []string
	finished := false
	for _, it := range e.items {
		if it.State != Downloading {
			continue
		}
		// Connections cannot be attributed to a single file, so with more
		// than one active download each sees the total.
		it.Conns = conns
		if conns > 0 {
			it.LastActivity = now
		}
		if it.LastActivity.IsZero() {
			if now.Sub(it.SentAt) >= e.opts.StartTimeout {
				fail(it, now, fmt.Sprintf("o PS4 não começou a baixar em %s", e.opts.StartTimeout))
				msgs = append(msgs, fmt.Sprintf("falhou: %s (%s)", it.Name, it.Err))
				finished = true
			}
			continue
		}
		idle := now.Sub(it.LastActivity)
		if conns == 0 && idle >= e.opts.IdleDone && complete(it) {
			it.State = Done
			it.FinishedAt = now
			msgs = append(msgs, fmt.Sprintf("concluído: %s", it.Name))
			finished = true
			continue
		}
		if conns == 0 && idle >= e.opts.StallTimeout {
			fail(it, now, fmt.Sprintf("parou com %s de %s", HumanBytes(it.Served), HumanBytes(it.Size)))
			msgs = append(msgs, fmt.Sprintf("falhou: %s (%s)", it.Name, it.Err))
			finished = true
		}
	}
	e.notifyLocked()
	e.mu.Unlock()

	for _, m := range msgs {
		e.log("%s", m)
	}
	if finished {
		e.kick()
	}
}

func complete(it *Item) bool {
	if it.Size < 0 {
		return it.Served > 0
	}
	return it.Served >= it.Size
}

func fail(it *Item, now time.Time, msg string) {
	it.State = Failed
	it.Err = msg
	it.FinishedAt = now
}

func resetForRetry(it *Item) {
	it.State = Pending
	it.Err = ""
	it.Served = 0
	it.Conns = 0
	it.SentAt = time.Time{}
	it.LastActivity = time.Time{}
	it.FinishedAt = time.Time{}
}

func (e *Engine) activeLocked() int {
	n := 0
	for _, it := range e.items {
		if it.State.Active() {
			n++
		}
	}
	return n
}

func (e *Engine) nextPendingLocked() *Item {
	for _, it := range e.items {
		if it.State == Pending {
			return it
		}
	}
	return nil
}

func (e *Engine) inProgressLocked(u string) bool {
	for _, it := range e.items {
		if it.URL == u && !it.State.Finished() {
			return true
		}
	}
	return false
}

func (e *Engine) indexLocked(id int) int {
	for i, it := range e.items {
		if it.ID == id {
			return i
		}
	}
	return -1
}

func (e *Engine) findLocked(id int) *Item {
	if i := e.indexLocked(id); i >= 0 {
		return e.items[i]
	}
	return nil
}

func (e *Engine) notify() {
	e.mu.Lock()
	e.notifyLocked()
	e.mu.Unlock()
}

func (e *Engine) notifyLocked() {
	select {
	case e.changed <- struct{}{}:
	default:
	}
}

func (e *Engine) kick() {
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

func (e *Engine) log(format string, args ...any) {
	msg := e.opts.Now().Format("15:04:05") + " " + fmt.Sprintf(format, args...)
	select {
	case e.logs <- msg:
	default:
	}
}

func urlPath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return u.Path // url.Parse already decodes percent-escapes in Path
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func sign(n int) int {
	if n < 0 {
		return -1
	}
	return 1
}

// HumanBytes formats n using binary units.
func HumanBytes(n int64) string {
	if n < 0 {
		return "?"
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
