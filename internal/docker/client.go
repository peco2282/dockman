package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"

	"dockman/internal/model"
)

var ErrNotConnected = errors.New("Docker client is not connected")

// Manager owns the Docker SDK client. Services share this manager so the UI
// never needs to depend on Docker SDK types directly.
type Manager struct {
	mu          sync.RWMutex
	client      *client.Client
	contextName string
}

type dockerContextInspect struct {
	Name      string `json:"Name"`
	Endpoints map[string]struct {
		Host          string `json:"Host"`
		SkipTLSVerify bool   `json:"SkipTLSVerify"`
	} `json:"Endpoints"`
	Storage struct {
		TLSPath string `json:"TLSPath"`
	} `json:"Storage"`
}

func (m *Manager) ListContexts(ctx context.Context) ([]string, string, error) {
	out, err := exec.CommandContext(ctx, "docker", "context", "ls", "--format", "{{if .Current}}*{{end}}{{.Name}}").CombinedOutput()
	if err != nil && len(strings.TrimSpace(string(out))) == 0 {
		return nil, "", err
	}
	var names []string
	current := ""
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "*") {
			current, line = strings.TrimPrefix(line, "*"), strings.TrimPrefix(line, "*")
		}
		names = append(names, line)
	}
	sort.Strings(names)
	m.mu.RLock()
	selected := m.contextName
	m.mu.RUnlock()
	if selected != "" {
		current = selected
	}
	return names, current, nil
}

func (m *Manager) SwitchContext(ctx context.Context, name string) error {
	out, err := exec.CommandContext(ctx, "docker", "context", "inspect", name).CombinedOutput()
	if err != nil && !json.Valid(out) {
		return err
	}
	var contexts []dockerContextInspect
	if err := json.Unmarshal(out, &contexts); err != nil || len(contexts) == 0 {
		if err != nil {
			return err
		}
		return errors.New("Docker context was not found")
	}
	ep, ok := contexts[0].Endpoints["docker"]
	if !ok || ep.Host == "" {
		return errors.New("Docker context has no Docker endpoint")
	}
	opts := []client.Opt{client.WithHost(ep.Host), client.WithAPIVersionNegotiation()}
	if contexts[0].Storage.TLSPath != "" && !strings.HasPrefix(contexts[0].Storage.TLSPath, "<") {
		opts = append(opts, client.WithTLSClientConfig(filepath.Join(contexts[0].Storage.TLSPath, "ca.pem"), filepath.Join(contexts[0].Storage.TLSPath, "cert.pem"), filepath.Join(contexts[0].Storage.TLSPath, "key.pem")))
	}
	c, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return err
	}
	if _, err = c.Ping(ctx, client.PingOptions{}); err != nil {
		_ = c.Close()
		return err
	}
	m.mu.Lock()
	old := m.client
	m.client, m.contextName = c, name
	m.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	return nil
}

func NewManager() *Manager { return &Manager{} }

func (m *Manager) ContextName() string { m.mu.RLock(); defer m.mu.RUnlock(); return m.contextName }

// Connect creates a client from the standard Docker environment and verifies
// that the engine is reachable. On Linux this defaults to the Unix socket.
func (m *Manager) Connect(ctx context.Context) error {
	c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}

	if _, err = c.Ping(ctx, client.PingOptions{}); err != nil {
		_ = c.Close()
		return err
	}

	m.mu.Lock()
	old := m.client
	m.client = c
	m.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	return nil
}

func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.client == nil {
		return nil
	}
	err := m.client.Close()
	m.client = nil
	return err
}

func (m *Manager) ListContainers(ctx context.Context) ([]model.Container, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.client == nil {
		return nil, ErrNotConnected
	}
	result, err := m.client.ContainerList(ctx, client.ContainerListOptions{All: true, Size: true})
	if err != nil {
		return nil, err
	}
	containers := make([]model.Container, 0, len(result.Items))
	for _, item := range result.Items {
		name := item.ID[:min(12, len(item.ID))]
		if len(item.Names) > 0 {
			name = strings.TrimPrefix(item.Names[0], "/")
		}
		ports := make([]string, 0, len(item.Ports))
		for _, port := range item.Ports {
			containerPort := fmt.Sprintf("%d/%s", port.PrivatePort, port.Type)
			if port.PublicPort > 0 {
				ports = append(ports, fmt.Sprintf("%d -> %s", port.PublicPort, containerPort))
			} else {
				ports = append(ports, containerPort)
			}
		}
		project := item.Labels["com.docker.compose.project"]
		containers = append(containers, model.Container{
			ID: item.ID, Name: name, Image: item.Image, State: string(item.State),
			Status: item.Status, Created: time.Unix(item.Created, 0), Ports: ports, Size: item.SizeRw, ComposeProject: project,
		})
	}
	return containers, nil
}

