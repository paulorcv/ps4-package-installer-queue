// Package ui implements the terminal interface: the source URL on top, the
// package browser on the left and the install queue on the right.
package ui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/paulorcv/ps4-package-installer-queue/internal/listing"
	"github.com/paulorcv/ps4-package-installer-queue/internal/monitor"
	"github.com/paulorcv/ps4-package-installer-queue/internal/queue"
)

const maxLogLines = 200

type focusArea int

const (
	focusURL focusArea = iota
	focusFiles
	focusQueue
)

// Deps are the collaborators the UI drives.
type Deps struct {
	Lister        *listing.Client
	Engine        *queue.Engine
	MonitorStatus <-chan monitor.Status
	StartURL      string
	PS4Host       string
	Version       string
}

// Model is the Bubble Tea model.
type Model struct {
	deps Deps
	keys keyMap
	help help.Model
	url  textinput.Model
	spin spinner.Model
	bar  progress.Model

	focus         focusArea
	width, height int
	now           time.Time

	// file browser
	curURL   string
	entries  []listing.Entry
	loading  bool
	listErr  string
	fileCur  int
	fileTop  int
	selected map[string]listing.Entry
	selOrder []string

	// queue
	items []queue.Item
	qCur  int
	qTop  int

	logs    []string
	monitor monitor.Status
}

// New builds the initial model.
func New(d Deps) Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "http://192.168.0.121:8080/"
	ti.CharLimit = 2048
	ti.SetValue(d.StartURL)

	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	sp.Style = styleTitle

	m := Model{
		deps:     d,
		keys:     newKeyMap(),
		help:     help.New(),
		url:      ti,
		spin:     sp,
		bar:      progress.New(progress.WithDefaultGradient(), progress.WithoutPercentage()),
		selected: map[string]listing.Entry{},
		now:      time.Now(),
		monitor:  monitor.Status{Message: "aguardando conexão com o NAS…"},
	}
	if d.StartURL == "" {
		m.setFocus(focusURL)
	} else {
		m.setFocus(focusFiles)
	}
	return m
}

// --- messages ---------------------------------------------------------------

type listedMsg struct {
	url     string
	entries []listing.Entry
	err     error
}

type queueChangedMsg struct{}
type logMsg string
type statusMsg monitor.Status
type tickMsg time.Time

// --- commands ---------------------------------------------------------------

func (m Model) loadDir(u string) tea.Cmd {
	lister := m.deps.Lister
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		entries, err := lister.List(ctx, u)
		if err == nil {
			lister.Sizes(ctx, entries)
		}
		return listedMsg{url: listing.EnsureDirURL(u), entries: entries, err: err}
	}
}

func waitChanges(e *queue.Engine) tea.Cmd {
	return func() tea.Msg {
		<-e.Changes()
		return queueChangedMsg{}
	}
}

func waitLog(e *queue.Engine) tea.Cmd {
	return func() tea.Msg { return logMsg(<-e.Logs()) }
}

func waitStatus(ch <-chan monitor.Status) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg { return statusMsg(<-ch) }
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// Init starts background listeners and loads the start URL.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		textinput.Blink,
		m.spin.Tick,
		waitChanges(m.deps.Engine),
		waitLog(m.deps.Engine),
		waitStatus(m.deps.MonitorStatus),
		tick(),
	}
	if m.deps.StartURL != "" {
		cmds = append(cmds, m.startLoad(m.deps.StartURL))
	}
	return tea.Batch(cmds...)
}

func (m *Model) startLoad(u string) tea.Cmd {
	m.loading = true
	m.listErr = ""
	return m.loadDir(u)
}

// --- update -----------------------------------------------------------------

