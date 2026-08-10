package ui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"dockman/internal/model"
	"dockman/internal/service"
)

type composeScreen struct {
	projects       []model.ComposeProject
	cursor         int
	selected       model.ComposeProject
	actionCursor   int
	loading        bool
	working        bool
	err            error
	notice         string
	logLines       []string
	logOffset      int
	logFollow      bool
	logDone        bool
	logEvents      <-chan service.LogEvent
	registerInputs []textinput.Model
	registerFocus  int
}

func newComposeScreen() composeScreen {
	placeholders := []string{"minecraft", "/opt/minecraft", "compose.yaml (optional)"}
	inputs := make([]textinput.Model, len(placeholders))
	for i, placeholder := range placeholders {
		inputs[i] = textinput.New()
		inputs[i].Placeholder = placeholder
		inputs[i].CharLimit = 512
	}
	return composeScreen{registerInputs: inputs}
}

type composeProjectsLoadedMsg struct {
	projects []model.ComposeProject
	err      error
}
type composeActionResultMsg struct {
	action service.ComposeAction
	err    error
}
type composeRegisterResultMsg struct {
	project model.ComposeProject
	err     error
}
type composeLogsOpenedMsg struct {
	events <-chan service.LogEvent
	err    error
}
type composeLogEventMsg struct{ event service.LogEvent }
type composeUnregisterResultMsg struct{ err error }

func (a App) loadComposeProjects() tea.Cmd {
	return func() tea.Msg {
		projects, err := a.composeService.List(context.Background())
		return composeProjectsLoadedMsg{projects, err}
	}
}

func (a App) updateComposeProjects(key string) (tea.Model, tea.Cmd) {
	if a.compose.loading {
		if key == "esc" || key == "q" {
			a.screen = ScreenMainMenu
		}
		return a, nil
	}
	switch key {
	case "up", "k":
		if len(a.compose.projects) > 0 {
			a.compose.cursor = (a.compose.cursor - 1 + len(a.compose.projects)) % len(a.compose.projects)
		}
	case "down", "j", "tab":
		if len(a.compose.projects) > 0 {
			a.compose.cursor = (a.compose.cursor + 1) % len(a.compose.projects)
		}
	case "r":
		a.compose.loading, a.compose.err, a.compose.notice = true, nil, ""
		return a, tea.Batch(a.spinner.Tick, a.loadComposeProjects())
	case "a":
		a.compose.err = nil
		a.compose.registerFocus = 0
		for i := range a.compose.registerInputs {
			a.compose.registerInputs[i].SetValue("")
			a.compose.registerInputs[i].Blur()
		}
		a.compose.registerInputs[0].Focus()
		a.screen = ScreenComposeRegister
	case "u":
		if len(a.compose.projects) > 0 && a.compose.projects[a.compose.cursor].Registered {
			project := a.compose.projects[a.compose.cursor]
			if manager, ok := a.composeService.(interface {
				Unregister(context.Context, string) error
			}); ok {
				a.compose.working = true
				return a, func() tea.Msg {
					return composeUnregisterResultMsg{manager.Unregister(context.Background(), project.Name)}
				}
			}
		}
	case "enter":
		if len(a.compose.projects) > 0 {
			a.compose.selected = a.compose.projects[a.compose.cursor]
			a.compose.actionCursor = 0
			a.compose.err = nil
			a.compose.notice = ""
			a.screen = ScreenComposeDetail
		}
	case "esc", "q", "backspace":
		a.screen = ScreenMainMenu
	}
	return a, nil
}

