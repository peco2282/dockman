package ui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"dockman/internal/model"
)

type networkScreen struct {
	items            []model.Network
	cursor           int
	detail           model.Network
	loading, working bool
	err              error
	notice           string
	nameInput        textinput.Model
	driverCursor     int
	containers       []model.Container
	containerCursor  int
	disconnectMode   bool
}

func newNetworkScreen() networkScreen {
	in := textinput.New()
	in.Placeholder = "app-network"
	in.CharLimit = 255
	in.SetWidth(50)
	return networkScreen{nameInput: in}
}
func (n *networkScreen) clampCursor() {
	if len(n.items) == 0 {
		n.cursor = 0
	} else if n.cursor >= len(n.items) {
		n.cursor = len(n.items) - 1
	}
}

type networksLoadedMsg struct {
	items []model.Network
	err   error
}
type networkDetailLoadedMsg struct {
	network model.Network
	err     error
}
type networkCreateResultMsg struct {
	network model.Network
	err     error
}
type networkActionResultMsg struct {
	notice  string
	deleted bool
	err     error
}

func (a App) loadNetworks() tea.Cmd {
	return func() tea.Msg { v, e := a.networksService.List(context.Background()); return networksLoadedMsg{v, e} }
}
func (a App) loadNetworkDetail(id string) tea.Cmd {
	return func() tea.Msg {
		v, e := a.networksService.Detail(context.Background(), id)
		return networkDetailLoadedMsg{v, e}
	}
}

func (a App) updateNetworks(key string) (tea.Model, tea.Cmd) {
	if a.networks.loading {
		if key == "esc" || key == "q" {
			a.screen = ScreenMainMenu
		}
		return a, nil
	}
	switch key {
	case "up", "k":
		if len(a.networks.items) > 0 {
			a.networks.cursor = (a.networks.cursor - 1 + len(a.networks.items)) % len(a.networks.items)
		}
	case "down", "j", "tab":
		if len(a.networks.items) > 0 {
			a.networks.cursor = (a.networks.cursor + 1) % len(a.networks.items)
		}
	case "c":
		a.networks.nameInput.SetValue("")
		a.networks.driverCursor = 0
		a.networks.err = nil
		a.screen = ScreenNetworkCreate
		return a, a.networks.nameInput.Focus()
	case "r":
		a.networks.loading = true
		a.networks.err = nil
		return a, tea.Batch(a.spinner.Tick, a.loadNetworks())
	case "enter":
		if len(a.networks.items) > 0 {
			a.networks.loading = true
			a.networks.err = nil
			a.screen = ScreenNetworkDetail
			return a, tea.Batch(a.spinner.Tick, a.loadNetworkDetail(a.networks.items[a.networks.cursor].ID))
		}
	case "esc", "q", "backspace":
		a.screen = ScreenMainMenu
	}
	return a, nil
}

func (a App) updateNetworkDetail(key string) (tea.Model, tea.Cmd) {
	if a.networks.loading || a.networks.working {
		return a, nil
	}
	switch key {
	case "c":
		items, err := a.containersService.List(context.Background())
		if err != nil {
			a.networks.err = err
			return a, nil
		}
		connected := map[string]bool{}
		for _, c := range a.networks.detail.Containers {
			connected[c.ID] = true
		}
		a.networks.containers = nil
		for _, c := range items {
			if !connected[c.ID] {
				a.networks.containers = append(a.networks.containers, c)
			}
		}
		a.networks.containerCursor = 0
		a.networks.disconnectMode = false
		a.screen = ScreenNetworkConnect
	case "x":
		if len(a.networks.detail.Containers) > 0 {
			a.networks.containerCursor = 0
			a.networks.disconnectMode = true
			a.screen = ScreenNetworkConnect
		}
	case "d":
		if len(a.networks.detail.Containers) == 0 {
			a.screen = ScreenNetworkDelete
		}
	case "r":
		a.networks.loading = true
		return a, tea.Batch(a.spinner.Tick, a.loadNetworkDetail(a.networks.detail.ID))
	case "esc", "q", "backspace":
		a.screen = ScreenNetworks
	}
	return a, nil
}

