package ui

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"dockman/internal/model"
	"dockman/internal/service"
)

type fakeConnection struct{ err error }

func (f fakeConnection) Check(context.Context) error { return f.err }

type fakeContainers struct{}

func (fakeContainers) List(context.Context) ([]model.Container, error) { return nil, nil }
func (fakeContainers) Detail(context.Context, string) (model.ContainerDetail, error) {
	return model.ContainerDetail{}, nil
}
func (fakeContainers) Execute(context.Context, string, service.ContainerAction) error { return nil }
func (fakeContainers) Stats(context.Context, string) (model.ContainerStats, error) {
	return model.ContainerStats{}, nil
}
func (fakeContainers) FollowLogs(context.Context, string) (<-chan service.LogEvent, error) {
	events := make(chan service.LogEvent)
	close(events)
	return events, nil
}
func (fakeContainers) Delete(context.Context, string, service.ContainerDeleteMode) error { return nil }
func (fakeContainers) Create(context.Context, service.ContainerCreateInput) (string, error) {
	return "1", nil
}
func (fakeContainers) OpenTerminal(context.Context, string, uint, uint) (model.TerminalSession, <-chan model.TerminalEvent, error) {
	events := make(chan model.TerminalEvent)
	close(events)
	return nil, events, nil
}

type fakeImages struct{}

func (fakeImages) List(context.Context) ([]model.Image, error) { return nil, nil }
func (fakeImages) Detail(context.Context, string) (model.ImageDetail, error) {
	return model.ImageDetail{}, nil
}
func (fakeImages) Pull(context.Context, string) (<-chan model.PullProgress, error) {
	progress := make(chan model.PullProgress)
	close(progress)
	return progress, nil
}
func (fakeImages) Delete(context.Context, string) error { return nil }

type fakeVolumes struct{}

func (fakeVolumes) List(context.Context) ([]model.Volume, error)         { return nil, nil }
func (fakeVolumes) Detail(context.Context, string) (model.Volume, error) { return model.Volume{}, nil }
func (fakeVolumes) Create(context.Context, string) (model.Volume, error) { return model.Volume{}, nil }
func (fakeVolumes) Delete(context.Context, string, string) error         { return nil }

type fakeNetworks struct{}

func (fakeNetworks) List(context.Context) ([]model.Network, error) { return nil, nil }
func (fakeNetworks) Detail(context.Context, string) (model.Network, error) {
	return model.Network{}, nil
}
func (fakeNetworks) Create(context.Context, string, string) (model.Network, error) {
	return model.Network{}, nil
}
func (fakeNetworks) Delete(context.Context, string) error             { return nil }
func (fakeNetworks) Connect(context.Context, string, string) error    { return nil }
func (fakeNetworks) Disconnect(context.Context, string, string) error { return nil }

type fakeCompose struct{}

func (fakeCompose) List(context.Context) ([]model.ComposeProject, error) { return nil, nil }
func (fakeCompose) Register(context.Context, service.ComposeRegisterInput) (model.ComposeProject, error) {
	return model.ComposeProject{}, nil
}
func (fakeCompose) Execute(context.Context, model.ComposeProject, service.ComposeAction) error {
	return nil
}
func (fakeCompose) FollowLogs(context.Context, model.ComposeProject) (<-chan service.LogEvent, error) {
	events := make(chan service.LogEvent)
	close(events)
	return events, nil
}

type fakeSystem struct{}

func (fakeSystem) Info(context.Context) (model.SystemInfo, error) { return model.SystemInfo{}, nil }
func (fakeSystem) AnalyzeCleanup(context.Context) (model.CleanupAnalysis, error) {
	return model.CleanupAnalysis{}, nil
}
func (fakeSystem) Prune(context.Context, model.CleanupSelection) (model.CleanupResult, error) {
	return model.CleanupResult{}, nil
}

func TestConnectionResultChangesScreen(t *testing.T) {
	a := NewApp(fakeConnection{}, fakeContainers{}, fakeImages{}, fakeVolumes{}, fakeNetworks{}, fakeCompose{}, fakeSystem{})
	model, _ := a.Update(connectionResultMsg{})
	if got := model.(App).screen; got != ScreenMainMenu {
		t.Fatalf("screen = %v", got)
	}

	a = NewApp(fakeConnection{}, fakeContainers{}, fakeImages{}, fakeVolumes{}, fakeNetworks{}, fakeCompose{}, fakeSystem{})
	model, _ = a.Update(connectionResultMsg{err: errors.New("unavailable")})
	if got := model.(App).screen; got != ScreenConnectionError {
		t.Fatalf("screen = %v", got)
	}
}

