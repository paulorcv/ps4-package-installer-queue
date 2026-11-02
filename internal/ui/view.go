package ui

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/paulorcv/ps4-package-installer-queue/internal/queue"
)

const (
	headerHeight = 1
	urlHeight    = 3
	logLines     = 4
)

// View renders the whole screen.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "iniciando…"
	}
	leftW := m.width / 2
	rightW := m.width - leftW
	bodyH := m.bodyHeight()

	body := lipgloss.JoinHorizontal(lipgloss.Top,
		m.viewFiles(leftW, bodyH),
		m.viewQueue(rightW, bodyH),
	)
	return lipgloss.JoinVertical(lipgloss.Left,
		m.viewHeader(),
		m.viewURL(),
		body,
		m.viewLogs(),
		m.helpView(),
	)
}

func (m Model) bodyHeight() int {
	used := headerHeight + urlHeight + logLines + 1 + lipgloss.Height(m.helpView())
	return max(6, m.height-used)
}

// listRows is how many content lines fit inside a panel (minus its title).
func (m Model) listRows() int {
	return max(1, m.bodyHeight()-3)
}

func (m Model) helpView() string {
	if m.help.ShowAll {
		return m.help.FullHelpView(m.keys.full(m.focus))
	}
	return m.help.ShortHelpView(m.keys.short(m.focus))
}

func (m Model) viewHeader() string {
	left := styleTitle.Render("PS4 PKG Queue") + styleMuted.Render(" "+m.deps.Version)

	var mon string
	if m.monitor.Connected {
		mon = styleOK.Render("● NAS conectado")
	} else {
		mon = styleWarn.Render("○ " + m.monitor.Message)
	}
	right := mon + styleMuted.Render("  ·  PS4 "+m.deps.PS4Host)

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		right = ansi.Truncate(right, max(0, m.width-lipgloss.Width(left)-1), "…")
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m Model) viewURL() string {
	label := styleBold.Render("Pasta ")
	return panelStyle(m.focus == focusURL).Width(m.width - 2).Render(label + m.url.View())
}

func (m Model) viewFiles(w, h int) string {
	cw := w - 4
	rows := h - 3
	focused := m.focus == focusFiles

	title := styleBold.Render("Arquivos")
	if p := displayPath(m.curURL); p != "" {
		title += styleMuted.Render("  " + p)
	}
	if n := len(m.selOrder); n > 0 {
		title += styleOK.Render(fmt.Sprintf("  %d marcado(s)", n))
	}
	if m.loading {
		title += "  " + m.spin.View()
	}
	lines := []string{ansi.Truncate(title, cw, "…")}

	switch {
	case m.listErr != "" && len(m.entries) == 0:
		lines = append(lines, styleErr.Render(wrap(m.listErr, cw)))
	case m.loading && len(m.entries) == 0:
		lines = append(lines, styleMuted.Render("carregando…"))
	case len(m.entries) == 0:
		lines = append(lines, styleMuted.Render("nenhuma pasta ou .pkg aqui"))
	default:
		queued := m.queuedURLs()
		end := min(len(m.entries), m.fileTop+rows)
		for i := m.fileTop; i < end; i++ {
			lines = append(lines, m.fileLine(i, cw, focused, queued))
		}
	}
	return panelStyle(focused).Width(w - 2).Height(h - 2).Render(strings.Join(lines, "\n"))
}

func (m Model) fileLine(i, cw int, focused bool, queued map[string]queue.State) string {
	e := m.entries[i]
	cursor := " "
	if i == m.fileCur {
		cursor = "›"
	}

	var line string
	if e.IsDir {
		line = cursor + " " + styleDir.Render(ansi.Truncate("▸ "+e.Name+"/", cw-2, "…"))
	} else {
		mark := styleMuted.Render("[ ]")
		if _, ok := m.selected[e.URL]; ok {
			mark = styleOK.Render("[x]")
		}
		right := styleMuted.Render(queue.HumanBytes(e.Size))
		if st, ok := queued[e.URL]; ok {
			tag := st.String()
			style := styleWarn
			if st == queue.Done {
				style = styleOK
			} else if st == queue.Failed {
				style = styleErr
			}
			right = style.Render(tag) + "  " + right
		}
		nameW := cw - 2 - 4 - lipgloss.Width(right) - 1
		name := ansi.Truncate(e.Name, max(4, nameW), "…")
		pad := max(1, cw-2-4-lipgloss.Width(name)-lipgloss.Width(right))
		line = cursor + " " + mark + " " + name + strings.Repeat(" ", pad) + right
	}
	if i == m.fileCur && focused {
		return styleCursor.Width(cw).Render(line)
	}
	return line
}

