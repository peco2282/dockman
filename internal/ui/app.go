package ui

import (
	"context"
	"errors"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"dockman/internal/model"
	"dockman/internal/service"
)

type ConnectionChecker interface{ Check(context.Context) error }

type ContainerManager interface {
	List(context.Context) ([]model.Container, error)
	Detail(context.Context, string) (model.ContainerDetail, error)
	Execute(context.Context, string, service.ContainerAction) error
	Stats(context.Context, string) (model.ContainerStats, error)
	FollowLogs(context.Context, string) (<-chan service.LogEvent, error)
	Delete(context.Context, string, service.ContainerDeleteMode) error
	Create(context.Context, service.ContainerCreateInput) (string, error)
	OpenTerminal(context.Context, string, uint, uint) (model.TerminalSession, <-chan model.TerminalEvent, error)
}

type ImageManager interface {
	List(context.Context) ([]model.Image, error)
	Detail(context.Context, string) (model.ImageDetail, error)
	Pull(context.Context, string) (<-chan model.PullProgress, error)
	Delete(context.Context, string) error
}

type VolumeManager interface {
	List(context.Context) ([]model.Volume, error)
	Detail(context.Context, string) (model.Volume, error)
	Create(context.Context, string) (model.Volume, error)
	Delete(context.Context, string, string) error
}

type NetworkManager interface {
	List(context.Context) ([]model.Network, error)
	Detail(context.Context, string) (model.Network, error)
	Create(context.Context, string, string) (model.Network, error)
	Delete(context.Context, string) error
	Connect(context.Context, string, string) error
	Disconnect(context.Context, string, string) error
}

type ComposeManager interface {
	List(context.Context) ([]model.ComposeProject, error)
	Register(context.Context, service.ComposeRegisterInput) (model.ComposeProject, error)
	Execute(context.Context, model.ComposeProject, service.ComposeAction) error
	FollowLogs(context.Context, model.ComposeProject) (<-chan service.LogEvent, error)
}

type SystemManager interface {
	Info(context.Context) (model.SystemInfo, error)
	AnalyzeCleanup(context.Context) (model.CleanupAnalysis, error)
	Prune(context.Context, model.CleanupSelection) (model.CleanupResult, error)
}

type connectionResultMsg struct{ err error }

type App struct {
	connection        ConnectionChecker
	containersService ContainerManager
	imagesService     ImageManager
	volumesService    VolumeManager
	networksService   NetworkManager
	composeService    ComposeManager
	systemService     SystemManager
	screen            Screen
	menuCursor        int
	errorFocus        int
	connectErr        error
	spinner           spinner.Model
	width             int
	height            int
	containers        containerScreen
	images            imageScreen
	volumes           volumeScreen
	networks          networkScreen
	compose           composeScreen
	system            systemScreen
	contexts          contextScreen
	logCancel         context.CancelFunc
	terminalCancel    context.CancelFunc
	pullCancel        context.CancelFunc
	composeCancel     context.CancelFunc
}

func NewApp(connection ConnectionChecker, containers ContainerManager, images ImageManager, volumes VolumeManager, networks NetworkManager, compose ComposeManager, system SystemManager) App {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return App{connection: connection, containersService: containers, imagesService: images, volumesService: volumes, networksService: networks, composeService: compose, systemService: system, screen: ScreenConnecting, spinner: s, containers: newContainerScreen(), images: newImageScreen(), volumes: newVolumeScreen(), networks: newNetworkScreen(), compose: newComposeScreen()}
}

func (a App) Init() tea.Cmd { return tea.Batch(a.spinner.Tick, a.checkConnection()) }

