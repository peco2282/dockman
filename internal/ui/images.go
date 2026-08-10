package ui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"dockman/internal/model"
)

type imageScreen struct {
	items         []model.Image
	cursor        int
	selected      model.Image
	detail        model.ImageDetail
	loading       bool
	working       bool
	err           error
	notice        string
	pullInput     textinput.Model
	progress      <-chan model.PullProgress
	progressLines []string
	pullDone      bool
	deleteCursor  int
}

func newImageScreen() imageScreen {
	input := textinput.New()
	input.Placeholder = "nginx:latest"
	input.CharLimit = 255
	input.SetWidth(50)
	return imageScreen{pullInput: input}
}

type imagesLoadedMsg struct {
	items []model.Image
	err   error
}
type imageDetailLoadedMsg struct {
	detail model.ImageDetail
	err    error
}
type imagePullOpenedMsg struct {
	progress <-chan model.PullProgress
	err      error
}
type imagePullProgressMsg struct{ progress model.PullProgress }
type imageDeleteResultMsg struct{ err error }

func (a App) loadImages() tea.Cmd {
	return func() tea.Msg {
		items, err := a.imagesService.List(context.Background())
		return imagesLoadedMsg{items, err}
	}
}
func (a App) loadImageDetail(id string) tea.Cmd {
	return func() tea.Msg {
		detail, err := a.imagesService.Detail(context.Background(), id)
		return imageDetailLoadedMsg{detail, err}
	}
}
func waitForPull(progress <-chan model.PullProgress) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-progress
		if !ok {
			return imagePullProgressMsg{model.PullProgress{Done: true}}
		}
		return imagePullProgressMsg{event}
	}
}

func (a App) updateImages(key string) (tea.Model, tea.Cmd) {
	if a.images.loading {
		if key == "esc" || key == "q" {
			a.screen = ScreenMainMenu
		}
		return a, nil
	}
	switch key {
	case "up", "k":
		if len(a.images.items) > 0 {
			a.images.cursor = (a.images.cursor - 1 + len(a.images.items)) % len(a.images.items)
		}
	case "down", "j", "tab":
		if len(a.images.items) > 0 {
			a.images.cursor = (a.images.cursor + 1) % len(a.images.items)
		}
	case "r":
		a.images.loading, a.images.err, a.images.notice = true, nil, ""
		return a, tea.Batch(a.spinner.Tick, a.loadImages())
	case "p":
		a.screen = ScreenImagePull
		a.images.err = nil
		a.images.pullDone = false
		a.images.progressLines = nil
		a.images.pullInput.SetValue("")
		return a, a.images.pullInput.Focus()
	case "enter":
		if len(a.images.items) > 0 {
			a.images.selected = a.images.items[a.images.cursor]
			a.images.loading, a.images.err = true, nil
			a.screen = ScreenImageDetail
			return a, tea.Batch(a.spinner.Tick, a.loadImageDetail(a.images.selected.ID))
		}
	case "esc", "q", "backspace":
		a.screen = ScreenMainMenu
	}
	return a, nil
}

func (a App) updateImageDetail(key string) (tea.Model, tea.Cmd) {
	if a.images.loading {
		return a, nil
	}
	switch key {
	case "d", "enter":
		a.images.deleteCursor = 0
		a.images.err = nil
		a.screen = ScreenImageDelete
	case "r":
		a.images.loading, a.images.err = true, nil
		return a, tea.Batch(a.spinner.Tick, a.loadImageDetail(a.images.detail.ID))
	case "esc", "q", "backspace":
		a.screen = ScreenImages
	}
	return a, nil
}

func (a App) updateImagePull(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if a.images.working {
		if key == "esc" || key == "q" {
			if a.pullCancel != nil {
				a.pullCancel()
				a.pullCancel = nil
			}
			a.images.working = false
			a.screen = ScreenImages
		}
		return a, nil
	}
	if a.images.pullDone {
		if key == "enter" || key == "esc" || key == "q" {
			a.images.loading = true
			a.screen = ScreenImages
			return a, tea.Batch(a.spinner.Tick, a.loadImages())
		}
		return a, nil
	}
	if key == "esc" {
		a.images.pullInput.Blur()
		a.screen = ScreenImages
		return a, nil
	}
	if key == "enter" {
		ctx, cancel := context.WithCancel(context.Background())
		a.pullCancel = cancel
		a.images.working = true
		a.images.err = nil
		a.images.progressLines = nil
		reference := a.images.pullInput.Value()
		return a, tea.Batch(a.spinner.Tick, func() tea.Msg {
			progress, err := a.imagesService.Pull(ctx, reference)
			return imagePullOpenedMsg{progress, err}
		})
	}
	var cmd tea.Cmd
	a.images.pullInput, cmd = a.images.pullInput.Update(msg)
	return a, cmd
}

func (a *App) appendPullProgress(status string) {
	if len(a.images.progressLines) > 0 && a.images.progressLines[len(a.images.progressLines)-1] == status {
		return
	}
	a.images.progressLines = append(a.images.progressLines, status)
	if len(a.images.progressLines) > 12 {
		a.images.progressLines = a.images.progressLines[len(a.images.progressLines)-12:]
	}
}

func (a App) updateImageDelete(key string) (tea.Model, tea.Cmd) {
	if a.images.working {
		return a, nil
	}
	switch key {
	case "left", "right", "up", "down", "tab":
		a.images.deleteCursor = 1 - a.images.deleteCursor
	case "esc", "q", "backspace":
		a.screen = ScreenImageDetail
	case "enter":
		if a.images.deleteCursor == 0 {
			a.screen = ScreenImageDetail
			return a, nil
		}
		a.images.working = true
		a.images.err = nil
		target := imageReference(a.images.selected)
		return a, tea.Batch(a.spinner.Tick, func() tea.Msg { return imageDeleteResultMsg{a.imagesService.Delete(context.Background(), target)} })
	}
	return a, nil
}

