package service

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"dockman/internal/model"
)

type ContainerRepository interface {
	ListContainers(context.Context) ([]model.Container, error)
	InspectContainer(context.Context, string) (model.ContainerDetail, error)
	StartContainer(context.Context, string) error
	StopContainer(context.Context, string) error
	RestartContainer(context.Context, string) error
	ContainerStats(context.Context, string) (model.ContainerStats, error)
	ContainerLogs(context.Context, string) (io.ReadCloser, error)
	RemoveContainer(context.Context, string, bool) error
	CreateContainer(context.Context, model.ContainerCreateRequest) (string, error)
	OpenContainerTerminal(context.Context, string, uint, uint) (model.TerminalSession, error)
}

type ContainerCreateInput struct {
	Name           string
	Image          string
	Ports          string
	Mounts         string
	Environment    string
	RestartPolicy  string
	Command        string
	Entrypoint     string
	Network        string
	Hostname       string
	WorkingDir     string
	User           string
	Labels         string
	CPUs           string
	Memory         string
	Start          bool
	Privileged     bool
	Capabilities   string
	Devices        string
	HealthCommand  string
	HealthInterval string
}

var containerNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type ContainerDeleteMode int

const (
	DeleteStopped ContainerDeleteMode = iota
	StopThenDelete
	ForceDelete
)

type LogEvent struct {
	Line string
	Err  error
	Done bool
}

type ContainerAction int

const (
	ContainerStart ContainerAction = iota
	ContainerStop
	ContainerRestart
)

func (a ContainerAction) String() string {
	switch a {
	case ContainerStart:
		return "Start"
	case ContainerStop:
		return "Stop"
	case ContainerRestart:
		return "Restart"
	default:
		return "Unknown"
	}
}

type ContainerService struct {
	repository ContainerRepository
	timeout    time.Duration
}

func NewContainerService(repository ContainerRepository) *ContainerService {
	return &ContainerService{repository: repository, timeout: 15 * time.Second}
}

func (s *ContainerService) List(ctx context.Context) ([]model.Container, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	items, err := s.repository.ListContainers(ctx)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if !items[i].Running() {
			continue
		}
		if stats, statsErr := s.repository.ContainerStats(ctx, items[i].ID); statsErr == nil {
			items[i].CPUPercent = stats.CPUPercent
			items[i].MemoryUsage = stats.MemoryUsage
			items[i].MemoryLimit = stats.MemoryLimit
			items[i].MemoryPercent = stats.MemoryPercent
			items[i].NetworkRx = stats.NetworkRx
			items[i].NetworkTx = stats.NetworkTx
			items[i].BlockRead = stats.BlockRead
			items[i].BlockWrite = stats.BlockWrite
		}
	}
	return items, nil
}

func (s *ContainerService) Detail(ctx context.Context, id string) (model.ContainerDetail, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.repository.InspectContainer(ctx, id)
}

func (s *ContainerService) Execute(ctx context.Context, id string, action ContainerAction) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	switch action {
	case ContainerStart:
		return s.repository.StartContainer(ctx, id)
	case ContainerStop:
		return s.repository.StopContainer(ctx, id)
	case ContainerRestart:
		return s.repository.RestartContainer(ctx, id)
	default:
		return fmt.Errorf("unsupported container action: %d", action)
	}
}

func (s *ContainerService) Stats(ctx context.Context, id string) (model.ContainerStats, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.repository.ContainerStats(ctx, id)
}