func TestEscapeReturnsFromFeatureScreen(t *testing.T) {
	a := NewApp(fakeConnection{}, fakeContainers{}, fakeImages{}, fakeVolumes{}, fakeNetworks{}, fakeCompose{}, fakeSystem{})
	a.screen = ScreenContainers
	model, _ := a.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if got := model.(App).screen; got != ScreenMainMenu {
		t.Fatalf("screen = %v", got)
	}
}

func TestEscapeDoesNotQuitApp(t *testing.T) {
	tests := []struct {
		name   string
		screen Screen
	}{
		{name: "main menu", screen: ScreenMainMenu},
		{name: "connecting", screen: ScreenConnecting},
		{name: "connection error", screen: ScreenConnectionError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewApp(fakeConnection{}, fakeContainers{}, fakeImages{}, fakeVolumes{}, fakeNetworks{}, fakeCompose{}, fakeSystem{})
			a.screen = tt.screen
			model, cmd := a.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			if cmd != nil {
				t.Fatal("Escape returned a command; expected it to be ignored")
			}
			if got := model.(App).screen; got != tt.screen {
				t.Fatalf("screen = %v, want %v", got, tt.screen)
			}
		})
	}
}

func TestTerminalKeyBytes(t *testing.T) {
	if got := string(terminalKeyBytes(tea.KeyPressMsg{Text: "ls", Code: 'l'})); got != "ls" {
		t.Fatalf("printable = %q", got)
	}
	if got := string(terminalKeyBytes(tea.KeyPressMsg{Code: tea.KeyUp})); got != "\x1b[A" {
		t.Fatalf("up = %q", got)
	}
	if got := terminalKeyBytes(tea.KeyPressMsg{Code: tea.KeyDelete}); len(got) != 1 || got[0] != 0x7f {
		t.Fatalf("delete = %v", got)
	}
}

func TestApplyTerminalTextHandlesBackspaceEcho(t *testing.T) {
	if got := applyTerminalText("prompt$ abc", "\b \b"); got != "prompt$ ab" {
		t.Fatalf("output = %q", got)
	}
	if got := applyTerminalText("line\n", "\b"); got != "line\n" {
		t.Fatalf("backspace crossed line boundary: %q", got)
	}
}

func TestFriendlyErrorExplainsCommonFailures(t *testing.T) {
	if got := friendlyError(context.DeadlineExceeded); !strings.Contains(got, "timed out") {
		t.Fatalf("timeout = %q", got)
	}
	if got := friendlyError(errors.New("permission denied")); !strings.Contains(got, "Permission denied") {
		t.Fatalf("permission = %q", got)
	}
	if got := friendlyError(errors.New("manifest unknown")); !strings.Contains(got, "image could not be downloaded") {
		t.Fatalf("image = %q", got)
	}
}

func TestContainerCreateUsesSeparatePreview(t *testing.T) {
	a := NewApp(fakeConnection{}, fakeContainers{}, fakeImages{}, fakeVolumes{}, fakeNetworks{}, fakeCompose{}, fakeSystem{})
	a.screen = ScreenContainerCreate
	a.containers.create.focus = 21
	a.containers.create.inputs[0].SetValue("web")
	a.containers.create.inputs[1].SetValue("nginx:latest")
	updated, _ := a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(App)
	if got.screen != ScreenContainerCreatePreview {
		t.Fatalf("screen = %v", got.screen)
	}
	if view := got.renderContainerCreatePreview(); !strings.Contains(view, "[ Create ]") || !strings.Contains(view, "nginx:latest") {
		t.Fatalf("preview = %q", view)
	}
}

func TestContainerImagePickerSelectsReference(t *testing.T) {
	a := NewApp(fakeConnection{}, fakeContainers{}, fakeImages{}, fakeVolumes{}, fakeNetworks{}, fakeCompose{}, fakeSystem{})
	a.screen = ScreenContainerImagePicker
	a.containers.create.imageChoices = []model.Image{{Repository: "nginx", Tag: "alpine"}}
	updated, _ := a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(App)
	if got.screen != ScreenContainerCreate || got.containers.create.inputs[1].Value() != "nginx:alpine" {
		t.Fatalf("selection = %v %q", got.screen, got.containers.create.inputs[1].Value())
	}
}

func (fakeContainers) ShellCommand(id string) *exec.Cmd { return exec.Command("true", id) }