// Update handles messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.Width = msg.Width
		m.url.Width = max(10, msg.Width-14)
		return m, nil

	case listedMsg:
		m.loading = false
		if msg.err != nil {
			m.listErr = msg.err.Error()
			m.addLog("erro ao listar " + msg.url + ": " + msg.err.Error())
			return m, nil
		}
		prev := m.curURL
		m.curURL = msg.url
		m.entries = msg.entries
		m.url.SetValue(msg.url)
		if prev != msg.url {
			m.fileCur, m.fileTop = 0, 0
		}
		m.clampFiles()
		return m, nil

	case queueChangedMsg:
		m.items = m.deps.Engine.Snapshot()
		m.clampQueue()
		return m, waitChanges(m.deps.Engine)

	case logMsg:
		m.addLog(string(msg))
		return m, waitLog(m.deps.Engine)

	case statusMsg:
		m.monitor = monitor.Status(msg)
		return m, waitStatus(m.deps.MonitorStatus)

	case tickMsg:
		m.now = time.Time(msg)
		return m, tick()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	if m.focus == focusURL {
		var cmd tea.Cmd
		m.url, cmd = m.url.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.keys
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	switch {
	case key.Matches(msg, k.NextPane):
		m.setFocus((m.focus + 1) % 3)
		return m, nil
	case key.Matches(msg, k.PrevPane):
		m.setFocus((m.focus + 2) % 3)
		return m, nil
	}

	if m.focus == focusURL {
		switch {
		case key.Matches(msg, k.Load):
			m.setFocus(focusFiles)
			return m, m.startLoad(m.url.Value())
		case key.Matches(msg, k.Cancel):
			m.url.SetValue(m.curURL)
			m.setFocus(focusFiles)
			return m, nil
		}
		var cmd tea.Cmd
		m.url, cmd = m.url.Update(msg)
		return m, cmd
	}

	switch {
	case key.Matches(msg, k.Quit):
		return m, tea.Quit
	case key.Matches(msg, k.FocusURL):
		m.setFocus(focusURL)
		return m, textinput.Blink
	case key.Matches(msg, k.Help):
		m.help.ShowAll = !m.help.ShowAll
		return m, nil
	}

	if m.focus == focusFiles {
		return m.handleFilesKey(msg)
	}
	return m.handleQueueKey(msg)
}

func (m Model) handleFilesKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.keys
	page := max(1, m.listRows())
	switch {
	case key.Matches(msg, k.Up):
		m.fileCur--
	case key.Matches(msg, k.Down):
		m.fileCur++
	case key.Matches(msg, k.PageUp):
		m.fileCur -= page
	case key.Matches(msg, k.PageDown):
		m.fileCur += page
	case key.Matches(msg, k.Home):
		m.fileCur = 0
	case key.Matches(msg, k.End):
		m.fileCur = len(m.entries) - 1
	case key.Matches(msg, k.Toggle):
		if e, ok := m.currentEntry(); ok && !e.IsDir {
			m.toggle(e)
			m.fileCur++
		}
	case key.Matches(msg, k.ToggleAll):
		m.toggleAll()
	case key.Matches(msg, k.ClearSel):
		m.clearSelection()
	case key.Matches(msg, k.Open):
		if e, ok := m.currentEntry(); ok {
			if e.IsDir {
				return m, m.startLoad(e.URL)
			}
			m.toggle(e)
		}
	case key.Matches(msg, k.Parent):
		if p, ok := listing.Parent(m.curURL); ok && m.curURL != "" {
			return m, m.startLoad(p)
		}
	case key.Matches(msg, k.Reload):
		if m.curURL != "" {
			return m, m.startLoad(m.curURL)
		}
	case key.Matches(msg, k.Enqueue):
		m.enqueue()
	}
	m.clampFiles()
	return m, nil
}

func (m Model) handleQueueKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.keys
	e := m.deps.Engine
	page := max(1, m.listRows()/2)
	cur, ok := m.currentItem()

	switch {
	case key.Matches(msg, k.Up):
		m.qCur--
	case key.Matches(msg, k.Down):
		m.qCur++
	case key.Matches(msg, k.PageUp):
		m.qCur -= page
	case key.Matches(msg, k.PageDown):
		m.qCur += page
	case key.Matches(msg, k.Home):
		m.qCur = 0
	case key.Matches(msg, k.End):
		m.qCur = len(m.items) - 1
	case key.Matches(msg, k.MoveUp):
		if ok && e.Move(cur.ID, -1) {
			m.items = e.Snapshot()
			m.qCur = indexOf(m.items, cur.ID)
		}
	case key.Matches(msg, k.MoveDown):
		if ok && e.Move(cur.ID, 1) {
			m.items = e.Snapshot()
			m.qCur = indexOf(m.items, cur.ID)
		}
	case key.Matches(msg, k.Remove):
		if ok && !e.Remove(cur.ID) {
			m.addLog("não é possível remover um item em andamento")
		}
	case key.Matches(msg, k.Retry):
		if ok && !e.Retry(cur.ID) {
			m.addLog("só itens com falha podem ser tentados de novo")
		}
	case key.Matches(msg, k.MarkDone):
		if ok && !e.MarkDone(cur.ID) {
			m.addLog("só itens baixando ou com falha podem ser marcados como concluídos")
		}
	case key.Matches(msg, k.ClearDone):
		e.ClearFinished()
	}
	m.clampQueue()
	return m, nil
}

