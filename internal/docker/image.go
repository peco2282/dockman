package docker

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/moby/moby/client"

	"dockman/internal/model"
)

func (m *Manager) ListImages(ctx context.Context) ([]model.Image, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.client == nil {
		return nil, ErrNotConnected
	}
	result, err := m.client.ImageList(ctx, client.ImageListOptions{})
	if err != nil {
		return nil, err
	}
	var images []model.Image
	for _, item := range result.Items {
		tags := item.RepoTags
		if len(tags) == 0 {
			tags = []string{"<none>:<none>"}
		}
		for _, fullTag := range tags {
			repository, tag := splitImageTag(fullTag)
			images = append(images, model.Image{ID: item.ID, Repository: repository, Tag: tag, Size: item.Size, Created: time.Unix(item.Created, 0), Containers: item.Containers})
		}
	}
	return images, nil
}

func (m *Manager) InspectImage(ctx context.Context, id string) (model.ImageDetail, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.client == nil {
		return model.ImageDetail{}, ErrNotConnected
	}
	result, err := m.client.ImageInspect(ctx, id)
	if err != nil {
		return model.ImageDetail{}, err
	}
	created, _ := time.Parse(time.RFC3339Nano, result.Created)
	return model.ImageDetail{ID: result.ID, RepoTags: result.RepoTags, RepoDigests: result.RepoDigests, Size: result.Size, Created: created, Architecture: result.Architecture, OS: result.Os, Author: result.Author, Layers: len(result.RootFS.Layers)}, nil
}

func (m *Manager) PullImage(ctx context.Context, reference string) (<-chan model.PullProgress, error) {
	m.mu.RLock()
	if m.client == nil {
		m.mu.RUnlock()
		return nil, ErrNotConnected
	}
	response, err := m.client.ImagePull(ctx, reference, client.ImagePullOptions{})
	m.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	progress := make(chan model.PullProgress, 64)
	go func() {
		defer close(progress)
		for message, streamErr := range response.JSONMessages(ctx) {
			if streamErr != nil {
				select {
				case progress <- model.PullProgress{Err: streamErr, Done: true}:
				case <-ctx.Done():
				}
				return
			}
			if message.Error != nil {
				select {
				case progress <- model.PullProgress{Err: errors.New(message.Error.Message), Done: true}:
				case <-ctx.Done():
				}
				return
			}
			status := strings.TrimSpace(strings.TrimSpace(message.ID+" ") + message.Status)
			if status != "" {
				select {
				case progress <- model.PullProgress{Status: status}:
				case <-ctx.Done():
					return
				}
			}
		}
		select {
		case progress <- model.PullProgress{Status: "Pull completed", Done: true}:
		case <-ctx.Done():
		}
	}()
	return progress, nil
}

func (m *Manager) RemoveImage(ctx context.Context, id string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.client == nil {
		return ErrNotConnected
	}
	_, err := m.client.ImageRemove(ctx, id, client.ImageRemoveOptions{Force: false, PruneChildren: true})
	return err
}

func splitImageTag(value string) (string, string) {
	if i := strings.LastIndex(value, "@"); i >= 0 {
		return value[:i], value[i+1:]
	}
	slash := strings.LastIndex(value, "/")
	colon := strings.LastIndex(value, ":")
	if colon > slash {
		return value[:colon], value[colon+1:]
	}
	return value, "latest"
}
