package queue

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/paulorcv/ps4-package-installer-queue/internal/monitor"
)

const ps4 = "192.168.0.104"

type fakeInstaller struct {
	mu   sync.Mutex
	urls []string
	err  error
}

func (f *fakeInstaller) InstallURL(_ context.Context, u string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.urls = append(f.urls, u)
	return f.err
}

func (f *fakeInstaller) sent() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.urls...)
}

type fakeActivity struct {
	mu    sync.Mutex
	conns int
	err   error
}

func (f *fakeActivity) Connections(context.Context) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.conns, f.err
}

func (f *fakeActivity) set(n int) {
	f.mu.Lock()
	f.conns = n
	f.mu.Unlock()
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type harness struct {
	t    *testing.T
	eng  *Engine
	inst *fakeInstaller
	act  *fakeActivity
	clk  *clock
}

func newHarness(t *testing.T, max int) *harness {
	clk := &clock{t: time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)}
	inst := &fakeInstaller{}
	act := &fakeActivity{}
	eng := New(Options{
		PS4Host:       ps4,
		MaxConcurrent: max,
		StartTimeout:  3 * time.Minute,
		IdleDone:      time.Minute,
		StallTimeout:  15 * time.Minute,
		PollInterval:  time.Hour, // tests drive Poll by hand
		Now:           clk.Now,
	}, inst, act)
	return &harness{t: t, eng: eng, inst: inst, act: act, clk: clk}
}