func (a App) checkConnection() tea.Cmd {
	return func() tea.Msg { return connectionResultMsg{err: a.connection.Check(context.Background())} }
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		if a.screen == ScreenContainerTerminal && a.containers.terminalSession != nil {
			return a, a.resizeContainerTerminal()
		}
	case connectionResultMsg:
		if msg.err != nil {
			a.connectErr, a.screen = msg.err, ScreenConnectionError
		} else {
			a.connectErr, a.screen = nil, ScreenMainMenu
		}
		return a, nil
	case containersLoadedMsg:
		a.containers.loading, a.containers.err = false, msg.err
		if msg.err == nil {
			a.containers.items = msg.items
			if len(msg.items) == 0 {
				a.containers.cursor = 0
			} else if a.containers.cursor >= len(msg.items) {
				a.containers.cursor = len(msg.items) - 1
			}
		}
		if a.screen == ScreenContainerStats && !a.containers.statsPaused {
			return a, statsDashboardTick()
		}
		return a, nil
	case statsDashboardTickMsg:
		if a.screen == ScreenContainerStats && !a.containers.statsPaused {
			a.containers.loading = true
			return a, a.loadContainerStatsDashboard()
		}
		return a, nil
	case contextsLoadedMsg:
		a.contexts.loading, a.contexts.err = false, msg.err
		if msg.err == nil {
			a.contexts.items, a.contexts.current, a.contexts.cursor = msg.items, msg.current, 0
			for i, name := range msg.items {
				if name == msg.current {
					a.contexts.cursor = i
				}
			}
		}
		return a, nil
	case contextSwitchedMsg:
		a.contexts.loading, a.contexts.err = false, msg.err
		if msg.err == nil {
			a.contexts.current = msg.name
			a.containers.loading = true
			a.screen = ScreenContainers
			return a, tea.Batch(a.spinner.Tick, a.loadContainers())
		}
		return a, nil
	case containerDetailLoadedMsg:
		a.containers.loading, a.containers.err = false, msg.err
		if msg.err == nil {
			a.containers.detail = msg.detail
			if msg.detail.Running {
				a.containers.actionCursor = int(service.ContainerStop)
				a.containers.statsLoading, a.containers.statsErr = true, nil
				return a, a.loadContainerStats(msg.detail.ID)
			} else {
				a.containers.actionCursor = int(service.ContainerStart)
				a.containers.statsLoading, a.containers.statsErr = false, nil
				a.containers.stats = model.ContainerStats{}
			}
		}
		return a, nil
	case containerStatsLoadedMsg:
		a.containers.statsLoading, a.containers.statsErr = false, msg.err
		if msg.err == nil {
			a.containers.stats = msg.stats
		}
		return a, nil
	case containerActionResultMsg:
		a.containers.working = false
		if msg.err != nil {
			a.containers.err, a.containers.notice = msg.err, msg.action.String()+" failed"
			return a, nil
		}
		a.containers.err, a.containers.notice = nil, msg.action.String()+" completed"
		return a, tea.Batch(a.loadContainers(), a.loadContainerDetail(a.containers.detail.ID))
	case logStreamOpenedMsg:
		a.containers.loading, a.containers.err = false, msg.err
		if msg.err == nil {
			a.containers.logEvents = msg.events
			return a, waitForLog(msg.events)
		}
		return a, nil
	case logEventMsg:
		if a.screen != ScreenContainerLogs {
			return a, nil
		}
		if msg.event.Line != "" {
			a.appendLog(msg.event.Line)
		}
		if msg.event.Err != nil {
			a.containers.err = msg.event.Err
		}
		if msg.event.Done {
			a.containers.logDone = true
			return a, nil
		}
		return a, waitForLog(a.containers.logEvents)
	case containerDeleteResultMsg:
		a.containers.working = false
		if msg.err != nil {
			a.containers.err, a.containers.notice = msg.err, "Delete failed"
			return a, nil
		}
		a.containers.notice = "Container deleted. Volumes were kept."
		a.screen, a.containers.loading = ScreenContainers, true
		return a, tea.Batch(a.spinner.Tick, a.loadContainers())
	case containerCreateResultMsg:
		a.containers.working = false
		if msg.err != nil {
			a.containers.err = msg.err
			return a, nil
		}
		if a.containers.create.start {
			a.containers.notice = "Container created and started."
		} else {
			a.containers.notice = "Container created. It is stopped; open it and choose Start when ready."
		}
		a.containers.loading = true
		a.screen = ScreenContainers
		return a, tea.Batch(a.spinner.Tick, a.loadContainers())
	case containerCreateImagesLoadedMsg:
		a.containers.create.imageLoading, a.containers.err = false, msg.err
		if msg.err == nil {
			a.containers.create.imageChoices = msg.items
			a.containers.create.imageCursor = 0
		}
		return a, nil
	case terminalOpenedMsg:
		if a.screen != ScreenContainerTerminal {
			if msg.session != nil {
				_ = msg.session.Close()
			}
			return a, nil
		}
		a.containers.loading, a.containers.err = false, msg.err
		if msg.err == nil {
			a.containers.terminalSession, a.containers.terminalEvents = msg.session, msg.events
			return a, waitForTerminal(msg.events)
		}
		return a, nil
	case terminalEventMsg:
		if a.screen != ScreenContainerTerminal {
			return a, nil
		}
		if len(msg.event.Data) > 0 {
			a.appendTerminalOutput(msg.event.Data)
		}
		if msg.event.Err != nil {
			a.containers.err = msg.event.Err
		}
		if msg.event.Done {
			a.containers.terminalDone = true
			return a, nil
		}
		return a, waitForTerminal(a.containers.terminalEvents)
	case terminalWriteResultMsg:
		if msg.err != nil {
			a.containers.err = msg.err
		}
		return a, nil
	case imagesLoadedMsg:
		a.images.loading, a.images.err = false, msg.err
		if msg.err == nil {
			a.images.items = msg.items
			if len(msg.items) == 0 {
				a.images.cursor = 0
			} else if a.images.cursor >= len(msg.items) {
				a.images.cursor = len(msg.items) - 1
			}
		}
		return a, nil
	case imageDetailLoadedMsg:
		a.images.loading, a.images.err = false, msg.err
		if msg.err == nil {
			a.images.detail = msg.detail
		}
		return a, nil
	case imagePullOpenedMsg:
		a.images.working, a.images.err = false, msg.err
		if msg.err == nil {
			a.images.progress = msg.progress
			return a, waitForPull(msg.progress)
		}
		return a, nil
	case imagePullProgressMsg:
		if a.screen != ScreenImagePull {
			return a, nil
		}
		if msg.progress.Status != "" {
			a.appendPullProgress(msg.progress.Status)
		}
		if msg.progress.Err != nil {
			a.images.err = msg.progress.Err
			a.images.working = false
		}
		if msg.progress.Done {
			a.images.working = false
			a.images.pullDone = msg.progress.Err == nil
			if a.pullCancel != nil {
				a.pullCancel()
				a.pullCancel = nil
			}
			return a, nil
		}
		return a, waitForPull(a.images.progress)
	case imageDeleteResultMsg:
		a.images.working = false
		if msg.err != nil {
			a.images.err = msg.err
			return a, nil
		}
		a.images.notice = "Image deleted."
		a.images.loading = true
		a.screen = ScreenImages
		return a, tea.Batch(a.spinner.Tick, a.loadImages())
	case volumesLoadedMsg:
		a.volumes.loading, a.volumes.err = false, msg.err
		if msg.err == nil {
			a.volumes.items = msg.items
			if len(msg.items) == 0 {
				a.volumes.cursor = 0
			} else if a.volumes.cursor >= len(msg.items) {
				a.volumes.cursor = len(msg.items) - 1
			}
		}
		return a, nil
	case volumeDetailLoadedMsg:
		a.volumes.loading, a.volumes.err = false, msg.err
		if msg.err == nil {
			a.volumes.detail = msg.volume
		}
		return a, nil
	case volumeCreateResultMsg:
		a.volumes.working = false
		if msg.err != nil {
			a.volumes.err = msg.err
			return a, nil
		}
		a.volumes.notice = "Volume created: " + msg.volume.Name
		a.volumes.loading = true
		a.screen = ScreenVolumes
		return a, tea.Batch(a.spinner.Tick, a.loadVolumes())
	case volumeDeleteResultMsg:
		a.volumes.working = false
		if msg.err != nil {
			a.volumes.err = msg.err
			return a, nil
		}
		a.volumes.notice = "Volume deleted."
		a.volumes.loading = true
		a.screen = ScreenVolumes
		return a, tea.Batch(a.spinner.Tick, a.loadVolumes())
	case networksLoadedMsg:
		a.networks.loading, a.networks.err = false, msg.err
		if msg.err == nil {
			a.networks.items = msg.items
			a.networks.clampCursor()
		}
		return a, nil
	case networkDetailLoadedMsg:
		a.networks.loading, a.networks.err = false, msg.err
		if msg.err == nil {
			a.networks.detail = msg.network
			a.networks.containerCursor = 0
		}
		return a, nil
	case networkCreateResultMsg:
		a.networks.working, a.networks.err = false, msg.err
		if msg.err == nil {
			a.networks.notice = "Network created: " + msg.network.Name
			a.networks.loading = true
			a.screen = ScreenNetworks
			return a, tea.Batch(a.spinner.Tick, a.loadNetworks())
		}
		return a, nil
	case networkActionResultMsg:
		a.networks.working, a.networks.err = false, msg.err
		if msg.err == nil {
			a.networks.notice = msg.notice
			if msg.deleted {
				a.networks.loading = true
				a.screen = ScreenNetworks
				return a, tea.Batch(a.spinner.Tick, a.loadNetworks())
			}
			a.networks.loading = true
			a.screen = ScreenNetworkDetail
			return a, tea.Batch(a.spinner.Tick, a.loadNetworkDetail(a.networks.detail.ID))
		}
		return a, nil
	case composeProjectsLoadedMsg:
		a.compose.loading, a.compose.err = false, msg.err
		if msg.err == nil {
			a.compose.projects = msg.projects
			if len(msg.projects) == 0 {
				a.compose.cursor = 0
			} else if a.compose.cursor >= len(msg.projects) {
				a.compose.cursor = len(msg.projects) - 1
			}
		}
		return a, nil
	case composeActionResultMsg:
		a.compose.working = false
		if msg.err != nil {
			a.compose.err = msg.err
			a.compose.notice = msg.action.String() + " failed"
			return a, nil
		}
		a.compose.notice = msg.action.String() + " completed"
		a.compose.loading = true
		a.screen = ScreenCompose
		return a, tea.Batch(a.spinner.Tick, a.loadComposeProjects())
	case composeRegisterResultMsg:
		a.compose.working = false
		if msg.err != nil {
			a.compose.err = msg.err
			return a, nil
		}
		a.compose.notice = "Compose project registered: " + msg.project.Name
		a.compose.loading = true
		a.screen = ScreenCompose
		return a, tea.Batch(a.spinner.Tick, a.loadComposeProjects())
	case composeUnregisterResultMsg:
		a.compose.working, a.compose.err = false, msg.err
		if msg.err == nil {
			a.compose.notice = "Compose project unregistered."
		}
		a.compose.loading = true
		a.screen = ScreenCompose
		return a, tea.Batch(a.spinner.Tick, a.loadComposeProjects())
	case composeLogsOpenedMsg:
		a.compose.loading, a.compose.err = false, msg.err
		if msg.err == nil {
			a.compose.logEvents = msg.events
			return a, waitForComposeLog(msg.events)
		}
		return a, nil
	case composeLogEventMsg:
		if a.screen != ScreenComposeLogs {
			return a, nil
		}
		if msg.event.Line != "" {
			a.appendComposeLog(msg.event.Line)
		}
		if msg.event.Err != nil {
			a.compose.err = msg.event.Err
		}
		if msg.event.Done {
			a.compose.logDone = true
			return a, nil
		}
		return a, waitForComposeLog(a.compose.logEvents)
	case systemInfoLoadedMsg:
		a.system.loading, a.system.err = false, msg.err
		if msg.err == nil {
			a.system.info = msg.info
			a.system.offset = 0
		}
		return a, nil
	case cleanupAnalyzedMsg:
		a.system.loading, a.system.err = false, msg.err
		if msg.err == nil {
			a.system.cleanup, a.system.cleanupCursor = msg.analysis, 0
			a.system.selection = allAvailable(msg.analysis)
		}
		return a, nil
	case cleanupPrunedMsg:
		a.system.loading, a.system.err = false, msg.err
		if msg.err != nil {
			return a, nil
		}
		a.system.notice = "Cleanup completed; actual reclaimed space: " + formatSignedBytes(int64(msg.result.SpaceReclaimed))
		a.system.loading = true
		return a, tea.Batch(a.spinner.Tick, a.loadCleanupAnalysis())
	case tea.KeyPressMsg:
		key := msg.String()
		if key == "ctrl+c" {
			if a.terminalCancel != nil {
				a.terminalCancel()
			}
			if a.containers.terminalSession != nil {
				_ = a.containers.terminalSession.Close()
			}
			if a.composeCancel != nil {
				a.composeCancel()
			}
			return a, tea.Quit
		}
		switch a.screen {
		case ScreenMainMenu:
			return a.updateMenu(key)
		case ScreenConnectionError:
			return a.updateConnectionError(key)
		case ScreenContainers:
			return a.updateContainers(key)
		case ScreenContainerDetail:
			return a.updateContainerDetail(key)
		case ScreenContainerLogs:
			return a.updateContainerLogs(key)
		case ScreenContainerDelete:
			return a.updateContainerDelete(key)
		case ScreenContainerCreate:
			return a.updateContainerCreate(msg)
		case ScreenContainerImagePicker:
			return a.updateContainerImagePicker(key)
		case ScreenContainerCreatePreview:
			return a.updateContainerCreatePreview(key)
		case ScreenContainerTerminal:
			return a.updateContainerTerminal(msg)
		case ScreenContainerStats:
			return a.updateContainerStatsDashboard(key)
		case ScreenImages:
			return a.updateImages(key)
		case ScreenImageDetail:
			return a.updateImageDetail(key)
		case ScreenImagePull:
			return a.updateImagePull(msg)
		case ScreenImageDelete:
			return a.updateImageDelete(key)
		case ScreenVolumes:
			return a.updateVolumes(key)
		case ScreenVolumeDetail:
			return a.updateVolumeDetail(key)
		case ScreenVolumeCreate:
			return a.updateVolumeCreate(msg)
		case ScreenVolumeDelete:
			return a.updateVolumeDelete(msg)
		case ScreenNetworks:
			return a.updateNetworks(key)
		case ScreenNetworkDetail:
			return a.updateNetworkDetail(key)
		case ScreenNetworkCreate:
			return a.updateNetworkCreate(msg)
		case ScreenNetworkDelete:
			return a.updateNetworkDelete(key)
		case ScreenNetworkConnect:
			return a.updateNetworkConnect(key)
		case ScreenCompose:
			return a.updateComposeProjects(key)
		case ScreenComposeRegister:
			return a.updateComposeRegister(msg)
		case ScreenComposeDetail:
			return a.updateComposeDetail(key)
		case ScreenComposeLogs:
			return a.updateComposeLogs(key)
		case ScreenComposeContainers:
			return a.updateComposeContainers(key)
		case ScreenSystem:
			return a.updateSystem(key)
		case ScreenCleanup:
			return a.updateCleanup(key)
		case ScreenCleanupConfirm:
			return a.updateCleanupConfirm(key)
		case ScreenContexts:
			return a.updateContexts(key)
		case ScreenConnecting:
			if key == "q" {
				return a, tea.Quit
			}
		default:
			if key == "esc" || key == "q" || key == "backspace" {
				a.screen = ScreenMainMenu
			}
		}
	}
	if a.screen == ScreenConnecting || a.containers.loading || a.containers.working || a.containers.create.imageLoading || a.images.loading || a.images.working || a.volumes.loading || a.volumes.working || a.networks.loading || a.networks.working || a.compose.loading || a.compose.working || a.system.loading {
		var cmd tea.Cmd
		a.spinner, cmd = a.spinner.Update(msg)
		return a, cmd
	}
	return a, nil
}

