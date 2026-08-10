package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"dockman/internal/model"
	"dockman/internal/service"
)

type volumeScreen struct {
	items       []model.Volume
	cursor      int
	detail      model.Volume
	loading     bool
	working     bool
	err         error
	notice      string
	createInput textinput.Model
	deleteInput textinput.Model
}

func newVolumeScreen() volumeScreen {
	create := textinput.New()
	create.Placeholder = "minecraft-data"
	create.CharLimit = 255
	create.SetWidth(50)
	confirm := textinput.New()
	confirm.CharLimit = 255
	confirm.SetWidth(50)
	return volumeScreen{createInput: create, deleteInput: confirm}
}

type volumesLoadedMsg struct {
	items []model.Volume
	err   error
}
type volumeDetailLoadedMsg struct {
	volume model.Volume
	err    error
}
type volumeCreateResultMsg struct {
	volume model.Volume
	err    error
}
type volumeDeleteResultMsg struct{ err error }

func (a App) loadVolumes() tea.Cmd {
	return func() tea.Msg {
		items, err := a.volumesService.List(context.Background())
		return volumesLoadedMsg{items, err}
	}
}
func (a App) loadVolumeDetail(name string) tea.Cmd {
	return func() tea.Msg {
		volume, err := a.volumesService.Detail(context.Background(), name)
		return volumeDetailLoadedMsg{volume, err}
	}
}

func (a App) updateVolumes(key string) (tea.Model, tea.Cmd) {
	if a.volumes.loading {
		if key == "esc" || key == "q" {
			a.screen = ScreenMainMenu
		}
		return a, nil
	}
	switch key {
	case "up", "k":
		if len(a.volumes.items) > 0 {
			a.volumes.cursor = (a.volumes.cursor - 1 + len(a.volumes.items)) % len(a.volumes.items)
		}
	case "down", "j", "tab":
		if len(a.volumes.items) > 0 {
			a.volumes.cursor = (a.volumes.cursor + 1) % len(a.volumes.items)
		}
	case "c":
		a.volumes.createInput.SetValue("")
		a.volumes.err = nil
		a.screen = ScreenVolumeCreate
		return a, a.volumes.createInput.Focus()
	case "r":
		a.volumes.loading, a.volumes.err, a.volumes.notice = true, nil, ""
		return a, tea.Batch(a.spinner.Tick, a.loadVolumes())
	case "enter":
		if len(a.volumes.items) > 0 {
			a.volumes.loading, a.volumes.err = true, nil
			a.screen = ScreenVolumeDetail
			return a, tea.Batch(a.spinner.Tick, a.loadVolumeDetail(a.volumes.items[a.volumes.cursor].Name))
		}
	case "esc", "q", "backspace":
		a.screen = ScreenMainMenu
	}
	return a, nil
}

func (a App) updateVolumeDetail(key string) (tea.Model, tea.Cmd) {
	if a.volumes.loading {
		return a, nil
	}
	switch key {
	case "d", "enter":
		if a.volumes.detail.InUse {
			a.volumes.notice = "Delete unavailable: this volume is used by a container."
			return a, nil
		}
		a.volumes.deleteInput.SetValue("")
		a.volumes.err = nil
		a.screen = ScreenVolumeDelete
		return a, a.volumes.deleteInput.Focus()
	case "r":
		a.volumes.loading, a.volumes.err = true, nil
		return a, tea.Batch(a.spinner.Tick, a.loadVolumeDetail(a.volumes.detail.Name))
	case "esc", "q", "backspace":
		a.screen = ScreenVolumes
	}
	return a, nil
}

func (a App) updateVolumeCreate(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if a.volumes.working {
		return a, nil
	}
	if key == "esc" {
		a.volumes.createInput.Blur()
		a.screen = ScreenVolumes
		return a, nil
	}
	if key == "enter" {
		a.volumes.working = true
		a.volumes.err = nil
		name := a.volumes.createInput.Value()
		return a, tea.Batch(a.spinner.Tick, func() tea.Msg {
			volume, err := a.volumesService.Create(context.Background(), name)
			return volumeCreateResultMsg{volume, err}
		})
	}
	var cmd tea.Cmd
	a.volumes.createInput, cmd = a.volumes.createInput.Update(msg)
	return a, cmd
}

func (a App) updateVolumeDelete(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if a.volumes.working {
		return a, nil
	}
	if key == "esc" {
		a.volumes.deleteInput.Blur()
		a.screen = ScreenVolumeDetail
		return a, nil
	}
	if key == "enter" {
		a.volumes.working = true
		a.volumes.err = nil
		name, confirmation := a.volumes.detail.Name, a.volumes.deleteInput.Value()
		return a, tea.Batch(a.spinner.Tick, func() tea.Msg {
			return volumeDeleteResultMsg{a.volumesService.Delete(context.Background(), name, confirmation)}
		})
	}
	var cmd tea.Cmd
	a.volumes.deleteInput, cmd = a.volumes.deleteInput.Update(msg)
	return a, cmd
}