func (a App) updateComposeRegister(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if a.compose.working {
		return a, nil
	}
	if key == "esc" {
		a.screen = ScreenCompose
		return a, nil
	}
	if key == "tab" || key == "down" || key == "up" {
		delta := 1
		if key == "up" {
			delta = -1
		}
		a.compose.registerInputs[a.compose.registerFocus].Blur()
		a.compose.registerFocus = (a.compose.registerFocus + delta + len(a.compose.registerInputs)) % len(a.compose.registerInputs)
		return a, a.compose.registerInputs[a.compose.registerFocus].Focus()
	}
	if key == "enter" {
		if a.compose.registerFocus < len(a.compose.registerInputs)-1 {
			a.compose.registerInputs[a.compose.registerFocus].Blur()
			a.compose.registerFocus++
			return a, a.compose.registerInputs[a.compose.registerFocus].Focus()
		}
		a.compose.working, a.compose.err = true, nil
		input := service.ComposeRegisterInput{Name: a.compose.registerInputs[0].Value(), WorkingDir: a.compose.registerInputs[1].Value(), ConfigFiles: a.compose.registerInputs[2].Value()}
		return a, tea.Batch(a.spinner.Tick, func() tea.Msg {
			project, err := a.composeService.Register(context.Background(), input)
			return composeRegisterResultMsg{project: project, err: err}
		})
	}
	var cmd tea.Cmd
	a.compose.registerInputs[a.compose.registerFocus], cmd = a.compose.registerInputs[a.compose.registerFocus].Update(msg)
	return a, cmd
}

func (a App) updateComposeDetail(key string) (tea.Model, tea.Cmd) {
	if a.compose.working {
		return a, nil
	}
	switch key {
	case "left", "up":
		a.compose.actionCursor = (a.compose.actionCursor + 11) % 12
	case "right", "down", "tab":
		a.compose.actionCursor = (a.compose.actionCursor + 1) % 12
	case "r":
		a.compose.loading = true
		a.screen = ScreenCompose
		return a, tea.Batch(a.spinner.Tick, a.loadComposeProjects())
	case "enter":
		switch a.compose.actionCursor {
		case 0, 1, 2, 3, 6, 7, 8, 9, 10, 11:
			actionIndex := a.compose.actionCursor
			if actionIndex >= 6 {
				actionIndex -= 2
			}
			action := service.ComposeAction(actionIndex)
			if action == service.ComposeRestart && a.compose.selected.RunningCount() == 0 {
				a.compose.notice = "Restart unavailable: no project containers are running."
				return a, nil
			}
			a.compose.working = true
			a.compose.err = nil
			a.compose.notice = action.String() + " in progress..."
			project := a.compose.selected
			return a, tea.Batch(a.spinner.Tick, func() tea.Msg {
				return composeActionResultMsg{action, a.composeService.Execute(context.Background(), project, action)}
			})
		case 4:
			return a.openComposeLogs()
		case 5:
			a.screen = ScreenComposeContainers
		}
	case "esc", "q", "backspace":
		a.screen = ScreenCompose
	}
	return a, nil
}

