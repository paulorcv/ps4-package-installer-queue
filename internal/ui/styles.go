package ui

import "github.com/charmbracelet/lipgloss"

var (
	colAccent  = lipgloss.AdaptiveColor{Light: "#5A3FD6", Dark: "#9B87F5"}
	colMuted   = lipgloss.AdaptiveColor{Light: "#8A8A8A", Dark: "#6C6C6C"}
	colBorder  = lipgloss.AdaptiveColor{Light: "#C8C8C8", Dark: "#3A3A3A"}
	colOK      = lipgloss.AdaptiveColor{Light: "#1E8A4C", Dark: "#4CC38A"}
	colWarn    = lipgloss.AdaptiveColor{Light: "#A86A00", Dark: "#E5B567"}
	colErr     = lipgloss.AdaptiveColor{Light: "#C62828", Dark: "#F07178"}
	colDir     = lipgloss.AdaptiveColor{Light: "#1565C0", Dark: "#82AAFF"}
	colCursorB = lipgloss.AdaptiveColor{Light: "#ECE8FB", Dark: "#2A2440"}

	styleTitle   = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	styleMuted   = lipgloss.NewStyle().Foreground(colMuted)
	styleOK      = lipgloss.NewStyle().Foreground(colOK)
	styleWarn    = lipgloss.NewStyle().Foreground(colWarn)
	styleErr     = lipgloss.NewStyle().Foreground(colErr)
	styleDir     = lipgloss.NewStyle().Foreground(colDir).Bold(true)
	styleBold    = lipgloss.NewStyle().Bold(true)
	styleCursor  = lipgloss.NewStyle().Background(colCursorB)
	stylePanel   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colBorder).Padding(0, 1)
	stylePanelOn = stylePanel.BorderForeground(colAccent)
)

func panelStyle(focused bool) lipgloss.Style {
	if focused {
		return stylePanelOn
	}
	return stylePanel
}