func (a App) renderVolumes() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Volumes") + "\n\n")
	if a.volumes.loading {
		b.WriteString(a.spinner.View() + " Loading volumes...\n")
	} else if a.volumes.err != nil {
		b.WriteString(errorStyle.Render("Could not load volumes") + "\n" + friendlyError(a.volumes.err) + "\n")
	} else if len(a.volumes.items) == 0 {
		b.WriteString("No volumes found. Press c to create one.\n")
	} else {
		nameWidth := 36
		if a.width > 0 && a.width < 80 {
			nameWidth = 24
		}
		b.WriteString(fmt.Sprintf("  %-*s %-14s %s\n", nameWidth, "NAME", "DRIVER", "USAGE"))
		maxRows := len(a.volumes.items)
		if a.height > 0 && maxRows > a.height-9 {
			maxRows = max(1, a.height-9)
		}
		start := 0
		if a.volumes.cursor >= maxRows {
			start = a.volumes.cursor - maxRows + 1
		}
		for i := start; i < min(len(a.volumes.items), start+maxRows); i++ {
			item := a.volumes.items[i]
			usage := "○ Not in use"
			if item.InUse {
				usage = "● In use"
			}
			line := fmt.Sprintf("  %-*s %-14s %s", nameWidth, truncate(item.Name, nameWidth), truncate(item.Driver, 14), usage)
			if i == a.volumes.cursor {
				line = selectedStyle.Render("> " + strings.TrimPrefix(line, "  "))
			}
			b.WriteString(line + "\n")
		}
	}
	if a.volumes.notice != "" {
		b.WriteString("\n" + a.volumes.notice + "\n")
	}
	b.WriteString("\n" + mutedStyle.Render("↑↓ Move   Enter Details   c Create   r Refresh   Esc Back"))
	return b.String()
}

func (a App) renderVolumeDetail() string {
	if a.volumes.loading {
		return titleStyle.Render("Volume Details") + "\n\n" + a.spinner.View() + " Loading details..."
	}
	v := a.volumes.detail
	var b strings.Builder
	b.WriteString(titleStyle.Render(v.Name) + "\n\nDriver:     " + v.Driver + "\nScope:      " + v.Scope + "\nCreated:    " + formatTime(v.Created) + "\nMountpoint: " + v.Mountpoint + "\nUsage:      ")
	if v.InUse {
		b.WriteString("● In use\n\nUsed by containers:\n")
		for _, name := range v.Containers {
			b.WriteString("  " + name + "\n")
		}
	} else {
		b.WriteString("○ Not in use\n")
	}
	writeStringMap(&b, "Labels", v.Labels)
	writeStringMap(&b, "Driver options", v.Options)
	b.WriteString("\n")
	if v.InUse {
		b.WriteString(mutedStyle.Render("[ Delete unavailable: volume is in use ]"))
	} else {
		b.WriteString(selectedStyle.Render("[ Delete ]"))
	}
	if a.volumes.notice != "" {
		b.WriteString("\n\n" + a.volumes.notice)
	}
	if a.volumes.err != nil {
		b.WriteString("\n" + errorStyle.Render(friendlyError(a.volumes.err)))
	}
	b.WriteString("\n\n" + mutedStyle.Render("Enter/d Delete   r Refresh   Esc Back"))
	return b.String()
}

func (a App) renderVolumeCreate() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Create Volume") + "\n\nName:\n" + a.volumes.createInput.View() + "\n\nDriver: local\n")
	if a.volumes.working {
		b.WriteString("\n" + a.spinner.View() + " Creating...")
	}
	if a.volumes.err != nil {
		b.WriteString("\n\n" + errorStyle.Render("Could not create volume") + "\n" + friendlyError(a.volumes.err))
	}
	b.WriteString("\n\n" + mutedStyle.Render("Enter Create   Esc Cancel"))
	return b.String()
}

func (a App) renderVolumeDelete() string {
	name := a.volumes.detail.Name
	confirmation := service.VolumeDeleteConfirmation(name)
	var b strings.Builder
	b.WriteString(errorStyle.Render(fmt.Sprintf("Delete volume %q?", name)) + "\n\nThis permanently deletes data stored in the volume.\nThis cannot be undone.\n\n")
	if confirmation == name {
		b.WriteString("Type the exact volume name to confirm:\n")
	} else {
		b.WriteString("This looks like an automatically generated volume name.\nType its last 8 characters to confirm: " + selectedStyle.Render(confirmation) + "\n")
	}
	b.WriteString(a.volumes.deleteInput.View())
	match := a.volumes.deleteInput.Value() == confirmation
	if match {
		b.WriteString("\n" + selectedStyle.Render("Name matches. Press Enter to delete."))
	} else {
		b.WriteString("\n" + mutedStyle.Render("The entered name must match exactly."))
	}
	if a.volumes.working {
		b.WriteString("\n\n" + a.spinner.View() + " Deleting...")
	}
	if a.volumes.err != nil {
		b.WriteString("\n\n" + errorStyle.Render("Could not delete volume") + "\n" + friendlyError(a.volumes.err))
	}
	b.WriteString("\n\n" + mutedStyle.Render("Enter Confirm   Esc Cancel"))
	return b.String()
}

func writeStringMap(b *strings.Builder, title string, values map[string]string) {
	if len(values) == 0 {
		return
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	b.WriteString("\n" + title + ":\n")
	for _, key := range keys {
		b.WriteString("  " + key + " = " + values[key] + "\n")
	}
}