func (a App) openComposeLogs() (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	a.composeCancel = cancel
	a.compose.loading = true
	a.compose.err = nil
	a.compose.logLines = nil
	a.compose.logOffset = 0
	a.compose.logFollow = true
	a.compose.logDone = false
	a.screen = ScreenComposeLogs
	project := a.compose.selected
	return a, tea.Batch(a.spinner.Tick, func() tea.Msg {
		events, err := a.composeService.FollowLogs(ctx, project)
		return composeLogsOpenedMsg{events, err}
	})
}
func waitForComposeLog(events <-chan service.LogEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-events
		if !ok {
			return composeLogEventMsg{service.LogEvent{Done: true}}
		}
		return composeLogEventMsg{event}
	}
}
func (a *App) appendComposeLog(line string) {
	const maxLines = 2000
	a.compose.logLines = append(a.compose.logLines, line)
	if len(a.compose.logLines) > maxLines {
		removed := len(a.compose.logLines) - maxLines
		a.compose.logLines = a.compose.logLines[removed:]
		a.compose.logOffset = max(0, a.compose.logOffset-removed)
	}
	if a.compose.logFollow {
		a.compose.logOffset = max(0, len(a.compose.logLines)-a.composeLogPageSize())
	}
}
func (a App) composeLogPageSize() int {
	if a.height > 8 {
		return a.height - 7
	}
	return 10
}
func (a App) updateComposeLogs(key string) (tea.Model, tea.Cmd) {
	page := a.composeLogPageSize()
	maxOffset := max(0, len(a.compose.logLines)-page)
	switch key {
	case "up", "k":
		a.compose.logFollow = false
		a.compose.logOffset = max(0, a.compose.logOffset-1)
	case "down", "j":
		a.compose.logOffset = min(maxOffset, a.compose.logOffset+1)
		a.compose.logFollow = a.compose.logOffset == maxOffset
	case "pgup":
		a.compose.logFollow = false
		a.compose.logOffset = max(0, a.compose.logOffset-page)
	case "pgdown":
		a.compose.logOffset = min(maxOffset, a.compose.logOffset+page)
		a.compose.logFollow = a.compose.logOffset == maxOffset
	case "end", "f":
		a.compose.logOffset = maxOffset
		a.compose.logFollow = true
	case "esc", "q", "backspace":
		if a.composeCancel != nil {
			a.composeCancel()
			a.composeCancel = nil
		}
		a.screen = ScreenComposeDetail
	}
	return a, nil
}
func (a App) updateComposeContainers(key string) (tea.Model, tea.Cmd) {
	if key == "esc" || key == "q" || key == "backspace" {
		a.screen = ScreenComposeDetail
	}
	return a, nil
}

func (a App) renderComposeProjects() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Compose Projects") + "\n\n")
	if a.compose.loading {
		b.WriteString(a.spinner.View() + " Loading Compose projects...\n")
	} else if a.compose.err != nil {
		b.WriteString(errorStyle.Render("Could not load Compose projects") + "\n" + friendlyError(a.compose.err) + "\n")
	} else if len(a.compose.projects) == 0 {
		b.WriteString("No Compose projects found.\n\nPress a to register a directory containing compose.yaml.\n")
	} else {
		b.WriteString("  PROJECT                         CONTAINERS   RUNNING   SOURCE\n")
		maxRows := len(a.compose.projects)
		if a.height > 0 && maxRows > a.height-9 {
			maxRows = max(1, a.height-9)
		}
		start := 0
		if a.compose.cursor >= maxRows {
			start = a.compose.cursor - maxRows + 1
		}
		for i := start; i < min(len(a.compose.projects), start+maxRows); i++ {
			project := a.compose.projects[i]
			source := "Detected"
			if project.Registered {
				source = "Saved"
			}
			line := fmt.Sprintf("  %-31s %-12d %-9d %s", truncate(project.Name, 31), len(project.Containers), project.RunningCount(), source)
			if i == a.compose.cursor {
				line = selectedStyle.Render("> " + strings.TrimPrefix(line, "  "))
			}
			b.WriteString(line + "\n")
		}
	}
	if a.compose.notice != "" {
		b.WriteString("\n" + a.compose.notice + "\n")
	}
	b.WriteString("\n" + mutedStyle.Render("↑↓ Move   Enter Details   a Register   u Unregister   r Refresh   Esc Back"))
	return b.String()
}

func (a App) renderComposeRegister() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Register Compose Project") + "\n\n")
	b.WriteString("Register a directory that already contains a Compose file.\nThe project remains available even while it is down.\n\n")
	labels := []string{"Project name", "Working directory", "Config files (comma-separated; blank = auto-detect)"}
	for i, input := range a.compose.registerInputs {
		marker := "  "
		if i == a.compose.registerFocus {
			marker = "> "
		}
		b.WriteString(marker + labels[i] + "\n  " + input.View() + "\n\n")
	}
	if a.compose.working {
		b.WriteString(a.spinner.View() + " Registering...\n")
	}
	if a.compose.err != nil {
		b.WriteString(errorStyle.Render(friendlyError(a.compose.err)) + "\n")
	}
	b.WriteString(mutedStyle.Render("Tab/↑↓ Field   Enter Next/Register   Esc Cancel"))
	return b.String()
}