// --- helpers ----------------------------------------------------------------

func (m *Model) setFocus(f focusArea) {
	m.focus = f
	if f == focusURL {
		m.url.Focus()
		m.url.CursorEnd()
	} else {
		m.url.Blur()
	}
}

func (m *Model) currentEntry() (listing.Entry, bool) {
	if m.fileCur < 0 || m.fileCur >= len(m.entries) {
		return listing.Entry{}, false
	}
	return m.entries[m.fileCur], true
}

func (m *Model) currentItem() (queue.Item, bool) {
	if m.qCur < 0 || m.qCur >= len(m.items) {
		return queue.Item{}, false
	}
	return m.items[m.qCur], true
}

func (m *Model) toggle(e listing.Entry) {
	if _, ok := m.selected[e.URL]; ok {
		delete(m.selected, e.URL)
		for i, u := range m.selOrder {
			if u == e.URL {
				m.selOrder = append(m.selOrder[:i], m.selOrder[i+1:]...)
				break
			}
		}
		return
	}
	m.selected[e.URL] = e
	m.selOrder = append(m.selOrder, e.URL)
}

func (m *Model) toggleAll() {
	all := true
	for _, e := range m.entries {
		if !e.IsDir {
			if _, ok := m.selected[e.URL]; !ok {
				all = false
				break
			}
		}
	}
	for _, e := range m.entries {
		if e.IsDir {
			continue
		}
		_, sel := m.selected[e.URL]
		if all == sel { // all selected => unselect; otherwise select missing
			m.toggle(e)
		}
	}
}

func (m *Model) clearSelection() {
	m.selected = map[string]listing.Entry{}
	m.selOrder = nil
}

func (m *Model) enqueue() {
	var items []queue.NewItem
	if len(m.selOrder) == 0 {
		if e, ok := m.currentEntry(); ok && !e.IsDir {
			items = append(items, queue.NewItem{Name: e.Name, URL: e.URL, Size: e.Size})
		}
	}
	for _, u := range m.selOrder {
		e := m.selected[u]
		items = append(items, queue.NewItem{Name: e.Name, URL: e.URL, Size: e.Size})
	}
	if len(items) == 0 {
		m.addLog("selecione um ou mais .pkg com espaço antes de enfileirar")
		return
	}
	m.deps.Engine.Add(items...)
	m.clearSelection()
}

func (m *Model) queuedURLs() map[string]queue.State {
	out := make(map[string]queue.State, len(m.items))
	for _, it := range m.items {
		if prev, ok := out[it.URL]; !ok || prev.Finished() {
			out[it.URL] = it.State
		}
	}
	return out
}

func (m *Model) addLog(s string) {
	if len(s) > 0 && (s[0] < '0' || s[0] > '9') {
		s = time.Now().Format("15:04:05") + " " + s
	}
	m.logs = append(m.logs, s)
	if len(m.logs) > maxLogLines {
		m.logs = m.logs[len(m.logs)-maxLogLines:]
	}
}

func (m *Model) clampFiles() {
	m.fileCur = clamp(m.fileCur, 0, len(m.entries)-1)
	m.fileTop = scrollTop(m.fileTop, m.fileCur, m.listRows())
}

func (m *Model) clampQueue() {
	m.qCur = clamp(m.qCur, 0, len(m.items)-1)
	m.qTop = scrollTop(m.qTop, m.qCur, max(1, m.listRows()/2))
}

func indexOf(items []queue.Item, id int) int {
	for i, it := range items {
		if it.ID == id {
			return i
		}
	}
	return 0
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	return min(max(v, lo), hi)
}

func scrollTop(top, cur, rows int) int {
	if rows < 1 {
		rows = 1
	}
	if cur < top {
		return cur
	}
	if cur >= top+rows {
		return cur - rows + 1
	}
	return max(top, 0)
}
