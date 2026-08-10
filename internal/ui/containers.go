package ui

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"dockman/internal/model"
	"dockman/internal/service"
)

type containerScreen struct {
	items           []model.Container
	cursor          int
	detail          model.ContainerDetail
	actionCursor    int
	loading         bool
	working         bool
	err             error
	notice          string
	stats           model.ContainerStats
	statsLoading    bool
	statsErr        error
	statsPaused     bool
	logLines        []string
	logOffset       int
	logFollow       bool
	logDone         bool
	logEvents       <-chan service.LogEvent
	deleteCursor    int
	create          containerCreateForm
	terminalSession model.TerminalSession
	terminalEvents  <-chan model.TerminalEvent
	terminalOutput  string
	terminalDone    bool
	searching       bool
	query           string
	filter          int
	sort            int
}

var containerFilters = []string{"All", "Running", "Stopped"}
var containerSorts = []string{"Name", "State", "Created", "Size", "CPU"}

func (s *containerScreen) visibleItems() []model.Container {
	items := make([]model.Container, 0, len(s.items))
	q := strings.ToLower(strings.TrimSpace(s.query))
	projects := s.composeProjects()
	if s.filter >= 3+len(projects) {
		s.filter = 0
	}
	for _, item := range s.items {
		if q != "" && !strings.Contains(strings.ToLower(item.Name), q) && !strings.Contains(strings.ToLower(item.Image), q) {
			continue
		}
		if (s.filter == 1 && !item.Running()) || (s.filter == 2 && item.Running()) || (s.filter >= 3 && item.ComposeProject != projects[s.filter-3]) {
			continue
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		switch s.sort {
		case 1:
			return a.State < b.State
		case 2:
			return a.Created.After(b.Created)
		case 3:
			return a.Size > b.Size
		case 4:
			return a.CPUPercent > b.CPUPercent
		default:
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		}
	})
	return items
}

func (s *containerScreen) composeProjects() []string {
	seen := map[string]bool{}
	var projects []string
	for _, item := range s.items {
		if item.ComposeProject != "" && !seen[item.ComposeProject] {
			seen[item.ComposeProject] = true
			projects = append(projects, item.ComposeProject)
		}
	}
	sort.Strings(projects)
	return projects
}

func (s *containerScreen) filterLabel() string {
	if s.filter < len(containerFilters) {
		return containerFilters[s.filter]
	}
	projects := s.composeProjects()
	if s.filter-3 < len(projects) {
		return "Compose: " + projects[s.filter-3]
	}
	return "All"
}

type containersLoadedMsg struct {
	items []model.Container
	err   error
}
type containerDetailLoadedMsg struct {
	detail model.ContainerDetail
	err    error
}
type containerActionResultMsg struct {
	action service.ContainerAction
	err    error
}

func (a App) loadContainers() tea.Cmd {
	return func() tea.Msg {
		items, err := a.containersService.List(context.Background())
		return containersLoadedMsg{items, err}
	}
}
func (a App) loadContainerDetail(id string) tea.Cmd {
	return func() tea.Msg {
		detail, err := a.containersService.Detail(context.Background(), id)
		return containerDetailLoadedMsg{detail, err}
	}
}
func (a App) executeContainerAction(id string, action service.ContainerAction) tea.Cmd {
	return func() tea.Msg {
		return containerActionResultMsg{action, a.containersService.Execute(context.Background(), id, action)}
	}
}

