package ui

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/paulorcv/ps4-package-installer-queue/internal/listing"
	"github.com/paulorcv/ps4-package-installer-queue/internal/monitor"
	"github.com/paulorcv/ps4-package-installer-queue/internal/queue"
)

type nopInstaller struct{}

func (nopInstaller) InstallURL(context.Context, string) error { return nil }

type nopActivity struct{}

func (nopActivity) Connections(context.Context) (int, error) { return 1, nil }

func newTestModel(t *testing.T) (Model, *queue.Engine) {
	t.Helper()
	eng := queue.New(queue.Options{
		PS4Host: "192.168.0.104", MaxConcurrent: 1,
		StartTimeout: time.Minute, IdleDone: time.Minute, StallTimeout: time.Minute, PollInterval: time.Hour,
	}, nopInstaller{}, nopActivity{})

	m := New(Deps{
		Lister:   listing.New(http.DefaultClient),
		Engine:   eng,
		StartURL: "http://192.168.0.121:8080/",
		PS4Host:  "192.168.0.104",
		Version:  "test",
	})
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	m = update(t, m, listedMsg{url: "http://192.168.0.121:8080/", entries: []listing.Entry{
		{Name: "GOD OF WAR 1", URL: "http://192.168.0.121:8080/GOD%20OF%20WAR%201/", IsDir: true, Size: -1},
		{Name: "Mega Man X Collection.pkg", URL: "http://192.168.0.121:8080/mmx.pkg", Size: 2_920_000_000},
		{Name: "Tekken 5.pkg", URL: "http://192.168.0.121:8080/t5.pkg", Size: 4_180_000_000},
		{Name: "Resident Evil 4.pkg", URL: "http://192.168.0.121:8080/re4.pkg", Size: 4_130_000_000},
	}})
	return m, eng
}

func update(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	nm, _ := m.Update(msg)
	return nm.(Model)
}

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestSelectAndEnqueue(t *testing.T) {
	m, eng := newTestModel(t)

	// Cursor starts on the directory; space on a directory does nothing.
	m = update(t, m, keyMsg(" "))
	if len(m.selOrder) != 0 {
		t.Fatal("directory was selected")
	}
	m = update(t, m, keyMsg("down"))
	m = update(t, m, keyMsg(" ")) // Mega Man, cursor moves to Tekken
	m = update(t, m, keyMsg("down"))
	m = update(t, m, keyMsg(" ")) // Resident Evil 4
	if len(m.selOrder) != 2 {
		t.Fatalf("selected %d, want 2", len(m.selOrder))
	}
	m = update(t, m, keyMsg("e"))
	if len(m.selOrder) != 0 {
		t.Error("selection not cleared after enqueue")
	}

	snap := eng.Snapshot()
	if len(snap) != 2 || snap[0].Name != "Mega Man X Collection.pkg" || snap[1].Name != "Resident Evil 4.pkg" {
		t.Fatalf("queue = %+v", snap)
	}

	// Adding more while the queue already has items.
	m = update(t, m, keyMsg("a"))
	m = update(t, m, keyMsg("e"))
	if got := len(eng.Snapshot()); got != 3 {
		t.Fatalf("queue length = %d, want 3 (duplicates skipped)", got)
	}
}

func TestViewRenders(t *testing.T) {
	m, eng := newTestModel(t)
	eng.Add(
		queue.NewItem{Name: "Mega Man X Collection.pkg", URL: "http://192.168.0.121:8080/mmx.pkg", Size: 2_920_000_000},
		queue.NewItem{Name: "Tekken 5.pkg", URL: "http://192.168.0.121:8080/t5.pkg", Size: 4_180_000_000},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan monitor.Event, 1)
	go eng.Run(ctx, events)
	deadline := time.Now().Add(2 * time.Second)
	for eng.Snapshot()[0].State != queue.Downloading && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	events <- monitor.Event{Host: "192.168.0.104", Method: "GET", Path: "/mmx.pkg", Status: 206, Bytes: 1_200_000_000}
	time.Sleep(50 * time.Millisecond)
	eng.Poll(ctx)

	m = update(t, m, queueChangedMsg{})
	m = update(t, m, statusMsg(monitor.Status{Connected: true}))
	m = update(t, m, keyMsg("down"))
	m = update(t, m, keyMsg(" "))

	out := m.View()
	plain := ansi.Strip(out)
	for _, want := range []string{"PS4 PKG Queue", "NAS conectado", "Arquivos", "Fila", "Tekken 5.pkg", "[x]", "baixando", "#1", "GOD OF WAR 1/"} {
		if !strings.Contains(plain, want) {
			t.Errorf("view missing %q", want)
		}
	}
	for i, line := range strings.Split(plain, "\n") {
		if w := ansi.StringWidth(line); w > 120 {
			t.Errorf("line %d is %d columns wide (max 120): %q", i, w, line)
		}
	}
	if h := len(strings.Split(out, "\n")); h > 30 {
		t.Errorf("view is %d lines tall (max 30)", h)
	}
	if os.Getenv("PS4PKGQ_DUMP_VIEW") != "" {
		t.Log("\n" + plain)
	}
}