func (s *ContainerService) FollowLogs(ctx context.Context, id string) (<-chan LogEvent, error) {
	reader, err := s.repository.ContainerLogs(ctx, id)
	if err != nil {
		return nil, err
	}
	events := make(chan LogEvent, 128)
	go func() {
		defer close(events)
		defer reader.Close()
		scanner := bufio.NewScanner(reader)
		buffer := make([]byte, 64*1024)
		scanner.Buffer(buffer, 1024*1024)
		for scanner.Scan() {
			select {
			case events <- LogEvent{Line: scanner.Text()}:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil && ctx.Err() == nil {
			select {
			case events <- LogEvent{Err: err, Done: true}:
			case <-ctx.Done():
			}
			return
		}
		select {
		case events <- LogEvent{Done: true}:
		case <-ctx.Done():
		}
	}()
	return events, nil
}

func (s *ContainerService) Delete(ctx context.Context, id string, mode ContainerDeleteMode) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	switch mode {
	case DeleteStopped:
		return s.repository.RemoveContainer(ctx, id, false)
	case StopThenDelete:
		if err := s.repository.StopContainer(ctx, id); err != nil {
			return fmt.Errorf("stop container: %w", err)
		}
		return s.repository.RemoveContainer(ctx, id, false)
	case ForceDelete:
		return s.repository.RemoveContainer(ctx, id, true)
	default:
		return fmt.Errorf("unsupported delete mode: %d", mode)
	}
}

func (s *ContainerService) Create(ctx context.Context, input ContainerCreateInput) (string, error) {
	request, err := validateContainerCreateInput(input)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	id, err := s.repository.CreateContainer(ctx, request)
	if err != nil {
		return "", err
	}
	if request.Start {
		if err := s.repository.StartContainer(ctx, id); err != nil {
			return id, fmt.Errorf("container was created but could not be started: %w", err)
		}
	}
	return id, nil
}

func (s *ContainerService) OpenTerminal(ctx context.Context, id string, width, height uint) (model.TerminalSession, <-chan model.TerminalEvent, error) {
	if width == 0 {
		width = 80
	}
	if height == 0 {
		height = 24
	}
	session, err := s.repository.OpenContainerTerminal(ctx, id, width, height)
	if err != nil {
		return nil, nil, err
	}
	events := make(chan model.TerminalEvent, 64)
	go func() {
		defer close(events)
		buffer := make([]byte, 32*1024)
		for {
			n, readErr := session.Read(buffer)
			if n > 0 {
				data := append([]byte(nil), buffer[:n]...)
				select {
				case events <- model.TerminalEvent{Data: data}:
				case <-ctx.Done():
					return
				}
			}
			if readErr != nil {
				if readErr == io.EOF || ctx.Err() != nil {
					select {
					case events <- model.TerminalEvent{Done: true}:
					case <-ctx.Done():
					}
					return
				}
				select {
				case events <- model.TerminalEvent{Err: readErr, Done: true}:
				case <-ctx.Done():
				}
				return
			}
		}
	}()
	return session, events, nil
}

func validateContainerCreateInput(input ContainerCreateInput) (model.ContainerCreateRequest, error) {
	request := model.ContainerCreateRequest{Name: strings.TrimSpace(input.Name), Image: strings.TrimSpace(input.Image), RestartPolicy: input.RestartPolicy, Network: strings.TrimSpace(input.Network), Hostname: strings.TrimSpace(input.Hostname), WorkingDir: strings.TrimSpace(input.WorkingDir), User: strings.TrimSpace(input.User), Start: input.Start, Privileged: input.Privileged, HealthCommand: strings.TrimSpace(input.HealthCommand)}
	if request.Name == "" || !containerNamePattern.MatchString(request.Name) {
		return request, errors.New("name must contain only letters, numbers, dots, dashes, or underscores")
	}
	if request.Image == "" || strings.ContainsAny(request.Image, " \t\r\n") {
		return request, errors.New("enter a valid image name, for example nginx:latest")
	}
	validPolicies := map[string]bool{"no": true, "always": true, "on-failure": true, "unless-stopped": true}
	if !validPolicies[request.RestartPolicy] {
		return request, errors.New("select a valid restart policy")
	}
	for _, entry := range splitCommaList(input.Ports) {
		parts := strings.Split(entry, ":")
		if len(parts) != 2 {
			return request, fmt.Errorf("invalid port mapping %q; use HOST:CONTAINER", entry)
		}
		host, err := parsePortNumber(parts[0])
		if err != nil {
			return request, fmt.Errorf("invalid host port in %q", entry)
		}
		containerPart := parts[1]
		protocol := "tcp"
		if value, proto, ok := strings.Cut(containerPart, "/"); ok {
			containerPart, protocol = value, strings.ToLower(proto)
		}
		if protocol != "tcp" && protocol != "udp" && protocol != "sctp" {
			return request, fmt.Errorf("invalid protocol in %q", entry)
		}
		containerPort, err := parsePortNumber(containerPart)
		if err != nil {
			return request, fmt.Errorf("invalid container port in %q", entry)
		}
		request.Ports = append(request.Ports, model.PortMapping{HostPort: host, ContainerPort: containerPort, Protocol: protocol})
	}
	for _, entry := range splitCommaList(input.Mounts) {
		parts := strings.Split(entry, ":")
		if len(parts) < 2 || len(parts) > 3 {
			return request, fmt.Errorf("invalid mount %q; use SOURCE:TARGET", entry)
		}
		source, target := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if source == "" || target == "" || !strings.HasPrefix(target, "/") {
			return request, fmt.Errorf("mount target must be an absolute path in %q", entry)
		}
		readOnly := false
		if len(parts) == 3 {
			if parts[2] != "ro" {
				return request, fmt.Errorf("invalid mount option in %q; only ro is supported", entry)
			}
			readOnly = true
		}
		bind := filepath.IsAbs(source) || strings.HasPrefix(source, "/")
		if !bind && !volumeNamePattern.MatchString(source) {
			return request, fmt.Errorf("invalid volume name in %q", entry)
		}
		request.Mounts = append(request.Mounts, model.MountSpec{Source: source, Target: target, ReadOnly: readOnly, Bind: bind})
	}
	for _, entry := range splitCommaList(input.Environment) {
		name, _, ok := strings.Cut(entry, "=")
		if !ok || !envNamePattern.MatchString(strings.TrimSpace(name)) {
			return request, fmt.Errorf("invalid environment variable %q; use NAME=VALUE", entry)
		}
		request.Environment = append(request.Environment, entry)
	}
	request.Command = strings.Fields(input.Command)
	request.Entrypoint = strings.Fields(input.Entrypoint)
	if request.WorkingDir != "" && !strings.HasPrefix(request.WorkingDir, "/") {
		return request, errors.New("working directory must be an absolute container path")
	}
	request.Labels = map[string]string{}
	for _, entry := range splitCommaList(input.Labels) {
		key, value, ok := strings.Cut(entry, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return request, fmt.Errorf("invalid label %q; use NAME=VALUE", entry)
		}
		request.Labels[key] = strings.TrimSpace(value)
	}
	if strings.TrimSpace(input.CPUs) != "" {
		cpus, err := strconv.ParseFloat(strings.TrimSpace(input.CPUs), 64)
		if err != nil || cpus <= 0 {
			return request, errors.New("CPU limit must be a positive number")
		}
		request.NanoCPUs = int64(cpus * 1e9)
	}
	if strings.TrimSpace(input.Memory) != "" {
		memory, err := parseMemory(input.Memory)
		if err != nil {
			return request, err
		}
		request.Memory = memory
	}
	for _, capability := range splitCommaList(input.Capabilities) {
		request.Capabilities = append(request.Capabilities, strings.ToUpper(capability))
	}
	for _, entry := range splitCommaList(input.Devices) {
		parts := strings.Split(entry, ":")
		if len(parts) < 2 || len(parts) > 3 {
			return request, fmt.Errorf("invalid device %q; use HOST:CONTAINER[:PERMISSIONS]", entry)
		}
		permissions := "rwm"
		if len(parts) == 3 {
			permissions = parts[2]
		}
		if strings.Trim(permissions, "rwm") != "" || permissions == "" {
			return request, fmt.Errorf("invalid device permissions in %q", entry)
		}
		request.Devices = append(request.Devices, model.DeviceSpec{HostPath: parts[0], ContainerPath: parts[1], Permissions: permissions})
	}
	if strings.TrimSpace(input.HealthInterval) != "" {
		interval, err := time.ParseDuration(strings.TrimSpace(input.HealthInterval))
		if err != nil || interval <= 0 {
			return request, errors.New("health interval must be a duration such as 30s or 1m")
		}
		request.HealthInterval = interval
	}
	return request, nil
}

func parseMemory(value string) (int64, error) {
	v := strings.ToLower(strings.TrimSpace(value))
	multiplier := int64(1)
	for suffix, m := range map[string]int64{"k": 1024, "kb": 1024, "m": 1024 * 1024, "mb": 1024 * 1024, "g": 1024 * 1024 * 1024, "gb": 1024 * 1024 * 1024} {
		if strings.HasSuffix(v, suffix) {
			v = strings.TrimSpace(strings.TrimSuffix(v, suffix))
			multiplier = m
			break
		}
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil || n <= 0 {
		return 0, errors.New("memory limit must be a positive size such as 512m or 2g")
	}
	return int64(n * float64(multiplier)), nil
}

func splitCommaList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	raw := strings.Split(value, ",")
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
func parsePortNumber(value string) (uint16, error) {
	number, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || number < 1 || number > 65535 {
		return 0, errors.New("port must be between 1 and 65535")
	}
	return uint16(number), nil
}