func (a App) updateContainers(key string) (tea.Model, tea.Cmd) {
	if a.containers.loading {
		if key == "esc" || key == "q" {
			a.screen = ScreenMainMenu
		}
		return a, nil
	}
	if a.containers.searching {
		switch key {
		case "enter":
			a.containers.searching = false
		case "esc":
			a.containers.searching, a.containers.query = false, ""
		case "backspace":
			r := []rune(a.containers.query)
			if len(r) > 0 {
				a.containers.query = string(r[:len(r)-1])
			}
		default:
			if len([]rune(key)) == 1 {
				a.containers.query += key
			}
		}
		a.containers.cursor = 0
		return a, nil
	}
	items := a.containers.visibleItems()
	switch key {
	case "up", "k":
		if len(items) > 0 {
			a.containers.cursor = (a.containers.cursor - 1 + len(items)) % len(items)
		}
	case "down", "j", "tab":
		if len(items) > 0 {
			a.containers.cursor = (a.containers.cursor + 1) % len(items)
		}
	case "/":
		a.containers.searching = true
	case "f":
		a.containers.filter = (a.containers.filter + 1) % (len(containerFilters) + len(a.containers.composeProjects()))
		a.containers.cursor = 0
	case "s":
		a.containers.sort = (a.containers.sort + 1) % len(containerSorts)
		a.containers.cursor = 0
	case "x":
		a.screen, a.contexts.loading, a.contexts.err = ScreenContexts, true, nil
		return a, tea.Batch(a.spinner.Tick, a.loadContexts())
	case "r":
		a.containers.loading, a.containers.err, a.containers.notice = true, nil, ""
		return a, tea.Batch(a.spinner.Tick, a.loadContainers())
	case "c":
		a.containers.create = newContainerCreateForm()
		a.containers.err, a.containers.notice = nil, ""
		a.screen = ScreenContainerCreate
		return a, a.containers.create.inputs[0].Focus()
	case "d":
		a.screen = ScreenContainerStats
		a.containers.loading, a.containers.err, a.containers.statsPaused = true, nil, false
		return a, tea.Batch(a.spinner.Tick, a.loadContainerStatsDashboard())
	case "enter":
		if len(items) > 0 {
			a.containers.loading, a.containers.err, a.containers.notice = true, nil, ""
			a.screen = ScreenContainerDetail
			id := items[a.containers.cursor].ID
			return a, tea.Batch(a.spinner.Tick, a.loadContainerDetail(id))
		}
	case "esc", "q", "backspace":
		a.screen = ScreenMainMenu
	}
	return a, nil
}

func (a App) updateContainerDetail(key string) (tea.Model, tea.Cmd) {
	if a.containers.loading || a.containers.working {
		if key == "esc" && !a.containers.working {
			a.screen = ScreenContainers
		}
		return a, nil
	}
	switch key {
	case "n":
		a.containers.create = cloneContainerCreateForm(a.containers.detail)
		a.containers.err, a.containers.notice = nil, ""
		a.screen = ScreenContainerCreate
		return a, a.containers.create.inputs[0].Focus()
	case "left", "up":
		a.containers.actionCursor = (a.containers.actionCursor + 5) % 6
	case "right", "down", "tab":
		a.containers.actionCursor = (a.containers.actionCursor + 1) % 6
	case "r":
		a.containers.loading, a.containers.err = true, nil
		return a, tea.Batch(a.spinner.Tick, a.loadContainerDetail(a.containers.detail.ID))
	case "enter":
		if a.containers.actionCursor == 3 {
			return a.openContainerLogs()
		}
		if a.containers.actionCursor == 4 {
			if !a.containers.detail.Running {
				a.containers.notice = "Terminal is available only while the container is running."
				return a, nil
			}
			return a.openContainerTerminal()
		}
		if a.containers.actionCursor == 5 {
			a.screen, a.containers.deleteCursor, a.containers.err, a.containers.notice = ScreenContainerDelete, 0, nil, ""
			return a, nil
		}
		action := service.ContainerAction(a.containers.actionCursor)
		if (action == service.ContainerStart && a.containers.detail.Running) || (action != service.ContainerStart && !a.containers.detail.Running) {
			a.containers.notice = action.String() + " is unavailable for the current state"
			return a, nil
		}
		a.containers.working, a.containers.err = true, nil
		a.containers.notice = action.String() + "ing..."
		if action == service.ContainerStop {
			a.containers.notice = "Stopping..."
		}
		return a, tea.Batch(a.spinner.Tick, a.executeContainerAction(a.containers.detail.ID, action))
	case "esc", "q", "backspace":
		a.screen = ScreenContainers
	}
	return a, nil
}

