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

const containerCreateInputCount = 18
const containerCreateFocusCount = 22

type containerCreateForm struct {
	inputs       [containerCreateInputCount]textinput.Model
	focus        int
	restart      int
	start        bool
	privileged   bool
	imageChoices []model.Image
	imageCursor  int
	imageLoading bool
}

var restartPolicies = []struct{ label, value string }{{"No automatic restart", "no"}, {"Unless Stopped (recommended)", "unless-stopped"}, {"Always", "always"}, {"On Failure", "on-failure"}}

func newContainerScreen() containerScreen { return containerScreen{create: newContainerCreateForm()} }
func newContainerCreateForm() containerCreateForm {
	placeholders := []string{"minecraft", "itzg/minecraft-server:latest", "25565:25565, 8080:80", "/opt/minecraft:/data, minecraft-data:/config", "EULA=TRUE, TZ=Asia/Tokyo", "java -Xmx2G -jar server.jar", "/entrypoint.sh", "app-network", "minecraft", "/data", "1000:1000", "team=games, env=prod", "2", "2g", "NET_ADMIN, SYS_TIME", "/dev/ttyUSB0:/dev/ttyUSB0:rwm", "curl -f http://localhost/health || exit 1", "30s"}
	var form containerCreateForm
	form.restart = 1
	form.start = true
	for i := range form.inputs {
		form.inputs[i] = textinput.New()
		form.inputs[i].Placeholder = placeholders[i]
		form.inputs[i].CharLimit = 1024
		form.inputs[i].SetWidth(60)
	}
	return form
}

func cloneContainerCreateForm(detail model.ContainerDetail) containerCreateForm {
	form := newContainerCreateForm()
	form.inputs[0].SetValue(detail.Name + "-copy")
	form.inputs[1].SetValue(detail.Image)
	ports := make([]string, 0, len(detail.Ports))
	for _, port := range detail.Ports {
		if parts := strings.SplitN(port, " -> ", 2); len(parts) == 2 {
			ports = append(ports, parts[0]+":"+parts[1])
		}
	}
	form.inputs[2].SetValue(strings.Join(ports, ", "))
	mounts := make([]string, 0, len(detail.Mounts))
	for _, mount := range detail.Mounts {
		value := mount.Source + ":" + mount.Destination
		if mount.ReadOnly {
			value += ":ro"
		}
		mounts = append(mounts, value)
	}
	form.inputs[3].SetValue(strings.Join(mounts, ", "))
	return form
}

type containerCreateResultMsg struct {
	id  string
	err error
}
type containerCreateImagesLoadedMsg struct {
	items []model.Image
	err   error
}

func (a App) updateContainerCreate(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	form := &a.containers.create
	key := msg.String()
	if a.containers.working {
		return a, nil
	}
	if key == "esc" {
		form.blurAll()
		a.screen = ScreenContainers
		return a, nil
	}
	if key == "f2" && form.focus == 1 {
		form.blurAll()
		form.imageLoading = true
		a.containers.err = nil
		a.screen = ScreenContainerImagePicker
		return a, tea.Batch(a.spinner.Tick, func() tea.Msg {
			items, err := a.imagesService.List(context.Background())
			return containerCreateImagesLoadedMsg{items, err}
		})
	}
	if key == "tab" || key == "down" {
		return a.moveContainerCreateFocus(1)
	}
	if key == "shift+tab" || key == "up" {
		return a.moveContainerCreateFocus(-1)
	}
	if form.focus == 18 && (key == "left" || key == "right") {
		delta := 1
		if key == "left" {
			delta = -1
		}
		form.restart = (form.restart + delta + len(restartPolicies)) % len(restartPolicies)
		return a, nil
	}
	if form.focus == 19 && (key == "left" || key == "right" || key == " ") {
		form.start = !form.start
		return a, nil
	}
	if form.focus == 20 && (key == "left" || key == "right" || key == " ") {
		form.privileged = !form.privileged
		return a, nil
	}
	if key == "enter" {
		if form.focus < 21 {
			return a.moveContainerCreateFocus(1)
		}
		form.blurAll()
		a.containers.err = nil
		a.screen = ScreenContainerCreatePreview
		return a, nil
	}
	if form.focus < containerCreateInputCount {
		var cmd tea.Cmd
		form.inputs[form.focus], cmd = form.inputs[form.focus].Update(msg)
		return a, cmd
	}
	return a, nil
}