func (m Model) viewQueue(w, h int) string {
	cw := w - 4
	rows := h - 3
	focused := m.focus == focusQueue

	var active, pending, done, failed int
	for _, it := range m.items {
		switch {
		case it.State.Active():
			active++
		case it.State == queue.Pending:
			pending++
		case it.State == queue.Done:
			done++
		case it.State == queue.Failed:
			failed++
		}
	}
	title := styleBold.Render("Fila") + styleMuted.Render(fmt.Sprintf("  %d ativo · %d pendente(s) · %d concluído(s)", active, pending, done))
	if failed > 0 {
		title += styleErr.Render(fmt.Sprintf(" · %d falha(s)", failed))
	}
	lines := []string{ansi.Truncate(title, cw, "…")}

	if len(m.items) == 0 {
		lines = append(lines, styleMuted.Render(wrap("fila vazia: marque arquivos à esquerda com espaço e pressione e", cw)))
	} else {
		perPage := max(1, rows/2)
		end := min(len(m.items), m.qTop+perPage)
		pos := 0
		for i, it := range m.items {
			if it.State == queue.Pending {
				pos++
			}
			if i < m.qTop || i >= end {
				continue
			}
			l1, l2 := m.queueLines(it, i, pos, cw)
			if i == m.qCur && focused {
				l1 = styleCursor.Width(cw).Render(l1)
				l2 = styleCursor.Width(cw).Render(l2)
			}
			lines = append(lines, l1, l2)
		}
	}
	return panelStyle(focused).Width(w - 2).Height(h - 2).Render(strings.Join(lines, "\n"))
}

func (m Model) queueLines(it queue.Item, i, pos, cw int) (string, string) {
	cursor := " "
	if i == m.qCur {
		cursor = "›"
	}

	var icon, state string
	switch it.State {
	case queue.Pending:
		icon, state = styleMuted.Render("○"), styleMuted.Render(fmt.Sprintf("#%d", pos))
	case queue.Sending:
		icon, state = m.spin.View(), styleWarn.Render("enviando")
	case queue.Downloading:
		icon, state = styleWarn.Render("↓"), styleWarn.Render("baixando")
	case queue.Done:
		icon, state = styleOK.Render("✓"), styleOK.Render("concluído")
	case queue.Failed:
		icon, state = styleErr.Render("✗"), styleErr.Render("falhou")
	}
	nameW := cw - 4 - lipgloss.Width(state) - 1
	name := ansi.Truncate(it.Name, max(4, nameW), "…")
	pad := max(1, cw-4-lipgloss.Width(name)-lipgloss.Width(state))
	l1 := cursor + " " + icon + " " + name + strings.Repeat(" ", pad) + state

	var detail string
	switch it.State {
	case queue.Pending:
		detail = styleMuted.Render(queue.HumanBytes(it.Size) + " · aguardando a vez")
	case queue.Sending:
		detail = styleMuted.Render("pedindo ao ezRemote para instalar…")
	case queue.Downloading:
		detail = m.progressLine(it, cw-4)
	case queue.Done:
		took := ""
		if !it.SentAt.IsZero() {
			took = " em " + duration(it.FinishedAt.Sub(it.SentAt))
		}
		detail = styleMuted.Render(fmt.Sprintf("%s · às %s%s", queue.HumanBytes(it.Size), it.FinishedAt.Format("15:04"), took))
	case queue.Failed:
		detail = styleErr.Render(it.Err)
	}
	l2 := "    " + ansi.Truncate(detail, cw-4, "…")
	return l1, l2
}

func (m Model) progressLine(it queue.Item, w int) string {
	pct := 0.0
	if it.Size > 0 {
		pct = min(1, float64(it.Served)/float64(it.Size))
	}
	activity := styleMuted.Render("aguardando o PS4")
	if it.Conns > 0 {
		activity = styleWarn.Render("transferindo")
	} else if !it.LastActivity.IsZero() {
		activity = styleMuted.Render("ocioso " + duration(m.now.Sub(it.LastActivity)))
	}
	text := fmt.Sprintf(" %3.0f%% %s/%s · ", pct*100, queue.HumanBytes(it.Served), queue.HumanBytes(it.Size)) +
		activity + styleMuted.Render(" · "+duration(m.now.Sub(it.SentAt)))

	bar := m.bar
	bar.Width = clamp(w-lipgloss.Width(text), 8, 30)
	return bar.ViewAs(pct) + text
}

func (m Model) viewLogs() string {
	rule := styleMuted.Render(strings.Repeat("─", m.width))
	start := max(0, len(m.logs)-logLines)
	lines := []string{rule}
	for _, l := range m.logs[start:] {
		lines = append(lines, styleMuted.Render(ansi.Truncate(l, m.width, "…")))
	}
	for len(lines) < logLines+1 {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func displayPath(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return u.Path
}

func duration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Round(time.Second)
	h := int(d.Hours())
	mm := int(d.Minutes()) % 60
	ss := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, mm, ss)
	}
	return fmt.Sprintf("%02d:%02d", mm, ss)
}

func wrap(s string, w int) string {
	return lipgloss.NewStyle().Width(max(10, w)).Render(s)
}
