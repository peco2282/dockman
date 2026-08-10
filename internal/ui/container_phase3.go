package ui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"dockman/internal/model"
	"dockman/internal/service"
)

type containerStatsLoadedMsg struct {
	stats model.ContainerStats
	err   error
}
type logStreamOpenedMsg struct {
	events <-chan service.LogEvent
	err    error
}
type logEventMsg struct{ event service.LogEvent }
type containerDeleteResultMsg struct{ err error }

func (a App) loadContainerStats(id string) tea.Cmd {
	return func() tea.Msg {
		stats, err := a.containersService.Stats(context.Background(), id)
		return containerStatsLoadedMsg{stats, err}
	}
}

func (a App) openContainerLogs() (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	a.logCancel = cancel
	a.screen = ScreenContainerLogs
	a.containers.loading, a.containers.err, a.containers.logDone = true, nil, false
	a.containers.logLines, a.containers.logOffset, a.containers.logFollow = nil, 0, true
	id := a.containers.detail.ID
	return a, tea.Batch(a.spinner.Tick, func() tea.Msg {
		events, err := a.containersService.FollowLogs(ctx, id)
		return logStreamOpenedMsg{events, err}
	})
}

func waitForLog(events <-chan service.LogEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-events
		if !ok {
			return logEventMsg{event: service.LogEvent{Done: true}}
		}
		return logEventMsg{event: event}
	}
}

func (a *App) appendLog(line string) {
	const maxLines = 2000
	a.containers.logLines = append(a.containers.logLines, line)
	if len(a.containers.logLines) > maxLines {
		removed := len(a.containers.logLines) - maxLines
		a.containers.logLines = a.containers.logLines[removed:]
		a.containers.logOffset = max(0, a.containers.logOffset-removed)
	}
	if a.containers.logFollow {
		a.containers.logOffset = max(0, len(a.containers.logLines)-a.logPageSize())
	}
}

func (a App) logPageSize() int {
	if a.height > 8 {
		return a.height - 7
	}
	return 10
}

func (a App) updateContainerLogs(key string) (tea.Model, tea.Cmd) {
	page := a.logPageSize()
	maxOffset := max(0, len(a.containers.logLines)-page)
	switch key {
	case "up", "k":
		a.containers.logFollow = false
		a.containers.logOffset = max(0, a.containers.logOffset-1)
	case "down", "j":
		a.containers.logOffset = min(maxOffset, a.containers.logOffset+1)
		a.containers.logFollow = a.containers.logOffset == maxOffset
	case "pgup":
		a.containers.logFollow = false
		a.containers.logOffset = max(0, a.containers.logOffset-page)
	case "pgdown":
		a.containers.logOffset = min(maxOffset, a.containers.logOffset+page)
		a.containers.logFollow = a.containers.logOffset == maxOffset
	case "end", "f":
		a.containers.logOffset = maxOffset
		a.containers.logFollow = true
	case "esc", "q", "backspace":
		if a.logCancel != nil {
			a.logCancel()
			a.logCancel = nil
		}
		a.screen = ScreenContainerDetail
	}
	return a, nil
}

func (a App) renderContainerLogs() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Logs - "+a.containers.detail.Name) + "\n")
	status := "LIVE / following"
	if !a.containers.logFollow {
		status = "PAUSED / scroll mode"
	}
	if a.containers.logDone {
		status = "STREAM ENDED"
	}
	b.WriteString(mutedStyle.Render(status) + "\n\n")
	if a.containers.loading {
		b.WriteString(a.spinner.View() + " Opening log stream...\n")
	} else if a.containers.err != nil {
		b.WriteString(errorStyle.Render("Could not read logs") + "\n" + friendlyError(a.containers.err) + "\n")
	} else if len(a.containers.logLines) == 0 {
		b.WriteString("No logs available yet.\n")
	} else {
		end := min(len(a.containers.logLines), a.containers.logOffset+a.logPageSize())
		for _, line := range a.containers.logLines[a.containers.logOffset:end] {
			b.WriteString(truncate(line, max(20, a.width-4)) + "\n")
		}
	}
	b.WriteString("\n" + mutedStyle.Render("↑↓ Scroll   PgUp/PgDn Page   End/f Follow   Esc Back"))
	return b.String()
}

func (a App) updateContainerDelete(key string) (tea.Model, tea.Cmd) {
	count := 2
	if a.containers.detail.Running {
		count = 3
	}
	if a.containers.working {
		return a, nil
	}
	switch key {
	case "left", "up":
		a.containers.deleteCursor = (a.containers.deleteCursor - 1 + count) % count
	case "right", "down", "tab":
		a.containers.deleteCursor = (a.containers.deleteCursor + 1) % count
	case "esc", "q", "backspace":
		a.screen = ScreenContainerDetail
	case "enter":
		if a.containers.deleteCursor == 0 {
			a.screen = ScreenContainerDetail
			return a, nil
		}
		mode := service.DeleteStopped
		if a.containers.detail.Running {
			if a.containers.deleteCursor == 1 {
				mode = service.StopThenDelete
			} else {
				mode = service.ForceDelete
			}
		}
		a.containers.working, a.containers.err, a.containers.notice = true, nil, "Deleting..."
		id := a.containers.detail.ID
		return a, tea.Batch(a.spinner.Tick, func() tea.Msg {
			return containerDeleteResultMsg{a.containersService.Delete(context.Background(), id, mode)}
		})
	}
	return a, nil
}

func (a App) renderContainerDelete() string {
	d := a.containers.detail
	var b strings.Builder
	b.WriteString(errorStyle.Render(fmt.Sprintf("Delete %q?", d.Name)) + "\n\nThis will delete the container.\nVolumes will NOT be deleted.\n")
	labels := []string{"[ Cancel ]"}
	if d.Running {
		b.WriteString("\nThe container is currently running. Choose how to proceed.\n")
		labels = append(labels, "[ Stop & Delete ]", "[ Force Delete ]")
	} else {
		labels = append(labels, "[ Delete ]")
	}
	b.WriteString("\n")
	for i, label := range labels {
		if i == a.containers.deleteCursor {
			label = selectedStyle.Render(label)
		}
		b.WriteString(label + "  ")
	}
	if a.containers.notice != "" {
		b.WriteString("\n\n" + a.containers.notice)
	}
	if a.containers.working {
		b.WriteString(" " + a.spinner.View())
	}
	if a.containers.err != nil {
		b.WriteString("\n" + errorStyle.Render(friendlyError(a.containers.err)))
	}
	b.WriteString("\n\n" + mutedStyle.Render("←→ Choose   Enter Confirm   Esc Cancel"))
	return b.String()
}
