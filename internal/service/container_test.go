package service

import (
	"context"
	"io"
	"strings"
	"testing"

	"dockman/internal/model"
)

type fakeContainerRepository struct{ action ContainerAction }

func (*fakeContainerRepository) ListContainers(context.Context) ([]model.Container, error) {
	return []model.Container{{ID: "1", Name: "web"}}, nil
}
func (*fakeContainerRepository) InspectContainer(context.Context, string) (model.ContainerDetail, error) {
	return model.ContainerDetail{ID: "1", Name: "web"}, nil
}
func (f *fakeContainerRepository) StartContainer(context.Context, string) error {
	f.action = ContainerStart
	return nil
}
func (f *fakeContainerRepository) StopContainer(context.Context, string) error {
	f.action = ContainerStop
	return nil
}
func (f *fakeContainerRepository) RestartContainer(context.Context, string) error {
	f.action = ContainerRestart
	return nil
}
func (*fakeContainerRepository) ContainerStats(context.Context, string) (model.ContainerStats, error) {
	return model.ContainerStats{CPUPercent: 12.5}, nil
}
func (*fakeContainerRepository) ContainerLogs(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("one\ntwo\n")), nil
}
func (*fakeContainerRepository) RemoveContainer(context.Context, string, bool) error { return nil }
func (*fakeContainerRepository) CreateContainer(context.Context, model.ContainerCreateRequest) (string, error) {
	return "created", nil
}
func (*fakeContainerRepository) OpenContainerTerminal(context.Context, string, uint, uint) (model.TerminalSession, error) {
	return &fakeTerminalSession{reader: strings.NewReader("ready\n")}, nil
}

type fakeTerminalSession struct{ reader *strings.Reader }

func (f *fakeTerminalSession) Read(p []byte) (int, error)             { return f.reader.Read(p) }
func (*fakeTerminalSession) Write(p []byte) (int, error)              { return len(p), nil }
func (*fakeTerminalSession) Close() error                             { return nil }
func (*fakeTerminalSession) Resize(context.Context, uint, uint) error { return nil }

func TestContainerServiceListsAllRepositoryResults(t *testing.T) {
	s := NewContainerService(&fakeContainerRepository{})
	items, err := s.List(context.Background())
	if err != nil || len(items) != 1 || items[0].Name != "web" {
		t.Fatalf("List() = %#v, %v", items, err)
	}
}

func TestContainerServiceExecutesAction(t *testing.T) {
	repository := &fakeContainerRepository{}
	s := NewContainerService(repository)
	if err := s.Execute(context.Background(), "1", ContainerRestart); err != nil {
		t.Fatal(err)
	}
	if repository.action != ContainerRestart {
		t.Fatalf("action = %v", repository.action)
	}
}

func TestContainerServiceFollowsLogs(t *testing.T) {
	s := NewContainerService(&fakeContainerRepository{})
	events, err := s.FollowLogs(context.Background(), "1")
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for event := range events {
		if event.Line != "" {
			lines = append(lines, event.Line)
		}
	}
	if len(lines) != 2 || lines[0] != "one" || lines[1] != "two" {
		t.Fatalf("lines = %#v", lines)
	}
}

type trackingDeleteRepository struct {
	fakeContainerRepository
	calls []string
}

func (r *trackingDeleteRepository) StopContainer(context.Context, string) error {
	r.calls = append(r.calls, "stop")
	return nil
}
func (r *trackingDeleteRepository) RemoveContainer(context.Context, string, bool) error {
	r.calls = append(r.calls, "remove")
	return nil
}

func TestContainerServiceStopThenDelete(t *testing.T) {
	repository := &trackingDeleteRepository{}
	s := NewContainerService(repository)
	if err := s.Delete(context.Background(), "1", StopThenDelete); err != nil {
		t.Fatal(err)
	}
	if strings.Join(repository.calls, ",") != "stop,remove" {
		t.Fatalf("calls = %v", repository.calls)
	}
}

func TestContainerCreateValidation(t *testing.T) {
	tests := []struct {
		name    string
		input   ContainerCreateInput
		wantErr bool
	}{
		{"valid", ContainerCreateInput{Name: "minecraft", Image: "itzg/minecraft-server:latest", Ports: "25565:25565", Mounts: "/opt/minecraft:/data, minecraft-data:/config:ro", Environment: "EULA=TRUE", RestartPolicy: "unless-stopped"}, false},
		{"missing image", ContainerCreateInput{Name: "web", RestartPolicy: "no"}, true},
		{"invalid port", ContainerCreateInput{Name: "web", Image: "nginx", Ports: "99999:80", RestartPolicy: "no"}, true},
		{"invalid environment", ContainerCreateInput{Name: "web", Image: "nginx", Environment: "BROKEN", RestartPolicy: "no"}, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := validateContainerCreateInput(test.input)
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestContainerCreateAdvancedOptions(t *testing.T) {
	request, err := validateContainerCreateInput(ContainerCreateInput{Name: "worker", Image: "alpine", RestartPolicy: "unless-stopped", Command: "sleep 60", Network: "app-net", WorkingDir: "/work", Labels: "team=platform", CPUs: "1.5", Memory: "512m", Capabilities: "net_admin", Devices: "/dev/null:/dev/test:r", HealthCommand: "true", HealthInterval: "30s", Privileged: true, Start: true})
	if err != nil {
		t.Fatal(err)
	}
	if request.NanoCPUs != 1500000000 || request.Memory != 512*1024*1024 || !request.Privileged || !request.Start {
		t.Fatalf("advanced request = %#v", request)
	}
	if len(request.Command) != 2 || request.Network != "app-net" || request.Labels["team"] != "platform" || len(request.Devices) != 1 {
		t.Fatalf("advanced request = %#v", request)
	}
}

func TestContainerServiceTerminalStreamsOutput(t *testing.T) {
	s := NewContainerService(&fakeContainerRepository{})
	session, events, err := s.OpenTerminal(context.Background(), "1", 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	var output string
	for event := range events {
		output += string(event.Data)
	}
	if output != "ready\n" {
		t.Fatalf("output = %q", output)
	}
}