func (m *Manager) InspectContainer(ctx context.Context, id string) (model.ContainerDetail, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.client == nil {
		return model.ContainerDetail{}, ErrNotConnected
	}
	result, err := m.client.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return model.ContainerDetail{}, err
	}
	item := result.Container
	detail := model.ContainerDetail{ID: item.ID, Name: strings.TrimPrefix(item.Name, "/")}
	detail.Created, _ = time.Parse(time.RFC3339Nano, item.Created)
	if item.Config != nil {
		detail.Image = item.Config.Image
		detail.TTY = item.Config.Tty
	}
	if item.State != nil {
		detail.State = string(item.State.Status)
		detail.Running = item.State.Running
		detail.StartedAt, _ = time.Parse(time.RFC3339Nano, item.State.StartedAt)
	}
	for _, mount := range item.Mounts {
		source := mount.Source
		if mount.Name != "" {
			source = mount.Name
		}
		detail.Mounts = append(detail.Mounts, model.Mount{Source: source, Destination: mount.Destination, ReadOnly: !mount.RW})
	}
	if item.NetworkSettings != nil {
		for containerPort, bindings := range item.NetworkSettings.Ports {
			if len(bindings) == 0 {
				detail.Ports = append(detail.Ports, containerPort.String())
				continue
			}
			for _, binding := range bindings {
				detail.Ports = append(detail.Ports, fmt.Sprintf("%s -> %s", binding.HostPort, containerPort))
			}
		}
	}
	return detail, nil
}

func (m *Manager) StartContainer(ctx context.Context, id string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.client == nil {
		return ErrNotConnected
	}
	_, err := m.client.ContainerStart(ctx, id, client.ContainerStartOptions{})
	return err
}

func (m *Manager) StopContainer(ctx context.Context, id string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.client == nil {
		return ErrNotConnected
	}
	_, err := m.client.ContainerStop(ctx, id, client.ContainerStopOptions{})
	return err
}

func (m *Manager) RestartContainer(ctx context.Context, id string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.client == nil {
		return ErrNotConnected
	}
	_, err := m.client.ContainerRestart(ctx, id, client.ContainerRestartOptions{})
	return err
}

func (m *Manager) ContainerStats(ctx context.Context, id string) (model.ContainerStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.client == nil {
		return model.ContainerStats{}, ErrNotConnected
	}
	result, err := m.client.ContainerStats(ctx, id, client.ContainerStatsOptions{Stream: false, IncludePreviousSample: true})
	if err != nil {
		return model.ContainerStats{}, err
	}
	defer result.Body.Close()
	var payload struct {
		CPUStats struct {
			CPUUsage struct {
				TotalUsage uint64 `json:"total_usage"`
			}
			SystemCPUUsage uint64 `json:"system_cpu_usage"`
			OnlineCPUs     uint32 `json:"online_cpus"`
		} `json:"cpu_stats"`
		PreCPUStats struct {
			CPUUsage struct {
				TotalUsage uint64 `json:"total_usage"`
			}
			SystemCPUUsage uint64 `json:"system_cpu_usage"`
		} `json:"precpu_stats"`
		MemoryStats struct {
			Usage uint64            `json:"usage"`
			Limit uint64            `json:"limit"`
			Stats map[string]uint64 `json:"stats"`
		} `json:"memory_stats"`
		Networks map[string]struct {
			RxBytes uint64 `json:"rx_bytes"`
			TxBytes uint64 `json:"tx_bytes"`
		} `json:"networks"`
		BlkioStats struct {
			IOServiceBytesRecursive []struct {
				Op    string `json:"op"`
				Value uint64 `json:"value"`
			} `json:"io_service_bytes_recursive"`
		} `json:"blkio_stats"`
	}
	if err := json.NewDecoder(result.Body).Decode(&payload); err != nil {
		return model.ContainerStats{}, err
	}
	var cpuDelta, systemDelta uint64
	if payload.CPUStats.CPUUsage.TotalUsage >= payload.PreCPUStats.CPUUsage.TotalUsage {
		cpuDelta = payload.CPUStats.CPUUsage.TotalUsage - payload.PreCPUStats.CPUUsage.TotalUsage
	}
	if payload.CPUStats.SystemCPUUsage >= payload.PreCPUStats.SystemCPUUsage {
		systemDelta = payload.CPUStats.SystemCPUUsage - payload.PreCPUStats.SystemCPUUsage
	}
	cores := payload.CPUStats.OnlineCPUs
	if cores == 0 {
		cores = 1
	}
	cpuPercent := 0.0
	if systemDelta > 0 {
		cpuPercent = float64(cpuDelta) / float64(systemDelta) * float64(cores) * 100
	}
	usage := payload.MemoryStats.Usage
	if cache := payload.MemoryStats.Stats["inactive_file"]; cache < usage {
		usage -= cache
	}
	memoryPercent := 0.0
	if payload.MemoryStats.Limit > 0 {
		memoryPercent = float64(usage) / float64(payload.MemoryStats.Limit) * 100
	}
	stats := model.ContainerStats{CPUPercent: cpuPercent, MemoryUsage: usage, MemoryLimit: payload.MemoryStats.Limit, MemoryPercent: memoryPercent}
	for _, network := range payload.Networks {
		stats.NetworkRx += network.RxBytes
		stats.NetworkTx += network.TxBytes
	}
	for _, entry := range payload.BlkioStats.IOServiceBytesRecursive {
		switch strings.ToLower(entry.Op) {
		case "read":
			stats.BlockRead += entry.Value
		case "write":
			stats.BlockWrite += entry.Value
		}
	}
	return stats, nil
}

