package ui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"dockman/internal/model"
)

type systemScreen struct {
	info          model.SystemInfo
	loading       bool
	err           error
	offset        int
	cleanup       model.CleanupAnalysis
	selection     model.CleanupSelection
	cleanupCursor int
	notice        string
}
type systemInfoLoadedMsg struct {
	info model.SystemInfo
	err  error
}
type cleanupAnalyzedMsg struct {
	analysis model.CleanupAnalysis
	err      error
}
type cleanupPrunedMsg struct {
	result model.CleanupResult
	err    error
}

func (a App) loadSystemInfo() tea.Cmd {
	return func() tea.Msg {
		info, err := a.systemService.Info(context.Background())
		return systemInfoLoadedMsg{info, err}
	}
}
func (a App) updateSystem(key string) (tea.Model, tea.Cmd) {
	lines := systemLines(a.system.info)
	page := a.systemPageSize()
	maxOffset := max(0, len(lines)-page)
	switch key {
	case "up", "k":
		a.system.offset = max(0, a.system.offset-1)
	case "down", "j":
		a.system.offset = min(maxOffset, a.system.offset+1)
	case "pgup":
		a.system.offset = max(0, a.system.offset-page)
	case "pgdown":
		a.system.offset = min(maxOffset, a.system.offset+page)
	case "r":
		a.system.loading, a.system.err = true, nil
		return a, tea.Batch(a.spinner.Tick, a.loadSystemInfo())
	case "p":
		a.screen, a.system.loading, a.system.err, a.system.notice = ScreenCleanup, true, nil, ""
		return a, tea.Batch(a.spinner.Tick, a.loadCleanupAnalysis())
	case "esc", "q", "backspace":
		a.screen = ScreenMainMenu
	}
	return a, nil
}

func (a App) loadCleanupAnalysis() tea.Cmd {
	return func() tea.Msg {
		v, err := a.systemService.AnalyzeCleanup(context.Background())
		return cleanupAnalyzedMsg{v, err}
	}
}
func (a App) runCleanup() tea.Cmd {
	selection := a.system.selection
	return func() tea.Msg {
		v, err := a.systemService.Prune(context.Background(), selection)
		return cleanupPrunedMsg{v, err}
	}
}

func cleanupKinds() []model.CleanupKind {
	return []model.CleanupKind{model.CleanupContainers, model.CleanupImages, model.CleanupVolumes, model.CleanupNetworks, model.CleanupBuildCache}
}
func cleanupKindName(k model.CleanupKind) string {
	return []string{"Stopped containers", "Dangling images", "Unused volumes", "Unused networks", "Build cache"}[int(k)]
}
func selectionHas(s model.CleanupSelection, k model.CleanupKind) bool {
	switch k {
	case model.CleanupContainers:
		return s.Containers
	case model.CleanupImages:
		return s.Images
	case model.CleanupVolumes:
		return s.Volumes
	case model.CleanupNetworks:
		return s.Networks
	default:
		return s.BuildCache
	}
}
func selectionSet(s *model.CleanupSelection, k model.CleanupKind, v bool) {
	switch k {
	case model.CleanupContainers:
		s.Containers = v
	case model.CleanupImages:
		s.Images = v
	case model.CleanupVolumes:
		s.Volumes = v
	case model.CleanupNetworks:
		s.Networks = v
	default:
		s.BuildCache = v
	}
}
func countKind(a model.CleanupAnalysis, k model.CleanupKind) int {
	n := 0
	for _, item := range a.Items {
		if item.Kind == k {
			n++
		}
	}
	return n
}
func sizeKind(a model.CleanupAnalysis, k model.CleanupKind) int64 {
	var n int64
	for _, item := range a.Items {
		if item.Kind == k && item.Size > 0 {
			n += item.Size
		}
	}
	return n
}
func allAvailable(a model.CleanupAnalysis) model.CleanupSelection {
	var s model.CleanupSelection
	for _, k := range cleanupKinds() {
		selectionSet(&s, k, countKind(a, k) > 0)
	}
	return s
}
func selectedEstimate(a model.CleanupAnalysis, s model.CleanupSelection) int64 {
	var n int64
	for _, k := range cleanupKinds() {
		if selectionHas(s, k) {
			n += sizeKind(a, k)
		}
	}
	return n
}
func anySelected(s model.CleanupSelection) bool {
	for _, k := range cleanupKinds() {
		if selectionHas(s, k) {
			return true
		}
	}
	return false
}