func (a App) updateNetworkCreate(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.networks.working {
		return a, nil
	}
	key := msg.String()
	if key == "esc" {
		a.screen = ScreenNetworks
		return a, nil
	}
	if key == "tab" || key == "up" || key == "down" {
		a.networks.driverCursor = 1 - a.networks.driverCursor
		return a, nil
	}
	if key == "enter" {
		name := a.networks.nameInput.Value()
		driver := []string{"bridge", "overlay"}[a.networks.driverCursor]
		a.networks.working = true
		a.networks.err = nil
		return a, tea.Batch(a.spinner.Tick, func() tea.Msg {
			n, e := a.networksService.Create(context.Background(), name, driver)
			return networkCreateResultMsg{n, e}
		})
	}
	var cmd tea.Cmd
	a.networks.nameInput, cmd = a.networks.nameInput.Update(msg)
	return a, cmd
}

func (a App) updateNetworkDelete(key string) (tea.Model, tea.Cmd) {
	if a.networks.working {
		return a, nil
	}
	if key == "esc" || key == "q" {
		a.screen = ScreenNetworkDetail
		return a, nil
	}
	if key == "enter" {
		id := a.networks.detail.ID
		a.networks.working = true
		return a, tea.Batch(a.spinner.Tick, func() tea.Msg {
			return networkActionResultMsg{notice: "Network deleted.", deleted: true, err: a.networksService.Delete(context.Background(), id)}
		})
	}
	return a, nil
}

func (a App) updateNetworkConnect(key string) (tea.Model, tea.Cmd) {
	if a.networks.working {
		return a, nil
	}
	disconnect := a.networks.disconnectMode
	count := len(a.networks.containers)
	if disconnect {
		count = len(a.networks.detail.Containers)
	}
	if key == "esc" || key == "q" {
		a.screen = ScreenNetworkDetail
		return a, nil
	}
	if (key == "up" || key == "k") && count > 0 {
		a.networks.containerCursor = (a.networks.containerCursor - 1 + count) % count
	}
	if (key == "down" || key == "j" || key == "tab") && count > 0 {
		a.networks.containerCursor = (a.networks.containerCursor + 1) % count
	}
	if key == "enter" && count > 0 {
		a.networks.working = true
		nid := a.networks.detail.ID
		if disconnect {
			cid := a.networks.detail.Containers[a.networks.containerCursor].ID
			return a, tea.Batch(a.spinner.Tick, func() tea.Msg {
				return networkActionResultMsg{notice: "Container disconnected.", err: a.networksService.Disconnect(context.Background(), nid, cid)}
			})
		}
		cid := a.networks.containers[a.networks.containerCursor].ID
		return a, tea.Batch(a.spinner.Tick, func() tea.Msg {
			return networkActionResultMsg{notice: "Container connected.", err: a.networksService.Connect(context.Background(), nid, cid)}
		})
	}
	return a, nil
}

func (a App) renderNetworks() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Networks") + "\n\n")
	if a.networks.loading {
		b.WriteString(a.spinner.View() + " Loading networks...\n")
	} else if a.networks.err != nil {
		b.WriteString(errorStyle.Render("Could not load networks") + "\n" + friendlyError(a.networks.err) + "\n")
	} else if len(a.networks.items) == 0 {
		b.WriteString("No networks found. Press c to create one.\n")
	} else {
		b.WriteString(fmt.Sprintf("  %-28s %-12s %-10s\n", "NAME", "DRIVER", "SCOPE"))
		for i, n := range a.networks.items {
			line := fmt.Sprintf("  %-28s %-12s %-10s", truncate(n.Name, 28), n.Driver, n.Scope)
			if i == a.networks.cursor {
				line = selectedStyle.Render("> " + strings.TrimPrefix(line, "  "))
			}
			b.WriteString(line + "\n")
		}
	}
	if a.networks.notice != "" {
		b.WriteString("\n" + a.networks.notice + "\n")
	}
	b.WriteString("\n" + mutedStyle.Render("↑/↓ Move   Enter Details   c Create   r Refresh   Esc Back"))
	return b.String()
}

