package docker

import (
	"context"
	"fmt"
	"strings"

	"github.com/moby/moby/client"

	"dockman/internal/model"
)

func (m *Manager) SystemInfo(ctx context.Context) (model.SystemInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.client == nil {
		return model.SystemInfo{}, ErrNotConnected
	}
	infoResult, err := m.client.Info(ctx, client.InfoOptions{})
	if err != nil {
		return model.SystemInfo{}, err
	}
	version, err := m.client.ServerVersion(ctx, client.ServerVersionOptions{})
	if err != nil {
		return model.SystemInfo{}, err
	}
	info := infoResult.Info
	return model.SystemInfo{DockerVersion: version.Version, APIVersion: version.APIVersion, MinAPIVersion: version.MinAPIVersion, Platform: version.Platform.Name, Containers: info.Containers, Running: info.ContainersRunning, Paused: info.ContainersPaused, Stopped: info.ContainersStopped, Images: info.Images, HostName: info.Name, OperatingSystem: info.OperatingSystem, KernelVersion: info.KernelVersion, Architecture: info.Architecture, CPUs: info.NCPU, MemoryBytes: info.MemTotal, StorageDriver: info.Driver, LoggingDriver: info.LoggingDriver, CgroupDriver: info.CgroupDriver, CgroupVersion: info.CgroupVersion, DockerRootDir: info.DockerRootDir, DefaultRuntime: info.DefaultRuntime, LiveRestore: info.LiveRestoreEnabled, ExperimentalBuild: info.ExperimentalBuild}, nil
}

func (m *Manager) AnalyzeCleanup(ctx context.Context) (model.CleanupAnalysis, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.client == nil {
		return model.CleanupAnalysis{}, ErrNotConnected
	}
	usage, err := m.client.DiskUsage(ctx, client.DiskUsageOptions{Containers: true, Images: true, Volumes: true, BuildCache: true, Verbose: true})
	if err != nil {
		return model.CleanupAnalysis{}, err
	}
	result := model.CleanupAnalysis{ContainersTotal: usage.Containers.TotalSize, ImagesTotal: usage.Images.TotalSize, VolumesTotal: usage.Volumes.TotalSize, BuildCacheTotal: usage.BuildCache.TotalSize}
	for _, c := range usage.Containers.Items {
		if c.State != "running" && c.State != "paused" {
			name := strings.TrimPrefix(first(c.Names), "/")
			result.Items = append(result.Items, model.CleanupItem{Kind: model.CleanupContainers, ID: c.ID, Name: name, Size: c.SizeRw})
		}
	}
	for _, image := range usage.Images.Items {
		if len(image.RepoTags) == 0 && image.Containers <= 0 {
			result.Items = append(result.Items, model.CleanupItem{Kind: model.CleanupImages, ID: image.ID, Name: "<dangling>", Size: image.Size})
		}
	}
	for _, volume := range usage.Volumes.Items {
		if volume.UsageData != nil && volume.UsageData.RefCount == 0 {
			result.Items = append(result.Items, model.CleanupItem{Kind: model.CleanupVolumes, ID: volume.Name, Name: volume.Name, Size: volume.UsageData.Size})
		}
	}
	networks, err := m.client.NetworkList(ctx, client.NetworkListOptions{})
	if err != nil {
		return model.CleanupAnalysis{}, err
	}
	for _, network := range networks.Items {
		if network.Name == "bridge" || network.Name == "host" || network.Name == "none" || network.Name == "nat" {
			continue
		}
		inspected, inspectErr := m.client.NetworkInspect(ctx, network.ID, client.NetworkInspectOptions{})
		if inspectErr != nil {
			return model.CleanupAnalysis{}, inspectErr
		}
		if len(inspected.Network.Containers) == 0 {
			result.Items = append(result.Items, model.CleanupItem{Kind: model.CleanupNetworks, ID: network.ID, Name: network.Name})
		}
	}
	for _, cache := range usage.BuildCache.Items {
		if !cache.InUse {
			result.Items = append(result.Items, model.CleanupItem{Kind: model.CleanupBuildCache, ID: cache.ID, Name: cache.Description, Size: cache.Size})
		}
	}
	return result, nil
}

func (m *Manager) Prune(ctx context.Context, selection model.CleanupSelection) (model.CleanupResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.client == nil {
		return model.CleanupResult{}, ErrNotConnected
	}
	result := model.CleanupResult{Deleted: map[model.CleanupKind]int{}}
	if selection.Containers {
		r, err := m.client.ContainerPrune(ctx, client.ContainerPruneOptions{})
		if err != nil {
			return result, fmt.Errorf("prune containers: %w", err)
		}
		result.Deleted[model.CleanupContainers] = len(r.Report.ContainersDeleted)
		result.SpaceReclaimed += r.Report.SpaceReclaimed
	}
	if selection.Images {
		r, err := m.client.ImagePrune(ctx, client.ImagePruneOptions{Filters: client.Filters{"dangling": {"true": true}}})
		if err != nil {
			return result, fmt.Errorf("prune images: %w", err)
		}
		result.Deleted[model.CleanupImages] = len(r.Report.ImagesDeleted)
		result.SpaceReclaimed += r.Report.SpaceReclaimed
	}
	if selection.Volumes {
		r, err := m.client.VolumePrune(ctx, client.VolumePruneOptions{All: true})
		if err != nil {
			return result, fmt.Errorf("prune volumes: %w", err)
		}
		result.Deleted[model.CleanupVolumes] = len(r.Report.VolumesDeleted)
		result.SpaceReclaimed += r.Report.SpaceReclaimed
	}
	if selection.Networks {
		r, err := m.client.NetworkPrune(ctx, client.NetworkPruneOptions{})
		if err != nil {
			return result, fmt.Errorf("prune networks: %w", err)
		}
		result.Deleted[model.CleanupNetworks] = len(r.Report.NetworksDeleted)
	}
	if selection.BuildCache {
		r, err := m.client.BuildCachePrune(ctx, client.BuildCachePruneOptions{All: true})
		if err != nil {
			return result, fmt.Errorf("prune build cache: %w", err)
		}
		result.Deleted[model.CleanupBuildCache] = len(r.Report.CachesDeleted)
		result.SpaceReclaimed += r.Report.SpaceReclaimed
	}
	return result, nil
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