func (a App) renderContainers() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Containers") + "\n\n")
	items := a.containers.visibleItems()
	status := "Filter: " + a.containers.filterLabel() + "  Sort: " + containerSorts[a.containers.sort]
	if a.containers.query != "" || a.containers.searching {
		status += "  Search: /" + a.containers.query
		if a.containers.searching {
			status += "_"
		}
	}
	b.WriteString(mutedStyle.Render(status+"  ("+strconv.Itoa(len(items))+"/"+strconv.Itoa(len(a.containers.items))+")") + "\n\n")
	if a.containers.loading {
		b.WriteString(a.spinner.View() + " Loading containers...\n")
	} else if a.containers.err != nil {
		b.WriteString(errorStyle.Render("Could not load containers") + "\n" + friendlyError(a.containers.err) + "\n")
	} else if len(items) == 0 {
		b.WriteString("No containers match the current search/filter.\n")
	} else {
		nameWidth, imageWidth := 22, 28
		if a.width > 0 && a.width < 90 {
			nameWidth, imageWidth = 16, 20
		}
		b.WriteString(fmt.Sprintf("  %-*s %-*s %-12s %s\n", nameWidth, "NAME", imageWidth, "IMAGE", "STATE", "STATUS"))
		maxRows := len(items)
		if a.height > 0 && maxRows > a.height-9 {
			maxRows = max(1, a.height-9)
		}
		start := 0
		if a.containers.cursor >= maxRows {
			start = a.containers.cursor - maxRows + 1
		}
		for i := start; i < min(len(items), start+maxRows); i++ {
			item := items[i]
			state := "○ " + displayState(item.State)
			if item.Running() {
				state = "● Running"
			}
			line := fmt.Sprintf("  %-*s %-*s %-12s %s", nameWidth, truncate(item.Name, nameWidth), imageWidth, truncate(item.Image, imageWidth), state, item.Status)
			if i == a.containers.cursor {
				line = selectedStyle.Render("> " + strings.TrimPrefix(line, "  "))
			}
			b.WriteString(line + "\n")
		}
	}
	if a.containers.notice != "" {
		b.WriteString("\n" + a.containers.notice + "\n")
	}
	b.WriteString("\n" + mutedStyle.Render("↑↓ Move   Enter Details   d Stats   c Create   r Refresh   Esc Back"))
	return b.String()
}

func (a App) renderContainerDetail() string {
	if a.containers.loading {
		return titleStyle.Render("Container Details") + "\n\n" + a.spinner.View() + " Loading details...\n\n" + mutedStyle.Render("Esc Back")
	}
	d := a.containers.detail
	var b strings.Builder
	b.WriteString(titleStyle.Render(d.Name) + "\n\n")
	state := "○ " + displayState(d.State)
	if d.Running {
		state = "● Running"
	}
	b.WriteString("Status:  " + state + "\nImage:   " + d.Image + "\nCreated: " + formatTime(d.Created) + "\n")
	if d.Running && !d.StartedAt.IsZero() {
		b.WriteString("Uptime:  " + formatDuration(time.Since(d.StartedAt)) + "\n")
	}
	if d.Running {
		if a.containers.statsLoading {
			b.WriteString("CPU:     Loading...\nMemory:  Loading...\n")
		} else if a.containers.statsErr != nil {
			b.WriteString("CPU:     Unavailable\nMemory:  Unavailable\n")
		} else {
			b.WriteString(fmt.Sprintf("CPU:     %.1f%%\nMemory:  %s / %s (%.1f%%)\n", a.containers.stats.CPUPercent, formatBytes(a.containers.stats.MemoryUsage), formatBytes(a.containers.stats.MemoryLimit), a.containers.stats.MemoryPercent))
		}
	}
	b.WriteString("\nPorts:\n")
	if len(d.Ports) == 0 {
		b.WriteString("  None\n")
	} else {
		for _, port := range d.Ports {
			b.WriteString("  " + port + "\n")
		}
	}
	b.WriteString("\nVolumes:\n")
	if len(d.Mounts) == 0 {
		b.WriteString("  None\n")
	} else {
		for _, mount := range d.Mounts {
			mode := ""
			if mount.ReadOnly {
				mode = " (read-only)"
			}
			b.WriteString("  " + mount.Source + " -> " + mount.Destination + mode + "\n")
		}
	}
	b.WriteString("\n")
	actions := []service.ContainerAction{service.ContainerStart, service.ContainerStop, service.ContainerRestart}
	for i, action := range actions {
		disabled := (action == service.ContainerStart && d.Running) || (action != service.ContainerStart && !d.Running)
		label := "[ " + action.String() + " ]"
		if disabled {
			label = mutedStyle.Render("[ " + action.String() + " unavailable ]")
		} else if i == a.containers.actionCursor {
			label = selectedStyle.Render(label)
		}
		b.WriteString(label + "  ")
	}
	logsLabel, terminalLabel, deleteLabel := "[ Logs ]", "[ Terminal ]", "[ Delete ]"
	if a.containers.actionCursor == 3 {
		logsLabel = selectedStyle.Render(logsLabel)
	}
	if !d.Running {
		terminalLabel = mutedStyle.Render("[ Terminal unavailable ]")
	} else if a.containers.actionCursor == 4 {
		terminalLabel = selectedStyle.Render(terminalLabel)
	}
	if a.containers.actionCursor == 5 {
		deleteLabel = selectedStyle.Render(deleteLabel)
	}
	b.WriteString(logsLabel + "  " + terminalLabel + "  " + deleteLabel)
	if a.containers.notice != "" {
		b.WriteString("\n\n" + a.containers.notice)
	}
	if a.containers.working {
		b.WriteString(" " + a.spinner.View())
	}
	if a.containers.err != nil {
		b.WriteString("\n" + errorStyle.Render(friendlyError(a.containers.err)))
	}
	b.WriteString("\n\n" + mutedStyle.Render("←→ Choose   Enter Run   n Clone   r Refresh   Esc Back"))
	return b.String()
}