func (a App) renderComposeDetail() string {
	p := a.compose.selected
	var b strings.Builder
	b.WriteString(titleStyle.Render("Compose - "+p.Name) + "\n\nContainers: " + fmt.Sprintf("%d total, %d running", len(p.Containers), p.RunningCount()) + "\nWorking directory: " + valueOrUnknown(p.WorkingDir) + "\nConfig files:\n")
	if len(p.ConfigFiles) == 0 {
		b.WriteString("  Unknown\n")
	} else {
		for _, file := range p.ConfigFiles {
			b.WriteString("  " + file + "\n")
		}
	}
	b.WriteString("\n")
	labels := []string{"Up", "Down", "Restart", "Pull", "Logs", "Containers", "Build", "Up+Build", "Stop", "Start", "Pause", "Down+Orphans"}
	for i, label := range labels {
		disabled := i == 2 && p.RunningCount() == 0
		text := "[ " + label + " ]"
		if disabled {
			text = mutedStyle.Render("[ " + label + " unavailable ]")
		} else if i == a.compose.actionCursor {
			text = selectedStyle.Render(text)
		}
		b.WriteString(text + "  ")
	}
	if a.compose.notice != "" {
		b.WriteString("\n\n" + a.compose.notice)
	}
	if a.compose.working {
		b.WriteString(" " + a.spinner.View())
	}
	if a.compose.err != nil {
		b.WriteString("\n" + errorStyle.Render(friendlyError(a.compose.err)))
	}
	b.WriteString("\n\n" + mutedStyle.Render("←→ Choose   Enter Run/Open   r Refresh   Esc Back"))
	return b.String()
}

func (a App) renderComposeLogs() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Compose Logs - "+a.compose.selected.Name) + "\n")
	status := "LIVE / following"
	if !a.compose.logFollow {
		status = "PAUSED / scroll mode"
	}
	if a.compose.logDone {
		status = "STREAM ENDED"
	}
	b.WriteString(mutedStyle.Render(status) + "\n\n")
	if a.compose.loading {
		b.WriteString(a.spinner.View() + " Opening Compose logs...\n")
	} else if a.compose.err != nil {
		b.WriteString(errorStyle.Render("Could not read Compose logs") + "\n" + friendlyError(a.compose.err) + "\n")
	} else if len(a.compose.logLines) == 0 {
		b.WriteString("No logs available yet.\n")
	} else {
		end := min(len(a.compose.logLines), a.compose.logOffset+a.composeLogPageSize())
		for _, line := range a.compose.logLines[a.compose.logOffset:end] {
			b.WriteString(truncate(line, max(20, a.width-4)) + "\n")
		}
	}
	b.WriteString("\n" + mutedStyle.Render("↑↓ Scroll   PgUp/PgDn Page   End/f Follow   Esc Back"))
	return b.String()
}

func (a App) renderComposeContainers() string {
	p := a.compose.selected
	var b strings.Builder
	b.WriteString(titleStyle.Render("Compose Containers - "+p.Name) + "\n\n")
	if len(p.Containers) == 0 {
		b.WriteString("No containers found.\n")
	} else {
		b.WriteString("  SERVICE              NAME                         STATE       STATUS\n")
		for _, container := range p.Containers {
			state := "○ " + displayState(container.State)
			if container.State == "running" {
				state = "● Running"
			}
			b.WriteString(fmt.Sprintf("  %-20s %-28s %-11s %s\n", truncate(container.Service, 20), truncate(container.Name, 28), state, container.Status))
		}
	}
	b.WriteString("\n" + mutedStyle.Render("Esc Back"))
	return b.String()
}
func valueOrUnknown(value string) string {
	if value == "" {
		return "Unknown"
	}
	return value
}