func (a App) moveContainerCreateFocus(delta int) (tea.Model, tea.Cmd) {
	form := &a.containers.create
	form.blurAll()
	form.focus = (form.focus + delta + containerCreateFocusCount) % containerCreateFocusCount
	if form.focus < containerCreateInputCount {
		return a, form.inputs[form.focus].Focus()
	}
	return a, nil
}

func (f containerCreateForm) containerCreateInput() service.ContainerCreateInput {
	return service.ContainerCreateInput{Name: f.inputs[0].Value(), Image: f.inputs[1].Value(), Ports: f.inputs[2].Value(), Mounts: f.inputs[3].Value(), Environment: f.inputs[4].Value(), Command: f.inputs[5].Value(), Entrypoint: f.inputs[6].Value(), Network: f.inputs[7].Value(), Hostname: f.inputs[8].Value(), WorkingDir: f.inputs[9].Value(), User: f.inputs[10].Value(), Labels: f.inputs[11].Value(), CPUs: f.inputs[12].Value(), Memory: f.inputs[13].Value(), Capabilities: f.inputs[14].Value(), Devices: f.inputs[15].Value(), HealthCommand: f.inputs[16].Value(), HealthInterval: f.inputs[17].Value(), RestartPolicy: restartPolicies[f.restart].value, Start: f.start, Privileged: f.privileged}
}
func (f *containerCreateForm) blurAll() {
	for i := range f.inputs {
		f.inputs[i].Blur()
	}
}

func (a App) renderContainerCreate() string {
	form := a.containers.create
	labels := []string{"Name", "Image", "Ports (host:container, comma-separated)", "Volumes / Bind mounts (source:target)", "Environment (NAME=VALUE, comma-separated)", "Command", "Entrypoint", "Network", "Hostname", "Working directory", "User", "Labels (NAME=VALUE, comma-separated)", "CPU limit (cores)", "Memory limit (for example 512m or 2g)", "Capabilities", "Devices (host:container:permissions)", "Healthcheck command", "Healthcheck interval"}
	var b strings.Builder
	b.WriteString(titleStyle.Render("Create Container") + "\n\n")
	start, end := 0, len(labels)
	maxFields := 7
	if end > maxFields {
		if form.focus < containerCreateInputCount {
			start = max(0, form.focus-maxFields/2)
		}
		if start+maxFields > end {
			start = end - maxFields
		}
		end = start + maxFields
		b.WriteString(mutedStyle.Render(fmt.Sprintf("Fields %d-%d of %d", start+1, end, len(labels))) + "\n")
	}
	for i := start; i < end; i++ {
		label := labels[i]
		prefix := "  "
		if form.focus == i {
			prefix = "> "
		}
		b.WriteString(prefix + label + ":\n  " + form.inputs[i].View() + "\n")
	}
	prefix := "  "
	if form.focus == 18 {
		prefix = "> "
	}
	b.WriteString(prefix + "Restart: " + restartPolicies[form.restart].label + "\n\n")
	prefix = "  "
	if form.focus == 19 {
		prefix = "> "
	}
	startLabel := "No"
	if form.start {
		startLabel = "Yes"
	}
	b.WriteString(prefix + "Start after create: " + startLabel + "\n")
	prefix = "  "
	if form.focus == 20 {
		prefix = "> "
	}
	privilegedLabel := "No"
	if form.privileged {
		privilegedLabel = "Yes"
	}
	b.WriteString(prefix + "Privileged: " + privilegedLabel + "\n")
	preview := "[ Review & Create ]"
	if form.focus == 21 {
		preview = selectedStyle.Render(preview)
	}
	b.WriteString("\n" + preview + "\n")
	if a.containers.err != nil {
		b.WriteString("\n\n" + errorStyle.Render("Could not create container") + "\n" + friendlyError(a.containers.err))
	}
	b.WriteString("\n\n" + mutedStyle.Render("Tab/↑↓ Next field   ←→ Change option   F2 Choose local image   Enter Continue   Esc Cancel"))
	return b.String()
}

func (a App) updateContainerImagePicker(key string) (tea.Model, tea.Cmd) {
	form := &a.containers.create
	if form.imageLoading {
		if key == "esc" {
			a.screen = ScreenContainerCreate
			return a, form.inputs[1].Focus()
		}
		return a, nil
	}
	switch key {
	case "up", "k":
		if len(form.imageChoices) > 0 {
			form.imageCursor = (form.imageCursor - 1 + len(form.imageChoices)) % len(form.imageChoices)
		}
	case "down", "j", "tab":
		if len(form.imageChoices) > 0 {
			form.imageCursor = (form.imageCursor + 1) % len(form.imageChoices)
		}
	case "enter":
		if len(form.imageChoices) > 0 {
			form.inputs[1].SetValue(imageReference(form.imageChoices[form.imageCursor]))
			a.screen = ScreenContainerCreate
			return a, form.inputs[1].Focus()
		}
	case "esc", "q", "backspace":
		a.screen = ScreenContainerCreate
		return a, form.inputs[1].Focus()
	}
	return a, nil
}

