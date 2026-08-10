package ui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type DockerContextManager interface {
	ListContexts(context.Context) ([]string, string, error)
	SwitchContext(context.Context, string) error
}

type contextScreen struct {
	items   []string
	current string
	cursor  int
	loading bool
	err     error
}
type contextsLoadedMsg struct {
	items   []string
	current string
	err     error
}
type contextSwitchedMsg struct {
	name string
	err  error
}

func (a App) loadContexts() tea.Cmd {
	return func() tea.Msg {
		m, ok := a.connection.(DockerContextManager)
		if !ok {
			return contextsLoadedMsg{err: context.Canceled}
		}
		items, current, err := m.ListContexts(context.Background())
		return contextsLoadedMsg{items, current, err}
	}
}
func (a App) switchContext(name string) tea.Cmd {
	return func() tea.Msg {
		m, ok := a.connection.(DockerContextManager)
		if !ok {
			return contextSwitchedMsg{name, context.Canceled}
		}
		return contextSwitchedMsg{name, m.SwitchContext(context.Background(), name)}
	}
}

func (a App) updateContexts(key string) (tea.Model, tea.Cmd) {
	if a.contexts.loading {
		if key == "esc" {
			a.screen = ScreenContainers
		}
		return a, nil
	}
	switch key {
	case "up", "k":
		if len(a.contexts.items) > 0 {
			a.contexts.cursor = (a.contexts.cursor - 1 + len(a.contexts.items)) % len(a.contexts.items)
		}
	case "down", "j", "tab":
		if len(a.contexts.items) > 0 {
			a.contexts.cursor = (a.contexts.cursor + 1) % len(a.contexts.items)
		}
	case "enter":
		if len(a.contexts.items) > 0 {
			a.contexts.loading = true
			return a, tea.Batch(a.spinner.Tick, a.switchContext(a.contexts.items[a.contexts.cursor]))
		}
	case "esc", "q", "backspace":
		a.screen = ScreenContainers
	}
	return a, nil
}

func (a App) renderContexts() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Docker Context") + "\n\n")
	if a.contexts.loading {
		b.WriteString(a.spinner.View() + " Switching context...\n")
	} else if a.contexts.err != nil {
		b.WriteString(errorStyle.Render(friendlyError(a.contexts.err)) + "\n")
	} else {
		for i, name := range a.contexts.items {
			label := "  " + name
			if name == a.contexts.current {
				label += " (current)"
			}
			if i == a.contexts.cursor {
				label = selectedStyle.Render("> " + strings.TrimSpace(label))
			}
			b.WriteString(label + "\n")
		}
	}
	b.WriteString("\n" + mutedStyle.Render("↑↓ Move   Enter Switch   Esc Back"))
	return b.String()
}
