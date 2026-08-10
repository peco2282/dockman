package docker

import (
	"context"
	"errors"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"

	"dockman/internal/model"
)

func (m *Manager) ListVolumes(ctx context.Context) ([]model.Volume, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.client == nil {
		return nil, ErrNotConnected
	}
	result, err := m.client.VolumeList(ctx, client.VolumeListOptions{})
	if err != nil {
		return nil, err
	}
	used, err := m.volumeUsage(ctx)
	if err != nil {
		return nil, err
	}
	volumes := make([]model.Volume, 0, len(result.Items))
	for _, item := range result.Items {
		created, _ := time.Parse(time.RFC3339Nano, item.CreatedAt)
		volumes = append(volumes, model.Volume{Name: item.Name, Driver: item.Driver, Mountpoint: item.Mountpoint, Scope: item.Scope, Created: created, InUse: len(used[item.Name]) > 0, Containers: used[item.Name], Labels: item.Labels, Options: item.Options})
	}
	return volumes, nil
}

func (m *Manager) InspectVolume(ctx context.Context, name string) (model.Volume, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.client == nil {
		return model.Volume{}, ErrNotConnected
	}
	result, err := m.client.VolumeInspect(ctx, name, client.VolumeInspectOptions{})
	if err != nil {
		return model.Volume{}, err
	}
	used, err := m.volumeUsage(ctx)
	if err != nil {
		return model.Volume{}, err
	}
	item := result.Volume
	created, _ := time.Parse(time.RFC3339Nano, item.CreatedAt)
	return model.Volume{Name: item.Name, Driver: item.Driver, Mountpoint: item.Mountpoint, Scope: item.Scope, Created: created, InUse: len(used[item.Name]) > 0, Containers: used[item.Name], Labels: item.Labels, Options: item.Options}, nil
}

// volumeUsage runs while the caller holds m.mu.RLock.
func (m *Manager) volumeUsage(ctx context.Context) (map[string][]string, error) {
	result, err := m.client.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}
	used := map[string][]string{}
	for _, container := range result.Items {
		name := container.ID[:min(12, len(container.ID))]
		if len(container.Names) > 0 {
			name = strings.TrimPrefix(container.Names[0], "/")
		}
		for _, mount := range container.Mounts {
			if mount.Name != "" {
				used[mount.Name] = append(used[mount.Name], name)
			}
		}
	}
	return used, nil
}

func (m *Manager) CreateVolume(ctx context.Context, name string) (model.Volume, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.client == nil {
		return model.Volume{}, ErrNotConnected
	}
	if _, err := m.client.VolumeInspect(ctx, name, client.VolumeInspectOptions{}); err == nil {
		return model.Volume{}, errors.New("a volume with this name already exists")
	} else if !cerrdefs.IsNotFound(err) {
		return model.Volume{}, err
	}
	result, err := m.client.VolumeCreate(ctx, client.VolumeCreateOptions{Name: name, Driver: "local"})
	if err != nil {
		return model.Volume{}, err
	}
	item := result.Volume
	created, _ := time.Parse(time.RFC3339Nano, item.CreatedAt)
	return model.Volume{Name: item.Name, Driver: item.Driver, Mountpoint: item.Mountpoint, Scope: item.Scope, Created: created}, nil
}

func (m *Manager) RemoveVolume(ctx context.Context, name string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.client == nil {
		return ErrNotConnected
	}
	_, err := m.client.VolumeRemove(ctx, name, client.VolumeRemoveOptions{Force: false})
	return err
}