func (m *Manager) ContainerLogs(ctx context.Context, id string) (io.ReadCloser, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.client == nil {
		return nil, ErrNotConnected
	}
	inspect, err := m.client.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		return nil, err
	}
	result, err := m.client.ContainerLogs(ctx, id, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Follow: true, Tail: "500", Timestamps: true})
	if err != nil {
		return nil, err
	}
	if inspect.Container.Config != nil && inspect.Container.Config.Tty {
		return result, nil
	}
	reader, writer := io.Pipe()
	go func() {
		_, copyErr := stdcopy.StdCopy(writer, writer, result)
		_ = result.Close()
		_ = writer.CloseWithError(copyErr)
	}()
	return reader, nil
}

func (m *Manager) RemoveContainer(ctx context.Context, id string, force bool) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.client == nil {
		return ErrNotConnected
	}
	_, err := m.client.ContainerRemove(ctx, id, client.ContainerRemoveOptions{Force: force, RemoveVolumes: false})
	return err
}

func (m *Manager) CreateContainer(ctx context.Context, request model.ContainerCreateRequest) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.client == nil {
		return "", ErrNotConnected
	}
	portSet := network.PortSet{}
	portBindings := network.PortMap{}
	for _, mapping := range request.Ports {
		port, err := network.ParsePort(fmt.Sprintf("%d/%s", mapping.ContainerPort, mapping.Protocol))
		if err != nil {
			return "", err
		}
		portSet[port] = struct{}{}
		portBindings[port] = append(portBindings[port], network.PortBinding{HostPort: fmt.Sprintf("%d", mapping.HostPort)})
	}
	mounts := make([]mount.Mount, 0, len(request.Mounts))
	for _, item := range request.Mounts {
		mountType := mount.TypeVolume
		if item.Bind {
			mountType = mount.TypeBind
		}
		mounts = append(mounts, mount.Mount{Type: mountType, Source: item.Source, Target: item.Target, ReadOnly: item.ReadOnly})
	}
	devices := make([]containertypes.DeviceMapping, 0, len(request.Devices))
	for _, item := range request.Devices {
		devices = append(devices, containertypes.DeviceMapping{PathOnHost: item.HostPath, PathInContainer: item.ContainerPath, CgroupPermissions: item.Permissions})
	}
	var healthcheck *containertypes.HealthConfig
	if request.HealthCommand != "" {
		healthcheck = &containertypes.HealthConfig{Test: []string{"CMD-SHELL", request.HealthCommand}, Interval: request.HealthInterval}
	}
	result, err := m.client.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name:             request.Name,
		Config:           &containertypes.Config{Image: request.Image, Env: request.Environment, ExposedPorts: portSet, Cmd: request.Command, Entrypoint: request.Entrypoint, Hostname: request.Hostname, WorkingDir: request.WorkingDir, User: request.User, Labels: request.Labels, Healthcheck: healthcheck},
		HostConfig:       &containertypes.HostConfig{PortBindings: portBindings, Mounts: mounts, RestartPolicy: containertypes.RestartPolicy{Name: containertypes.RestartPolicyMode(request.RestartPolicy)}, Resources: containertypes.Resources{NanoCPUs: request.NanoCPUs, Memory: request.Memory, Devices: devices}, NetworkMode: containertypes.NetworkMode(request.Network), Privileged: request.Privileged, CapAdd: request.Capabilities},
		NetworkingConfig: containerNetworkingConfig(request.Network),
	})
	if err != nil {
		return "", err
	}
	return result.ID, nil
}

func containerNetworkingConfig(name string) *network.NetworkingConfig {
	if name == "" || name == "default" || name == "bridge" || name == "host" || name == "none" {
		return nil
	}
	return &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{name: {}}}
}