func (a App) updateMenu(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		a.menuCursor = (a.menuCursor - 1 + len(mainMenuItems)) % len(mainMenuItems)
	case "down", "j", "tab":
		a.menuCursor = (a.menuCursor + 1) % len(mainMenuItems)
	case "enter":
		item := mainMenuItems[a.menuCursor]
		if item.exit {
			return a, tea.Quit
		}
		a.screen = item.screen
		if item.screen == ScreenContainers {
			a.containers.loading, a.containers.err, a.containers.notice = true, nil, ""
			return a, tea.Batch(a.spinner.Tick, a.loadContainers())
		}
		if item.screen == ScreenImages {
			a.images.loading, a.images.err, a.images.notice = true, nil, ""
			return a, tea.Batch(a.spinner.Tick, a.loadImages())
		}
		if item.screen == ScreenVolumes {
			a.volumes.loading, a.volumes.err, a.volumes.notice = true, nil, ""
			return a, tea.Batch(a.spinner.Tick, a.loadVolumes())
		}
		if item.screen == ScreenNetworks {
			a.networks.loading, a.networks.err, a.networks.notice = true, nil, ""
			return a, tea.Batch(a.spinner.Tick, a.loadNetworks())
		}
		if item.screen == ScreenCompose {
			a.compose.loading, a.compose.err, a.compose.notice = true, nil, ""
			return a, tea.Batch(a.spinner.Tick, a.loadComposeProjects())
		}
		if item.screen == ScreenSystem {
			a.system.loading, a.system.err = true, nil
			return a, tea.Batch(a.spinner.Tick, a.loadSystemInfo())
		}
	case "q":
		return a, tea.Quit
	}
	return a, nil
}