func (a App) renderImages() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Images") + "\n\n")
	if a.images.loading {
		b.WriteString(a.spinner.View() + " Loading images...\n")
	} else if a.images.err != nil {
		b.WriteString(errorStyle.Render("Could not load images") + "\n" + friendlyError(a.images.err) + "\n")
	} else if len(a.images.items) == 0 {
		b.WriteString("No images found. Press p to pull one.\n")
	} else {
		repoWidth := 36
		if a.width > 0 && a.width < 85 {
			repoWidth = 24
		}
		b.WriteString(fmt.Sprintf("  %-*s %-16s %-12s %s\n", repoWidth, "REPOSITORY", "TAG", "SIZE", "CONTAINERS"))
		maxRows := len(a.images.items)
		if a.height > 0 && maxRows > a.height-9 {
			maxRows = max(1, a.height-9)
		}
		start := 0
		if a.images.cursor >= maxRows {
			start = a.images.cursor - maxRows + 1
		}
		for i := start; i < min(len(a.images.items), start+maxRows); i++ {
			item := a.images.items[i]
			usedBy := "Unknown"
			if item.Containers >= 0 {
				usedBy = fmt.Sprintf("%d", item.Containers)
			}
			line := fmt.Sprintf("  %-*s %-16s %-12s %s", repoWidth, truncate(item.Repository, repoWidth), truncate(item.Tag, 16), formatSignedBytes(item.Size), usedBy)
			if i == a.images.cursor {
				line = selectedStyle.Render("> " + strings.TrimPrefix(line, "  "))
			}
			b.WriteString(line + "\n")
		}
	}
	if a.images.notice != "" {
		b.WriteString("\n" + a.images.notice + "\n")
	}
	b.WriteString("\n" + mutedStyle.Render("↑↓ Move   Enter Details   p Pull   r Refresh   Esc Back"))
	return b.String()
}

func (a App) renderImageDetail() string {
	if a.images.loading {
		return titleStyle.Render("Image Details") + "\n\n" + a.spinner.View() + " Loading details..."
	}
	d := a.images.detail
	var b strings.Builder
	b.WriteString(titleStyle.Render("Image Details") + "\n\nID:      " + shortID(d.ID) + "\nSize:    " + formatSignedBytes(d.Size) + "\nCreated: " + formatTime(d.Created) + "\nSystem:  " + d.OS + " / " + d.Architecture + fmt.Sprintf("\nLayers:  %d\n", d.Layers))
	if d.Author != "" {
		b.WriteString("Author:  " + d.Author + "\n")
	}
	b.WriteString("\nTags:\n")
	if len(d.RepoTags) == 0 {
		b.WriteString("  None (untagged)\n")
	} else {
		for _, tag := range d.RepoTags {
			b.WriteString("  " + tag + "\n")
		}
	}
	b.WriteString("\n")
	label := "[ Delete ]"
	label = selectedStyle.Render(label)
	b.WriteString(label)
	if a.images.err != nil {
		b.WriteString("\n\n" + errorStyle.Render(friendlyError(a.images.err)))
	}
	b.WriteString("\n\n" + mutedStyle.Render("Enter/d Delete   r Refresh   Esc Back"))
	return b.String()
}

func (a App) renderImagePull() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Pull Image") + "\n\nImage name and optional tag:\n" + a.images.pullInput.View() + "\n\nExample: nginx:latest\n")
	if a.images.working {
		b.WriteString("\n" + a.spinner.View() + " Pulling...\n")
		for _, line := range a.images.progressLines {
			b.WriteString("  " + truncate(line, max(20, a.width-6)) + "\n")
		}
	}
	if a.images.err != nil {
		b.WriteString("\n" + errorStyle.Render("Pull failed") + "\n" + friendlyError(a.images.err) + "\n")
	}
	if a.images.pullDone {
		b.WriteString("\n" + selectedStyle.Render("Pull completed successfully.") + "\nPress Enter to return to Images.\n")
	}
	b.WriteString("\n" + mutedStyle.Render("Enter Pull   Esc Back/Cancel"))
	return b.String()
}

func (a App) renderImageDelete() string {
	target := imageReference(a.images.selected)
	var b strings.Builder
	b.WriteString(errorStyle.Render(fmt.Sprintf("Delete image %q?", target)) + "\n\nContainers created from this image will not be deleted.\nIf a container uses this image, Docker will refuse the deletion.\n\n")
	cancel, del := "[ Cancel ]", "[ Delete ]"
	if a.images.deleteCursor == 0 {
		cancel = selectedStyle.Render(cancel)
	} else {
		del = selectedStyle.Render(del)
	}
	b.WriteString(cancel + "  " + del)
	if a.images.working {
		b.WriteString("\n\n" + a.spinner.View() + " Deleting...")
	}
	if a.images.err != nil {
		b.WriteString("\n\n" + errorStyle.Render("Could not delete image") + "\n" + friendlyError(a.images.err))
	}
	b.WriteString("\n\n" + mutedStyle.Render("←→ Choose   Enter Confirm   Esc Cancel"))
	return b.String()
}

func imageReference(image model.Image) string {
	if image.Repository == "<none>" {
		return image.ID
	}
	return image.Repository + ":" + image.Tag
}
func shortID(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
func formatSignedBytes(value int64) string {
	if value < 0 {
		return "Unknown"
	}
	return formatBytes(uint64(value))
}