// step dispatches and waits for in-flight sends to settle.
func (h *harness) step() {
	h.t.Helper()
	h.eng.dispatch(context.Background())
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		busy := false
		for _, it := range h.eng.Snapshot() {
			if it.State == Sending {
				busy = true
			}
		}
		if !busy {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	h.t.Fatal("send did not settle")
}

func (h *harness) states() []State {
	var s []State
	for _, it := range h.eng.Snapshot() {
		s = append(s, it.State)
	}
	return s
}

func (h *harness) expect(want ...State) {
	h.t.Helper()
	got := h.states()
	if len(got) != len(want) {
		h.t.Fatalf("states = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			h.t.Fatalf("states = %v, want %v", got, want)
		}
	}
}

func served(path string, n int64) monitor.Event {
	return monitor.Event{Host: ps4, Method: "GET", Path: path, Status: 206, Bytes: n}
}

func TestOneAtATimeAndAddWhileRunning(t *testing.T) {
	h := newHarness(t, 1)
	h.eng.Add(NewItem{Name: "a", URL: "http://nas/a.pkg", Size: 100})
	h.eng.Add(NewItem{Name: "b", URL: "http://nas/b.pkg", Size: 50})
	h.step()
	h.expect(Downloading, Pending)

	// A package added while a download runs just waits its turn.
	h.eng.Add(NewItem{Name: "c", URL: "http://nas/c%20d.pkg", Size: 10})
	h.step()
	h.expect(Downloading, Pending, Pending)
	if got := h.inst.sent(); len(got) != 1 || got[0] != "http://nas/a.pkg" {
		t.Fatalf("sent = %v", got)
	}

	// Download a: connection open, bytes logged.
	h.act.set(1)
	h.eng.OnEvent(served("/a.pkg", 100))
	h.eng.Poll(context.Background())
	h.expect(Downloading, Pending, Pending)

	// Connection closes; done only after the idle period.
	h.act.set(0)
	h.clk.Advance(30 * time.Second)
	h.eng.Poll(context.Background())
	h.expect(Downloading, Pending, Pending)
	h.clk.Advance(31 * time.Second)
	h.eng.Poll(context.Background())
	h.expect(Done, Pending, Pending)

	h.step()
	h.expect(Done, Downloading, Pending)

	// Percent-encoded URLs match the decoded paths in the log.
	h.eng.OnEvent(served("/b.pkg", 50))
	h.clk.Advance(2 * time.Minute)
	h.eng.Poll(context.Background())
	h.step()
	h.expect(Done, Done, Downloading)
	h.eng.OnEvent(served("/c d.pkg", 10))
	h.clk.Advance(2 * time.Minute)
	h.eng.Poll(context.Background())
	h.expect(Done, Done, Done)
}

func TestIgnoresOtherHostsAndMethods(t *testing.T) {
	h := newHarness(t, 1)
	h.eng.Add(NewItem{Name: "a", URL: "http://nas/a.pkg", Size: 100})
	h.step()
	h.eng.OnEvent(monitor.Event{Host: "192.168.0.103", Method: "GET", Path: "/a.pkg", Status: 200, Bytes: 100})
	h.eng.OnEvent(monitor.Event{Host: ps4, Method: "HEAD", Path: "/a.pkg", Status: 200, Bytes: 100})
	h.eng.OnEvent(monitor.Event{Host: ps4, Method: "GET", Path: "/a.pkg", Status: 404, Bytes: 100})
	if got := h.eng.Snapshot()[0].Served; got != 0 {
		t.Fatalf("served = %d, want 0", got)
	}
}

func TestStartTimeout(t *testing.T) {
	h := newHarness(t, 1)
	h.eng.Add(NewItem{Name: "a", URL: "http://nas/a.pkg", Size: 100}, NewItem{Name: "b", URL: "http://nas/b.pkg", Size: 1})
	h.step()
	h.clk.Advance(3 * time.Minute)
	h.eng.Poll(context.Background())
	h.expect(Failed, Pending)
	h.step()
	h.expect(Failed, Downloading)
}

func TestStallAndRetry(t *testing.T) {
	h := newHarness(t, 1)
	h.eng.Add(NewItem{Name: "a", URL: "http://nas/a.pkg", Size: 100})
	h.step()
	h.eng.OnEvent(served("/a.pkg", 40)) // partial
	h.clk.Advance(2 * time.Minute)
	h.eng.Poll(context.Background())
	h.expect(Downloading) // not complete, not stalled yet
	h.clk.Advance(14 * time.Minute)
	h.eng.Poll(context.Background())
	h.expect(Failed)

	id := h.eng.Snapshot()[0].ID
	if !h.eng.Retry(id) {
		t.Fatal("retry refused")
	}
	it := h.eng.Snapshot()[0]
	if it.State != Pending || it.Served != 0 || it.Err != "" {
		t.Fatalf("after retry: %+v", it)
	}
}

func TestSendFailure(t *testing.T) {
	h := newHarness(t, 1)
	h.inst.err = errors.New("offline")
	h.eng.Add(NewItem{Name: "a", URL: "http://nas/a.pkg", Size: 1}, NewItem{Name: "b", URL: "http://nas/b.pkg", Size: 1})
	h.step()
	h.step()
	h.expect(Failed, Failed)
}

// timeoutErr behaves like the error net/http returns when ezRemote does not
// answer in time.
type timeoutErr struct{}

func (timeoutErr) Error() string   { return "Client.Timeout exceeded" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestUnreachableInstallerPausesInsteadOfFailing(t *testing.T) {
	h := newHarness(t, 1)
	h.inst.err = timeoutErr{}
	h.eng.Add(
		NewItem{Name: "a", URL: "http://nas/a.pkg", Size: 1},
		NewItem{Name: "b", URL: "http://nas/b.pkg", Size: 1},
		NewItem{Name: "c", URL: "http://nas/c.pkg", Size: 1},
	)
	h.step()
	// Nothing failed, and the queue did not race through b and c.
	h.expect(Pending, Pending, Pending)
	if got := len(h.inst.sent()); got != 1 {
		t.Fatalf("sent %d requests, want 1", got)
	}
	st := h.eng.Status()
	if st.RetryAt.IsZero() || st.LastErr == "" {
		t.Fatalf("status = %+v, want paused", st)
	}

	// Still paused before the backoff ends.
	h.clk.Advance(10 * time.Second)
	h.step()
	if got := len(h.inst.sent()); got != 1 {
		t.Fatalf("sent while paused: %d", got)
	}

	// Second failure doubles the wait (15s -> 30s).
	h.clk.Advance(5 * time.Second)
	h.step()
	if got := len(h.inst.sent()); got != 2 {
		t.Fatalf("sent %d, want 2 after backoff", got)
	}
	if wait := h.eng.Status().RetryAt.Sub(h.clk.Now()); wait != 30*time.Second {
		t.Fatalf("second backoff = %s, want 30s", wait)
	}

	// ezRemote is back: the first item goes out and the pause clears.
	h.inst.mu.Lock()
	h.inst.err = nil
	h.inst.mu.Unlock()
	h.clk.Advance(30 * time.Second)
	h.step()
	h.expect(Downloading, Pending, Pending)
	if st := h.eng.Status(); !st.RetryAt.IsZero() {
		t.Fatalf("still paused after success: %+v", st)
	}
	if sent := h.inst.sent(); sent[len(sent)-1] != "http://nas/a.pkg" {
		t.Fatalf("order changed: %v", sent)
	}
}

func TestBackoffIsCapped(t *testing.T) {
	h := newHarness(t, 1)
	h.eng.opts.RetryMax = 40 * time.Second
	h.inst.err = timeoutErr{}
	h.eng.Add(NewItem{Name: "a", URL: "http://nas/a.pkg", Size: 1})
	for i := 0; i < 5; i++ {
		h.step()
		wait := h.eng.Status().RetryAt.Sub(h.clk.Now())
		if wait > 40*time.Second {
			t.Fatalf("backoff %s exceeds cap", wait)
		}
		h.clk.Advance(wait)
	}
}

func TestIsNetworkError(t *testing.T) {
	if !IsNetworkError(timeoutErr{}) || !IsNetworkError(context.DeadlineExceeded) {
		t.Error("timeouts must be transient")
	}
	if IsNetworkError(errors.New("ezRemote recusou a instalação: Invalid URL")) {
		t.Error("an answer from ezRemote is not a network error")
	}
}

func TestMonitorErrorKeepsWaiting(t *testing.T) {
	h := newHarness(t, 1)
	h.eng.Add(NewItem{Name: "a", URL: "http://nas/a.pkg", Size: 1})
	h.step()
	h.act.err = errors.New("ssh down")
	h.clk.Advance(time.Hour)
	h.eng.Poll(context.Background())
	h.expect(Downloading)
}

func TestConcurrencyLimit(t *testing.T) {
	h := newHarness(t, 2)
	h.eng.Add(
		NewItem{Name: "a", URL: "http://nas/a.pkg"},
		NewItem{Name: "b", URL: "http://nas/b.pkg"},
		NewItem{Name: "c", URL: "http://nas/c.pkg"},
	)
	h.step()
	h.expect(Downloading, Downloading, Pending)
}

func TestDuplicatesAndQueueEditing(t *testing.T) {
	h := newHarness(t, 1)
	h.eng.Add(NewItem{Name: "a", URL: "http://nas/a.pkg"})
	if n := h.eng.Add(NewItem{Name: "a", URL: "http://nas/a.pkg"}); n != 0 {
		t.Fatalf("duplicate added")
	}
	h.eng.Add(NewItem{Name: "b", URL: "http://nas/b.pkg"}, NewItem{Name: "c", URL: "http://nas/c.pkg"})
	h.step() // a downloading

	snap := h.eng.Snapshot()
	if h.eng.Remove(snap[0].ID) {
		t.Fatal("removed an active item")
	}
	// Move c above b; the active item is skipped over.
	if !h.eng.Move(snap[2].ID, -1) {
		t.Fatal("move refused")
	}
	names := []string{}
	for _, it := range h.eng.Snapshot() {
		names = append(names, it.Name)
	}
	if names[1] != "c" || names[2] != "b" {
		t.Fatalf("order = %v", names)
	}
	if h.eng.Move(snap[2].ID, -1) {
		t.Fatal("moved past the active item")
	}
	if !h.eng.Remove(snap[1].ID) {
		t.Fatal("could not remove pending item")
	}

	// Finished items can be cleared, and a URL can be queued again.
	h.eng.MarkDone(snap[0].ID)
	if n := h.eng.ClearFinished(); n != 1 {
		t.Fatalf("cleared %d", n)
	}
	if n := h.eng.Add(NewItem{Name: "a", URL: "http://nas/a.pkg"}); n != 1 {
		t.Fatalf("re-add after done failed")
	}
}

func TestUnknownSizeCompletesOnQuiet(t *testing.T) {
	h := newHarness(t, 1)
	h.eng.Add(NewItem{Name: "a", URL: "http://nas/a.pkg", Size: -1})
	h.step()
	h.eng.OnEvent(served("/a.pkg", 10))
	h.clk.Advance(2 * time.Minute)
	h.eng.Poll(context.Background())
	h.expect(Done)
}

func TestRunLoop(t *testing.T) {
	h := newHarness(t, 1)
	h.eng.opts.PollInterval = 10 * time.Millisecond
	events := make(chan monitor.Event, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go h.eng.Run(ctx, events)

	h.eng.Add(NewItem{Name: "a", URL: "http://nas/a.pkg", Size: 5})
	waitFor(t, func() bool { return h.states()[0] == Downloading })
	events <- served("/a.pkg", 5)
	waitFor(t, func() bool { return h.eng.Snapshot()[0].Served == 5 })
	h.clk.Advance(2 * time.Minute)
	waitFor(t, func() bool { return h.states()[0] == Done })
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met in time")
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{-1: "?", 0: "0 B", 1023: "1023 B", 1024: "1.0 KB", 8_530_000_000: "7.9 GB"}
	for in, want := range cases {
		if got := HumanBytes(in); got != want {
			t.Errorf("HumanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}
