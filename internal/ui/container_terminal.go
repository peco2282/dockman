package ui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"dockman/internal/model"
)

type terminalOpenedMsg struct {
	session model.TerminalSession
	events  <-chan model.TerminalEvent
	err     error
}
type terminalEventMsg struct{ event model.TerminalEvent }
type terminalWriteResultMsg struct{ err error }

func (a App) openContainerTerminal() (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	a.terminalCancel = cancel
	a.screen = ScreenContainerTerminal
	a.containers.loading = true
	a.containers.err = nil
	a.containers.terminalDone = false
	a.containers.terminalOutput = ""
	width, height := terminalDimensions(a.width, a.height)
	id := a.containers.detail.ID
	return a, tea.Batch(a.spinner.Tick, func() tea.Msg {
		session, events, err := a.containersService.OpenTerminal(ctx, id, width, height)
		return terminalOpenedMsg{session, events, err}
	})
}

func waitForTerminal(events <-chan model.TerminalEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-events
		if !ok {
			return terminalEventMsg{model.TerminalEvent{Done: true}}
		}
		return terminalEventMsg{event}
	}
}

func (a *App) appendTerminalOutput(data []byte) {
	text := ansi.Strip(string(data))
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "")
	a.containers.terminalOutput = applyTerminalText(a.containers.terminalOutput, text)
	const maxOutput = 1024 * 1024
	if len(a.containers.terminalOutput) > maxOutput {
		a.containers.terminalOutput = strings.ToValidUTF8(a.containers.terminalOutput[len(a.containers.terminalOutput)-maxOutput:], "")
	}
}

func (a App) updateContainerTerminal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	// Esc belongs to the shell (vim, less, readline) while a session is open,
	// so only Ctrl+] detaches. Without a live session Esc simply goes back.
	live := a.containers.terminalSession != nil && !a.containers.terminalDone
	if key == "ctrl+]" || (key == "esc" && !live) {
		a.closeContainerTerminal()
		a.screen = a.containers.originOr()
		return a, nil
	}
	if a.containers.loading {
		return a, nil
	}
	if a.containers.terminalDone {
		if key == "enter" {
			a.closeContainerTerminal()
			a.screen = a.containers.originOr()
		}
		return a, nil
	}
	if a.containers.terminalSession == nil {
		return a, nil
	}
	data := terminalKeyBytes(msg)
	if len(data) == 0 {
		return a, nil
	}
	session := a.containers.terminalSession
	return a, func() tea.Msg { _, err := session.Write(data); return terminalWriteResultMsg{err} }
}

func (a *App) closeContainerTerminal() {
	if a.terminalCancel != nil {
		a.terminalCancel()
		a.terminalCancel = nil
	}
	if a.containers.terminalSession != nil {
		_ = a.containers.terminalSession.Close()
		a.containers.terminalSession = nil
	}
	a.containers.terminalEvents = nil
}

func (a App) resizeContainerTerminal() tea.Cmd {
	session := a.containers.terminalSession
	width, height := terminalDimensions(a.width, a.height)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return terminalWriteResultMsg{session.Resize(ctx, width, height)}
	}
}
func terminalDimensions(width, height int) (uint, uint) {
	if width < 20 {
		width = 80
	}
	height -= 3
	if height < 5 {
		height = 24
	}
	return uint(width), uint(height)
}

func terminalKeyBytes(msg tea.KeyPressMsg) []byte {
	key := msg.Key()
	if key.Text != "" {
		return []byte(key.Text)
	}
	switch msg.String() {
	case "enter":
		return []byte{'\r'}
	case "esc":
		return []byte{0x1b}
	case "tab":
		return []byte{'\t'}
	case "backspace":
		return []byte{0x7f}
	case "up":
		return []byte("\x1b[A")
	case "down":
		return []byte("\x1b[B")
	case "right":
		return []byte("\x1b[C")
	case "left":
		return []byte("\x1b[D")
	case "home":
		return []byte("\x1b[H")
	case "end":
		return []byte("\x1b[F")
	case "delete":
		// Treat Delete like Backspace in this beginner-focused shell. Full-screen
		// terminal applications can still use Ctrl+D for forward-delete behavior.
		return []byte{0x7f}
	case "pgup":
		return []byte("\x1b[5~")
	case "pgdown":
		return []byte("\x1b[6~")
	}
	value := msg.String()
	if strings.HasPrefix(value, "ctrl+") && len(value) == 6 {
		letter := value[5]
		if letter >= 'a' && letter <= 'z' {
			return []byte{letter - 'a' + 1}
		}
	}
	return nil
}

func applyTerminalText(current, incoming string) string {
	runes := []rune(current)
	for _, r := range incoming {
		switch r {
		case '\b':
			if len(runes) > 0 && runes[len(runes)-1] != '\n' {
				runes = runes[:len(runes)-1]
			}
		case 0:
			// Ignore NUL control bytes.
		default:
			runes = append(runes, r)
		}
	}
	return string(runes)
}

func (a App) renderContainerTerminal() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Terminal - "+a.containers.detail.Name) + "\n")
	b.WriteString(mutedStyle.Render("Ctrl+] Detach   (for vim/top, use e for a full-screen shell)") + "\n")
	if a.containers.loading {
		b.WriteString("\n" + a.spinner.View() + " Opening /bin/bash or /bin/sh...")
		return b.String()
	}
	if a.containers.err != nil {
		b.WriteString("\n" + errorStyle.Render("Terminal error") + "\n" + friendlyError(a.containers.err))
		if a.containers.terminalSession == nil {
			return b.String() + "\n\n" + mutedStyle.Render("Esc Back")
		}
	}
	lines := strings.Split(a.containers.terminalOutput, "\n")
	page := a.height - 3
	if page < 5 {
		page = 20
	}
	if len(lines) > page {
		lines = lines[len(lines)-page:]
	}
	b.WriteString("\n" + strings.Join(lines, "\n"))
	if a.containers.terminalDone {
		b.WriteString("\n\n" + mutedStyle.Render("Shell exited. Press Enter or Esc to return."))
	}
	return b.String()
}
