package ui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"dockman/internal/model"
	"dockman/internal/service"
)

type containerCall struct {
	id     string
	action service.ContainerAction
}

type recordingContainers struct {
	fakeContainers
	calls *[]containerCall
}

func (r recordingContainers) Execute(_ context.Context, id string, action service.ContainerAction) error {
	*r.calls = append(*r.calls, containerCall{id, action})
	return nil
}

type recordingCompose struct {
	fakeCompose
	calls *[]service.ComposeAction
}

func (r recordingCompose) Execute(_ context.Context, _ model.ComposeProject, action service.ComposeAction) error {
	*r.calls = append(*r.calls, action)
	return nil
}

type recordingSession struct{ written *[]byte }

func (s recordingSession) Read([]byte) (int, error) { return 0, nil }
func (s recordingSession) Write(p []byte) (int, error) {
	*s.written = append(*s.written, p...)
	return len(p), nil
}
func (s recordingSession) Close() error                             { return nil }
func (s recordingSession) Resize(context.Context, uint, uint) error { return nil }

// runCmd executes cmd and any batched children, returning the produced messages.
func runCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, child := range batch {
			out = append(out, runCmd(child)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func key(text string) tea.KeyPressMsg {
	r := []rune(text)
	return tea.KeyPressMsg{Code: r[0], Text: text}
}

func newShortcutApp(containers ContainerManager, compose ComposeManager) App {
	a := NewApp(fakeConnection{}, containers, fakeImages{}, fakeVolumes{}, fakeNetworks{}, compose, fakeSystem{})
	a.screen = ScreenContainers
	a.containers.items = []model.Container{
		{ID: "id-db", Name: "db", State: "exited"},
		{ID: "id-web", Name: "web", State: "running"},
	}
	return a
}

func TestContainerListToggleStartStop(t *testing.T) {
	var calls []containerCall
	a := newShortcutApp(recordingContainers{calls: &calls}, fakeCompose{})
	a.containers.cursor = 1 // web (sorted by name: db, web)

	m, cmd := a.Update(key("S"))
	got := m.(App)
	if got.screen != ScreenContainers || !got.containers.working {
		t.Fatalf("screen=%v working=%v", got.screen, got.containers.working)
	}
	var result tea.Msg
	for _, msg := range runCmd(cmd) {
		if r, ok := msg.(containerActionResultMsg); ok {
			result = r
		}
	}
	if len(calls) != 1 || calls[0] != (containerCall{"id-web", service.ContainerStop}) {
		t.Fatalf("calls = %#v", calls)
	}
	m, _ = got.Update(result)
	got = m.(App)
	if got.screen != ScreenContainers || got.containers.working || got.containers.notice != "Stop completed: web" {
		t.Fatalf("after result: screen=%v working=%v notice=%q", got.screen, got.containers.working, got.containers.notice)
	}

	// The stopped container starts instead.
	got.containers.cursor = 0
	_, cmd = got.Update(key("S"))
	runCmd(cmd)
	if len(calls) != 2 || calls[1] != (containerCall{"id-db", service.ContainerStart}) {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestContainerListIgnoresActionsWhileWorking(t *testing.T) {
	var calls []containerCall
	a := newShortcutApp(recordingContainers{calls: &calls}, fakeCompose{})
	a.containers.working = true
	_, cmd := a.Update(key("R"))
	runCmd(cmd)
	if len(calls) != 0 {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestSubScreensReturnToWhereTheyWereOpened(t *testing.T) {
	for _, origin := range []Screen{ScreenContainers, ScreenContainerDetail} {
		for _, k := range []string{"l", "D"} {
			a := newShortcutApp(fakeContainers{}, fakeCompose{})
			a.containers.cursor = 1
			if origin == ScreenContainerDetail {
				a.screen = ScreenContainerDetail
				a.containers.detail = detailFromContainer(a.containers.items[1])
			}
			m, _ := a.Update(key(k))
			got := m.(App)
			if got.screen == origin {
				t.Fatalf("%q from %v did not open a sub-screen", k, origin)
			}
			m, _ = got.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			if back := m.(App).screen; back != origin {
				t.Fatalf("%q from %v: Esc went to %v", k, origin, back)
			}
		}
	}
}

func TestTerminalSendsEscapeAndDetachesOnCtrlBracket(t *testing.T) {
	var written []byte
	a := newShortcutApp(fakeContainers{}, fakeCompose{})
	a.screen = ScreenContainerTerminal
	a.containers.origin = ScreenContainers
	a.containers.terminalSession = recordingSession{&written}

	m, cmd := a.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	runCmd(cmd)
	got := m.(App)
	if got.screen != ScreenContainerTerminal {
		t.Fatalf("Esc detached the terminal")
	}
	if string(written) != "\x1b" {
		t.Fatalf("written = %q", written)
	}
	m, _ = got.Update(tea.KeyPressMsg{Code: ']', Mod: tea.ModCtrl})
	if back := m.(App).screen; back != ScreenContainers {
		t.Fatalf("Ctrl+] went to %v", back)
	}
}

func TestComposeShortcutsAndDownConfirmation(t *testing.T) {
	var calls []service.ComposeAction
	a := newShortcutApp(fakeContainers{}, recordingCompose{calls: &calls})
	a.screen = ScreenComposeDetail
	a.compose.selected = model.ComposeProject{Name: "site", WorkingDir: "/srv/site"}

	m, cmd := a.Update(key("d"))
	runCmd(cmd)
	got := m.(App)
	if len(calls) != 0 || got.compose.confirm < 0 {
		t.Fatalf("Down ran without confirmation: calls=%v confirm=%d", calls, got.compose.confirm)
	}
	m, cmd = got.Update(key("n"))
	runCmd(cmd)
	got = m.(App)
	if len(calls) != 0 || got.compose.confirm >= 0 {
		t.Fatalf("n did not cancel: calls=%v", calls)
	}

	m, _ = got.Update(key("d"))
	m, cmd = m.(App).Update(key("y"))
	runCmd(cmd)
	if len(calls) != 1 || calls[0] != service.ComposeDown {
		t.Fatalf("calls = %v", calls)
	}

	got = m.(App)
	got.compose.working = false
	_, cmd = got.Update(key("U"))
	runCmd(cmd)
	if len(calls) != 2 || calls[1] != service.ComposeUpBuild {
		t.Fatalf("calls = %v", calls)
	}
}

func TestComposeButtonKeysAreUnique(t *testing.T) {
	seen := map[string]bool{"r": true} // r is Refresh
	for _, b := range composeButtons {
		if seen[b.key] {
			t.Fatalf("duplicate key %q", b.key)
		}
		seen[b.key] = true
	}
}
