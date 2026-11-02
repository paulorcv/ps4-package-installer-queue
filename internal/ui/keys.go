package ui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	// global
	Quit, NextPane, PrevPane, FocusURL, Help key.Binding
	// url
	Load, Cancel key.Binding
	// lists
	Up, Down, PageUp, PageDown, Home, End key.Binding
	// files
	Toggle, ToggleAll, Open, Parent, Enqueue, Reload, ClearSel key.Binding
	// queue
	Remove, Retry, MarkDone, MoveUp, MoveDown, ClearDone key.Binding
}

func newKeyMap() keyMap {
	b := func(keys []string, k, desc string) key.Binding {
		return key.NewBinding(key.WithKeys(keys...), key.WithHelp(k, desc))
	}
	return keyMap{
		Quit:     b([]string{"q", "ctrl+c"}, "q", "sair"),
		NextPane: b([]string{"tab"}, "tab", "trocar painel"),
		PrevPane: b([]string{"shift+tab"}, "shift+tab", "painel anterior"),
		FocusURL: b([]string{"u", "/"}, "u", "editar URL"),
		Help:     b([]string{"?"}, "?", "ajuda"),

		Load:   b([]string{"enter"}, "enter", "abrir URL"),
		Cancel: b([]string{"esc"}, "esc", "voltar"),

		Up:       b([]string{"up", "k"}, "↑/k", "subir"),
		Down:     b([]string{"down", "j"}, "↓/j", "descer"),
		PageUp:   b([]string{"pgup"}, "pgup", "página acima"),
		PageDown: b([]string{"pgdown"}, "pgdn", "página abaixo"),
		Home:     b([]string{"home", "g"}, "g", "início"),
		End:      b([]string{"end", "G"}, "G", "fim"),

		Toggle:    b([]string{" "}, "espaço", "marcar"),
		ToggleAll: b([]string{"a"}, "a", "marcar todos"),
		Open:      b([]string{"enter", "right", "l"}, "enter", "abrir pasta"),
		Parent:    b([]string{"backspace", "left", "h"}, "⌫", "pasta acima"),
		Enqueue:   b([]string{"e", "i"}, "e", "enfileirar"),
		Reload:    b([]string{"r", "f5"}, "r", "recarregar"),
		ClearSel:  b([]string{"esc"}, "esc", "limpar seleção"),

		Remove:    b([]string{"d", "delete"}, "d", "remover"),
		Retry:     b([]string{"r"}, "r", "tentar de novo"),
		MarkDone:  b([]string{"m"}, "m", "marcar concluído"),
		MoveUp:    b([]string{"K", "shift+up"}, "K", "mover p/ cima"),
		MoveDown:  b([]string{"J", "shift+down"}, "J", "mover p/ baixo"),
		ClearDone: b([]string{"c"}, "c", "limpar concluídos"),
	}
}

func (k keyMap) short(f focusArea) []key.Binding {
	switch f {
	case focusURL:
		return []key.Binding{k.Load, k.Cancel, k.NextPane}
	case focusFiles:
		return []key.Binding{k.Toggle, k.ToggleAll, k.Open, k.Parent, k.Enqueue, k.NextPane, k.Help, k.Quit}
	default:
		return []key.Binding{k.MoveUp, k.MoveDown, k.Remove, k.Retry, k.MarkDone, k.ClearDone, k.NextPane, k.Help, k.Quit}
	}
}

func (k keyMap) full(f focusArea) [][]key.Binding {
	nav := []key.Binding{k.Up, k.Down, k.PageUp, k.PageDown, k.Home, k.End}
	global := []key.Binding{k.NextPane, k.PrevPane, k.FocusURL, k.Help, k.Quit}
	switch f {
	case focusURL:
		return [][]key.Binding{{k.Load, k.Cancel}, global}
	case focusFiles:
		return [][]key.Binding{nav, {k.Toggle, k.ToggleAll, k.ClearSel, k.Enqueue}, {k.Open, k.Parent, k.Reload}, global}
	default:
		return [][]key.Binding{nav, {k.MoveUp, k.MoveDown, k.Remove}, {k.Retry, k.MarkDone, k.ClearDone}, global}
	}
}
