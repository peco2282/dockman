package model

import "time"

type Container struct {
	ID             string
	Name           string
	Image          string
	State          string
	Status         string
	Created        time.Time
	Ports          []string
	Size           int64
	CPUPercent     float64
	MemoryUsage    uint64
	MemoryLimit    uint64
	MemoryPercent  float64
	NetworkRx      uint64
	NetworkTx      uint64
	BlockRead      uint64
	BlockWrite     uint64
	ComposeProject string
}

func (c Container) Running() bool { return c.State == "running" }

type Mount struct {
	Source      string
	Destination string
	ReadOnly    bool
}

type ContainerDetail struct {
	ID        string
	Name      string
	Image     string
	State     string
	Running   bool
	Created   time.Time
	StartedAt time.Time
	Ports     []string
	Mounts    []Mount
	TTY       bool
}

type ContainerStats struct {
	CPUPercent    float64
	MemoryUsage   uint64
	MemoryLimit   uint64
	MemoryPercent float64
	NetworkRx     uint64
	NetworkTx     uint64
	BlockRead     uint64
	BlockWrite    uint64
}

type ContainerCreateRequest struct {
	Name           string
	Image          string
	Ports          []PortMapping
	Mounts         []MountSpec
	Environment    []string
	RestartPolicy  string
	Command        []string
	Entrypoint     []string
	Network        string
	Hostname       string
	WorkingDir     string
	User           string
	Labels         map[string]string
	NanoCPUs       int64
	Memory         int64
	Start          bool
	Privileged     bool
	Capabilities   []string
	Devices        []DeviceSpec
	HealthCommand  string
	HealthInterval time.Duration
}

type DeviceSpec struct{ HostPath, ContainerPath, Permissions string }

type PortMapping struct {
	HostPort      uint16
	ContainerPort uint16
	Protocol      string
}

type MountSpec struct {
	Source   string
	Target   string
	ReadOnly bool
	Bind     bool
}