func (a App) renderNetworkDetail() string {
	if a.networks.loading {
		return titleStyle.Render("Network Details") + "\n\n" + a.spinner.View() + " Loading details..."
	}
	n := a.networks.detail
	var b strings.Builder
	b.WriteString(titleStyle.Render(n.Name) + "\n\nID:         " + n.ID + "\nDriver:     " + n.Driver + "\nScope:      " + n.Scope + fmt.Sprintf("\nIPv4/IPv6:  %t / %t\nInternal:   %t\nAttachable: %t\n", n.IPv4, n.IPv6, n.Internal, n.Attachable))
	if n.IPAMDriver != "" {
		b.WriteString("IPAM:       " + n.IPAMDriver + "\n")
	}
	for _, s := range n.Subnets {
		b.WriteString("Subnet:     " + s.Subnet + "\nGateway:    " + s.Gateway + "\n")
		if s.IPRange != "" {
			b.WriteString("IP range:   " + s.IPRange + "\n")
		}
	}
	b.WriteString("\nConnected containers:\n")
	if len(n.Containers) == 0 {
		b.WriteString("  None\n")
	}
	for _, c := range n.Containers {
		b.WriteString(fmt.Sprintf("  %s  IPv4=%s  IPv6=%s  MAC=%s\n", c.Name, c.IPv4Address, c.IPv6Address, c.MACAddress))
	}
	writeStringMap(&b, "Labels", n.Labels)
	writeStringMap(&b, "Driver options", n.Options)
	if a.networks.notice != "" {
		b.WriteString("\n" + a.networks.notice)
	}
	if a.networks.err != nil {
		b.WriteString("\n" + errorStyle.Render(friendlyError(a.networks.err)))
	}
	b.WriteString("\n\n" + mutedStyle.Render("c Connect   x Disconnect   d Delete   r Refresh   Esc Back"))
	return b.String()
}

func (a App) renderNetworkCreate() string {
	drivers := []string{"bridge", "overlay"}
	d := drivers[a.networks.driverCursor]
	out := titleStyle.Render("Create Network") + "\n\nName:\n" + a.networks.nameInput.View() + "\n\nDriver: " + selectedStyle.Render(d)
	if a.networks.working {
		out += "\n\n" + a.spinner.View() + " Creating..."
	}
	if a.networks.err != nil {
		out += "\n\n" + errorStyle.Render(friendlyError(a.networks.err))
	}
	return out + "\n\n" + mutedStyle.Render("Tab Change driver   Enter Create   Esc Cancel")
}
func (a App) renderNetworkDelete() string {
	out := errorStyle.Render("Delete network \""+a.networks.detail.Name+"\"?") + "\n\nThis cannot be undone."
	if a.networks.working {
		out += "\n\n" + a.spinner.View() + " Deleting..."
	}
	if a.networks.err != nil {
		out += "\n\n" + errorStyle.Render(friendlyError(a.networks.err))
	}
	return out + "\n\n" + mutedStyle.Render("Enter Confirm   Esc Cancel")
}
func (a App) renderNetworkConnect() string {
	disconnect := a.networks.disconnectMode
	title := "Connect Container"
	var names []string
	if disconnect {
		title = "Disconnect Container"
		for _, c := range a.networks.detail.Containers {
			names = append(names, c.Name)
		}
	} else {
		for _, c := range a.networks.containers {
			names = append(names, c.Name)
		}
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render(title) + "\n\n")
	if len(names) == 0 {
		b.WriteString("No eligible containers.\n")
	}
	for i, name := range names {
		line := "  " + name
		if i == a.networks.containerCursor {
			line = selectedStyle.Render("> " + name)
		}
		b.WriteString(line + "\n")
	}
	if a.networks.working {
		b.WriteString("\n" + a.spinner.View() + " Working...")
	}
	return b.String() + "\n\n" + mutedStyle.Render("↑/↓ Move   Enter Confirm   Esc Cancel")
}