func (a App) renderContainerImagePicker() string {
	form := a.containers.create
	var b strings.Builder
	b.WriteString(titleStyle.Render("Choose Local Image") + "\n\n")
	if form.imageLoading {
		b.WriteString(a.spinner.View() + " Loading images...\n")
	} else if a.containers.err != nil {
		b.WriteString(errorStyle.Render(friendlyError(a.containers.err)) + "\n")
	} else if len(form.imageChoices) == 0 {
		b.WriteString("No local images found. Return and enter an image name manually.\n")
	} else {
		b.WriteString(fmt.Sprintf("  %-38s %-16s %s\n", "REPOSITORY", "TAG", "SIZE"))
		maxRows := len(form.imageChoices)
		if a.height > 0 && maxRows > a.height-8 {
			maxRows = max(1, a.height-8)
		}
		start := 0
		if form.imageCursor >= maxRows {
			start = form.imageCursor - maxRows + 1
		}
		for i := start; i < min(len(form.imageChoices), start+maxRows); i++ {
			item := form.imageChoices[i]
			line := fmt.Sprintf("  %-38s %-16s %s", truncate(item.Repository, 38), truncate(item.Tag, 16), formatSignedBytes(item.Size))
			if i == form.imageCursor {
				line = selectedStyle.Render("> " + strings.TrimPrefix(line, "  "))
			}
			b.WriteString(line + "\n")
		}
	}
	b.WriteString("\n" + mutedStyle.Render("↑/↓ Move   Enter Select   Esc Back to manual entry"))
	return b.String()
}

func (a App) updateContainerCreatePreview(key string) (tea.Model, tea.Cmd) {
	if a.containers.working {
		return a, nil
	}
	if key == "esc" || key == "q" || key == "backspace" || key == "e" {
		a.screen = ScreenContainerCreate
		return a, nil
	}
	if key == "enter" || key == "c" {
		input := a.containers.create.containerCreateInput()
		a.containers.working = true
		a.containers.err = nil
		return a, tea.Batch(a.spinner.Tick, func() tea.Msg {
			id, err := a.containersService.Create(context.Background(), input)
			return containerCreateResultMsg{id, err}
		})
	}
	return a, nil
}

func (a App) renderContainerCreatePreview() string {
	i := a.containers.create.containerCreateInput()
	value := func(v string) string {
		if strings.TrimSpace(v) == "" {
			return mutedStyle.Render("(default)")
		}
		return v
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render("Review Container") + "\n\n")
	b.WriteString("Name:         " + value(i.Name) + "\nImage:        " + value(i.Image) + "\nCommand:      " + value(i.Command) + "\nEntrypoint:   " + value(i.Entrypoint) + "\nNetwork:      " + value(i.Network) + "\nHostname:     " + value(i.Hostname) + "\nWorkdir:      " + value(i.WorkingDir) + "\nUser:         " + value(i.User) + "\nPorts:        " + value(i.Ports) + "\nMounts:       " + value(i.Mounts) + "\nEnvironment:  " + value(i.Environment) + "\nLabels:       " + value(i.Labels) + "\nCPU / RAM:    " + value(i.CPUs) + " / " + value(i.Memory) + "\nRestart:      " + i.RestartPolicy + fmt.Sprintf("\nStart:        %t\nPrivileged:   %t\n", i.Start, i.Privileged) + "Capabilities: " + value(i.Capabilities) + "\nDevices:      " + value(i.Devices) + "\nHealthcheck:  " + value(i.HealthCommand) + " (" + value(i.HealthInterval) + ")\n")
	if a.containers.working {
		b.WriteString("\n" + a.spinner.View() + " Creating...")
	}
	if a.containers.err != nil {
		b.WriteString("\n\n" + errorStyle.Render("Could not create container") + "\n" + friendlyError(a.containers.err))
	}
	b.WriteString("\n\n" + selectedStyle.Render("[ Create ]") + "\n\n" + mutedStyle.Render("Enter/c Create   e/Esc Edit"))
	return b.String()
}