func truncate(value string, width int) string {
	if utf8.RuneCountInString(value) <= width {
		return value
	}
	runes := []rune(value)
	if width <= 1 {
		return string(runes[:width])
	}
	return string(runes[:width-1]) + "…"
}
func titleCase(value string) string {
	r := []rune(value)
	if len(r) == 0 {
		return "Unknown"
	}
	return strings.ToUpper(string(r[0])) + string(r[1:])
}
func displayState(value string) string {
	if value == "exited" || value == "dead" || value == "created" {
		return "Stopped"
	}
	return titleCase(value)
}
func friendlyError(err error) string {
	if err == nil {
		return ""
	}
	text := strings.TrimSpace(err.Error())
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}
	lower := strings.ToLower(text)
	switch {
	case errors.Is(err, context.DeadlineExceeded) || strings.Contains(lower, "context deadline exceeded"):
		return "The operation timed out. Docker may be busy; try again."
	case strings.Contains(lower, "permission denied") || strings.Contains(lower, "access is denied"):
		return "Permission denied. Check whether this user can access Docker."
	case strings.Contains(lower, "connection refused") || strings.Contains(lower, "cannot connect") || strings.Contains(lower, "docker.sock") && strings.Contains(lower, "no such file"):
		return "Docker Engine could not be reached. Check that Docker is running and accessible."
	case strings.Contains(lower, "manifest unknown") || strings.Contains(lower, "pull access denied"):
		return "The image could not be downloaded. Check the image name, tag, and registry permissions."
	case strings.Contains(lower, "no such container") || strings.Contains(lower, "no such image") || strings.Contains(lower, "no such volume") || strings.Contains(lower, "not found"):
		return "The selected item no longer exists. Refresh the screen and try again."
	case strings.Contains(lower, "is being used") || strings.Contains(lower, "conflict"):
		return "Docker refused the operation because the item is still in use. " + truncate(text, 160)
	case strings.Contains(lower, "executable file not found") || strings.Contains(lower, "file not found") && strings.Contains(lower, "docker"):
		return "The Docker Compose command was not found. Install the Docker Compose plugin and try again."
	default:
		return truncate(text, 200)
	}
}
func formatTime(value time.Time) string {
	if value.IsZero() {
		return "Unknown"
	}
	return value.Local().Format("2006-01-02 15:04:05")
}
func formatDuration(value time.Duration) string {
	if value < 0 {
		return "Unknown"
	}
	d := value / (24 * time.Hour)
	value %= 24 * time.Hour
	h := value / time.Hour
	value %= time.Hour
	m := value / time.Minute
	if d > 0 {
		return fmt.Sprintf("%dd %dh %dm", d, h, m)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}

func formatBytes(value uint64) string {
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	div, exp := uint64(unit), 0
	for n := value / unit; n >= unit && exp < 3; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(value)/float64(div), "KMGT"[exp])
}