func (a App) updateCleanup(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		a.system.cleanupCursor = max(0, a.system.cleanupCursor-1)
	case "down", "j":
		a.system.cleanupCursor = min(4, a.system.cleanupCursor+1)
	case " ":
		k := cleanupKinds()[a.system.cleanupCursor]
		if countKind(a.system.cleanup, k) > 0 {
			selectionSet(&a.system.selection, k, !selectionHas(a.system.selection, k))
		}
	case "enter":
		if anySelected(a.system.selection) {
			a.screen = ScreenCleanupConfirm
		}
	case "r":
		a.system.loading = true
		a.system.err = nil
		return a, tea.Batch(a.spinner.Tick, a.loadCleanupAnalysis())
	case "esc", "q", "backspace":
		a.screen = ScreenSystem
	}
	return a, nil
}
func (a App) updateCleanupConfirm(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "y":
		a.system.loading = true
		a.system.err = nil
		a.screen = ScreenCleanup
		return a, tea.Batch(a.spinner.Tick, a.runCleanup())
	case "n", "esc", "q", "backspace":
		a.screen = ScreenCleanup
	}
	return a, nil
}
func (a App) renderCleanup() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Docker Cleanup") + "\n\n")
	if a.system.loading {
		b.WriteString(a.spinner.View() + " Analyzing cleanup candidates...\n")
		return b.String()
	}
	if a.system.err != nil {
		b.WriteString(errorStyle.Render("Cleanup analysis failed") + "\n" + friendlyError(a.system.err) + "\n\n" + mutedStyle.Render("r Retry   Esc Back"))
		return b.String()
	}
	if a.system.notice != "" {
		b.WriteString(a.system.notice + "\n\n")
	}
	b.WriteString("Disk usage (docker system df equivalent)\n")
	b.WriteString(fmt.Sprintf("  Containers %-10s  Images %-10s  Volumes %-10s  Build cache %s\n\n", formatSignedBytes(a.system.cleanup.ContainersTotal), formatSignedBytes(a.system.cleanup.ImagesTotal), formatSignedBytes(a.system.cleanup.VolumesTotal), formatSignedBytes(a.system.cleanup.BuildCacheTotal)))
	b.WriteString("Cleanup candidates (Space toggles a category)\n")
	for i, k := range cleanupKinds() {
		mark := " "
		if selectionHas(a.system.selection, k) {
			mark = "x"
		}
		line := fmt.Sprintf("  [%s] %-20s %3d  estimated %s", mark, cleanupKindName(k), countKind(a.system.cleanup, k), formatSignedBytes(sizeKind(a.system.cleanup, k)))
		if i == a.system.cleanupCursor {
			line = selectedStyle.Render("> " + strings.TrimSpace(line))
		}
		b.WriteString(line + "\n")
	}
	b.WriteString(fmt.Sprintf("\nEstimated maximum reclaim: %s\n", formatSignedBytes(selectedEstimate(a.system.cleanup, a.system.selection))))
	b.WriteString(mutedStyle.Render("Estimate may differ because image layers can be shared."))
	if len(a.system.cleanup.Items) > 0 {
		b.WriteString("\n\nTargets:\n")
		limit := min(len(a.system.cleanup.Items), 12)
		for _, item := range a.system.cleanup.Items[:limit] {
			name := item.Name
			if name == "" {
				name = shortID(item.ID)
			}
			b.WriteString(fmt.Sprintf("  %-18s %-30s %s\n", cleanupKindName(item.Kind), truncate(name, 30), formatSignedBytes(item.Size)))
		}
		if len(a.system.cleanup.Items) > limit {
			b.WriteString(fmt.Sprintf("  ... and %d more\n", len(a.system.cleanup.Items)-limit))
		}
	}
	b.WriteString("\n" + mutedStyle.Render("↑/↓ Move   Space Toggle   Enter Review   r Refresh   Esc Back"))
	return b.String()
}
func (a App) renderCleanupConfirm() string {
	return titleStyle.Render("Confirm Docker Cleanup") + "\n\n" + errorStyle.Render("This permanently deletes the selected unused resources.") + "\n\n" + fmt.Sprintf("Selected estimate: %s\n", formatSignedBytes(selectedEstimate(a.system.cleanup, a.system.selection))) + "\nType y to prune, or n/Esc to cancel.\n\n" + mutedStyle.Render("y Prune   n Cancel")
}
func (a App) systemPageSize() int {
	if a.height > 8 {
		return a.height - 6
	}
	return 18
}
func (a App) renderSystem() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Docker System") + "\n\n")
	if a.system.loading {
		b.WriteString(a.spinner.View() + " Loading Docker system information...\n")
	} else if a.system.err != nil {
		b.WriteString(errorStyle.Render("Could not load system information") + "\n" + friendlyError(a.system.err) + "\n")
	} else {
		lines := systemLines(a.system.info)
		end := min(len(lines), a.system.offset+a.systemPageSize())
		for _, line := range lines[a.system.offset:end] {
			b.WriteString(line + "\n")
		}
		if len(lines) > a.systemPageSize() {
			b.WriteString(fmt.Sprintf("\nShowing %d-%d of %d\n", a.system.offset+1, end, len(lines)))
		}
	}
	b.WriteString("\n" + mutedStyle.Render("↑↓/PgUp/PgDn Scroll   p Cleanup   r Refresh   Esc Back"))
	return b.String()
}
func systemLines(info model.SystemInfo) []string {
	return []string{
		"Docker Engine", fmt.Sprintf("  Version:       %s", valueOrUnknown(info.DockerVersion)), fmt.Sprintf("  API Version:   %s (minimum %s)", valueOrUnknown(info.APIVersion), valueOrUnknown(info.MinAPIVersion)), fmt.Sprintf("  Platform:      %s", valueOrUnknown(info.Platform)), "",
		"Containers", fmt.Sprintf("  Total:         %d", info.Containers), fmt.Sprintf("  ● Running:     %d", info.Running), fmt.Sprintf("  ◐ Paused:      %d", info.Paused), fmt.Sprintf("  ○ Stopped:     %d", info.Stopped), fmt.Sprintf("  Images:        %d", info.Images), "",
		"Server", fmt.Sprintf("  Hostname:      %s", valueOrUnknown(info.HostName)), fmt.Sprintf("  OS:            %s", valueOrUnknown(info.OperatingSystem)), fmt.Sprintf("  Kernel:        %s", valueOrUnknown(info.KernelVersion)), fmt.Sprintf("  Architecture:  %s", valueOrUnknown(info.Architecture)), fmt.Sprintf("  CPU:           %d cores", info.CPUs), fmt.Sprintf("  Memory:        %s", formatSignedBytes(info.MemoryBytes)), "",
		"Docker Configuration", fmt.Sprintf("  Storage:       %s", valueOrUnknown(info.StorageDriver)), fmt.Sprintf("  Logging:       %s", valueOrUnknown(info.LoggingDriver)), fmt.Sprintf("  Cgroup:        %s", formatCgroup(info.CgroupDriver, info.CgroupVersion)), fmt.Sprintf("  Runtime:       %s", valueOrUnknown(info.DefaultRuntime)), fmt.Sprintf("  Data Root:     %s", valueOrUnknown(info.DockerRootDir)), fmt.Sprintf("  Live Restore:  %s", yesNo(info.LiveRestore)), fmt.Sprintf("  Experimental:  %s", yesNo(info.ExperimentalBuild))}
}
func yesNo(value bool) string {
	if value {
		return "Yes"
	}
	return "No"
}
func formatCgroup(driver, version string) string {
	driver = valueOrUnknown(driver)
	if version == "" {
		return driver
	}
	return driver + " (v" + version + ")"
}
