package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"dockman/internal/model"
)

type statsDashboardTickMsg struct{}

func (a App) loadContainerStatsDashboard() tea.Cmd {
	return func() tea.Msg {
		items, err := a.containersService.List(context.Background())
		return containersLoadedMsg{items: items, err: err}
	}
}

func statsDashboardTick() tea.Cmd {
	return tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg { return statsDashboardTickMsg{} })
}

func (a App) updateContainerStatsDashboard(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "s":
		a.containers.sort = 4
	case "m":
		a.containers.sort = 5
	case "p", " ":
		a.containers.statsPaused = !a.containers.statsPaused
		if !a.containers.statsPaused {
			return a, a.loadContainerStatsDashboard()
		}
	case "r":
		return a, a.loadContainerStatsDashboard()
	case "esc", "q", "backspace":
		a.containers.sort = 4
		a.screen = ScreenContainers
	}
	return a, nil
}

func (a App) renderContainerStatsDashboard() string {
	items := runningContainers(append([]model.Container(nil), a.containers.items...))
	sort.SliceStable(items, func(i, j int) bool {
		if a.containers.sort == 5 {
			return items[i].MemoryPercent > items[j].MemoryPercent
		}
		return items[i].CPUPercent > items[j].CPUPercent
	})
	var b strings.Builder
	b.WriteString(titleStyle.Render("Real-time Container Stats") + "\n")
	state := "LIVE · refresh 1.5s"
	if a.containers.statsPaused {
		state = "PAUSED"
	}
	sortName := "CPU"
	if a.containers.sort == 5 {
		sortName = "Memory"
	}
	b.WriteString(mutedStyle.Render(state+" · sort: "+sortName) + "\n\n")
	b.WriteString(fmt.Sprintf("  %-22s %8s %19s %17s %17s\n", "NAME", "CPU", "MEM USAGE / %", "NET I/O", "BLOCK I/O"))
	for _, item := range items {
		line := fmt.Sprintf("  %-22s %7.1f%% %8s / %5.1f%% %8s/%-8s %8s/%-8s", truncate(item.Name, 22), item.CPUPercent, formatBytes(item.MemoryUsage), item.MemoryPercent, formatBytes(item.NetworkRx), formatBytes(item.NetworkTx), formatBytes(item.BlockRead), formatBytes(item.BlockWrite))
		if item.CPUPercent >= 80 || item.MemoryPercent >= 85 {
			line = errorStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}
	if len(items) == 0 {
		b.WriteString("No running containers.\n")
	}
	b.WriteString("\n" + mutedStyle.Render("s CPU sort   m Memory sort   p/Space Pause   r Refresh   Esc Back"))
	return b.String()
}

func runningContainers(items []model.Container) []model.Container {
	out := items[:0]
	for _, item := range items {
		if item.Running() {
			out = append(out, item)
		}
	}
	return out
}
