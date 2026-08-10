package service

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dockman/internal/model"
)

type fakeComposeRepository struct{}

func (fakeComposeRepository) ListComposeProjects(context.Context) ([]model.ComposeProject, error) {
	return []model.ComposeProject{{Name: "demo", WorkingDir: "/srv/demo"}}, nil
}

type fakeComposeRunner struct{ operations [][]string }

func (f *fakeComposeRunner) Run(_ context.Context, _ model.ComposeProject, operation ...string) error {
	f.operations = append(f.operations, operation)
	return nil
}
func (*fakeComposeRunner) Stream(context.Context, model.ComposeProject, ...string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("web | ready\n")), nil
}

func TestComposeServiceDownDoesNotRequestVolumes(t *testing.T) {
	runner := &fakeComposeRunner{}
	service := NewComposeServiceWithRegistry(fakeComposeRepository{}, runner, "")
	project := model.ComposeProject{Name: "demo", WorkingDir: "/srv/demo"}
	if err := service.Execute(context.Background(), project, ComposeDown); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(runner.operations[0], " ")
	if got != "down" {
		t.Fatalf("operation = %q", got)
	}
}
func TestComposeArgsIncludeProjectAndFiles(t *testing.T) {
	args := composeArgs(model.ComposeProject{Name: "demo", ConfigFiles: []string{"compose.yaml", "compose.prod.yaml"}}, "up", "--detach")
	got := strings.Join(args, " ")
	want := "compose --project-name demo --file compose.yaml --file compose.prod.yaml up --detach"
	if got != want {
		t.Fatalf("args = %q", got)
	}
}
func TestComposeServiceStreamsLogs(t *testing.T) {
	service := NewComposeServiceWithRegistry(fakeComposeRepository{}, &fakeComposeRunner{}, "")
	events, err := service.FollowLogs(context.Background(), model.ComposeProject{Name: "demo", WorkingDir: "/srv/demo"})
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for event := range events {
		if event.Line != "" {
			lines = append(lines, event.Line)
		}
	}
	if len(lines) != 1 || lines[0] != "web | ready" {
		t.Fatalf("lines = %v", lines)
	}
}

type disappearingComposeRepository struct{ calls int }

func (r *disappearingComposeRepository) ListComposeProjects(context.Context) ([]model.ComposeProject, error) {
	r.calls++
	if r.calls == 1 {
		return []model.ComposeProject{{Name: "demo", WorkingDir: "/srv/demo", Containers: []model.ComposeContainer{{Name: "web"}}}}, nil
	}
	return nil, nil
}

func TestComposeServiceKeepsKnownProjectAfterDown(t *testing.T) {
	repository := &disappearingComposeRepository{}
	service := NewComposeServiceWithRegistry(repository, &fakeComposeRunner{}, "")
	if _, err := service.List(context.Background()); err != nil {
		t.Fatal(err)
	}
	projects, err := service.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Name != "demo" || len(projects[0].Containers) != 0 {
		t.Fatalf("projects = %#v", projects)
	}
}

func TestComposeServiceRegistersAndReloadsProject(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "compose.yaml"), []byte("services: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	registry := filepath.Join(t.TempDir(), "compose-projects.json")
	service := NewComposeServiceWithRegistry(fakeComposeRepository{}, &fakeComposeRunner{}, registry)
	project, err := service.Register(context.Background(), ComposeRegisterInput{Name: "Demo-App", WorkingDir: directory})
	if err != nil {
		t.Fatal(err)
	}
	if project.Name != "demo-app" || !project.Registered || len(project.ConfigFiles) != 1 {
		t.Fatalf("project = %#v", project)
	}

	reloaded := NewComposeServiceWithRegistry(emptyComposeRepository{}, &fakeComposeRunner{}, registry)
	projects, err := reloaded.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Name != "demo-app" || !projects[0].Registered {
		t.Fatalf("projects = %#v", projects)
	}
}

type emptyComposeRepository struct{}

func (emptyComposeRepository) ListComposeProjects(context.Context) ([]model.ComposeProject, error) {
	return nil, nil
}

func TestComposeServicePull(t *testing.T) {
	runner := &fakeComposeRunner{}
	service := NewComposeServiceWithRegistry(fakeComposeRepository{}, runner, "")
	if err := service.Execute(context.Background(), model.ComposeProject{Name: "demo", WorkingDir: "/srv/demo"}, ComposePull); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(runner.operations[0], " "); got != "pull" {
		t.Fatalf("operation = %q", got)
	}
}