func (a App) updateConnectionError(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "left", "right", "tab", "up", "down":
		a.errorFocus = 1 - a.errorFocus
	case "r":
		a.screen, a.connectErr = ScreenConnecting, nil
		return a, tea.Batch(a.spinner.Tick, a.checkConnection())
	case "enter":
		if a.errorFocus == 0 {
			a.screen, a.connectErr = ScreenConnecting, nil
			return a, tea.Batch(a.spinner.Tick, a.checkConnection())
		}
		return a, tea.Quit
	case "q":
		return a, tea.Quit
	}
	return a, nil
}

func (a App) View() tea.View {
	content := a.render()
	if a.screen != ScreenContainerTerminal && a.width > 0 && a.height > 0 {
		horizontalPosition := lipgloss.Center
		if a.screen == ScreenContainerLogs || a.screen == ScreenComposeLogs || a.screen == ScreenContainerStats || a.screen == ScreenContainerCreatePreview {
			horizontalPosition = lipgloss.Left
		}
		content = lipgloss.Place(a.width, a.height, horizontalPosition, lipgloss.Center, content)
	}
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (a App) render() string {
	if a.width > 0 && (a.width < 40 || a.height < 10) {
		return errorStyle.Render("Terminal is too small") + "\n\nPlease resize it to at least 40 x 10.\n\nCtrl+C Quit"
	}
	switch a.screen {
	case ScreenConnecting:
		return titleStyle.Render("Docker Manager") + "\n\n" + a.spinner.View() + " Connecting to Docker Engine...\n\n" + mutedStyle.Render("q Quit")
	case ScreenConnectionError:
		return a.renderConnectionError()
	case ScreenMainMenu:
		return a.renderMenu()
	case ScreenContainers:
		return a.renderContainers()
	case ScreenContainerDetail:
		return a.renderContainerDetail()
	case ScreenContainerLogs:
		return a.renderContainerLogs()
	case ScreenContainerDelete:
		return a.renderContainerDelete()
	case ScreenContainerCreate:
		return a.renderContainerCreate()
	case ScreenContainerImagePicker:
		return a.renderContainerImagePicker()
	case ScreenContainerCreatePreview:
		return a.renderContainerCreatePreview()
	case ScreenContainerTerminal:
		return a.renderContainerTerminal()
	case ScreenContainerStats:
		return a.renderContainerStatsDashboard()
	case ScreenImages:
		return a.renderImages()
	case ScreenImageDetail:
		return a.renderImageDetail()
	case ScreenImagePull:
		return a.renderImagePull()
	case ScreenImageDelete:
		return a.renderImageDelete()
	case ScreenVolumes:
		return a.renderVolumes()
	case ScreenVolumeDetail:
		return a.renderVolumeDetail()
	case ScreenVolumeCreate:
		return a.renderVolumeCreate()
	case ScreenVolumeDelete:
		return a.renderVolumeDelete()
	case ScreenNetworks:
		return a.renderNetworks()
	case ScreenNetworkDetail:
		return a.renderNetworkDetail()
	case ScreenNetworkCreate:
		return a.renderNetworkCreate()
	case ScreenNetworkDelete:
		return a.renderNetworkDelete()
	case ScreenNetworkConnect:
		return a.renderNetworkConnect()
	case ScreenCompose:
		return a.renderComposeProjects()
	case ScreenComposeRegister:
		return a.renderComposeRegister()
	case ScreenComposeDetail:
		return a.renderComposeDetail()
	case ScreenComposeLogs:
		return a.renderComposeLogs()
	case ScreenComposeContainers:
		return a.renderComposeContainers()
	case ScreenSystem:
		return a.renderSystem()
	case ScreenCleanup:
		return a.renderCleanup()
	case ScreenCleanupConfirm:
		return a.renderCleanupConfirm()
	case ScreenContexts:
		return a.renderContexts()
	default:
		return a.renderPlaceholder()
	}
}

func (a App) renderMenu() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Docker Manager") + "\n\n")
	for i, item := range mainMenuItems {
		line := "  " + item.label
		if i == a.menuCursor {
			line = selectedStyle.Render("> " + item.label)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n" + mutedStyle.Render("↑↓ Move   Enter Select   q Quit"))
	return b.String()
}

func (a App) renderPlaceholder() string {
	names := map[Screen]string{}
	return titleStyle.Render(names[a.screen]) + "\n\nThis feature is planned for a later phase.\n\n" + mutedStyle.Render("Esc Back   Ctrl+C Quit")
}

func (a App) renderConnectionError() string {
	reason := "Docker Engine could not be reached."
	var connectionError *service.ConnectionError
	if errors.As(a.connectErr, &connectionError) && connectionError.Kind == service.ConnectionPermissionDenied {
		reason = "Permission to access Docker was denied."
	}
	retry, exit := "[ Retry ]", "[ Exit ]"
	if a.errorFocus == 0 {
		retry = selectedStyle.Render(retry)
	} else {
		exit = selectedStyle.Render(exit)
	}
	return errorStyle.Render("Cannot connect to Docker") + "\n\n" + reason + "\n\nPossible causes:\n- Docker is not running\n- Permission denied\n- Current user cannot access Docker\n\n" + retry + "  " + exit + "\n\n" + mutedStyle.Render("←→ Choose   Enter Select   r Retry   q Quit")
}
