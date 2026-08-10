package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"dockman/internal/model"
)

type ImageRepository interface {
	ListImages(context.Context) ([]model.Image, error)
	InspectImage(context.Context, string) (model.ImageDetail, error)
	PullImage(context.Context, string) (<-chan model.PullProgress, error)
	RemoveImage(context.Context, string) error
}

type ImageService struct {
	repository ImageRepository
	timeout    time.Duration
}

func NewImageService(repository ImageRepository) *ImageService {
	return &ImageService{repository: repository, timeout: 15 * time.Second}
}
func (s *ImageService) List(ctx context.Context) ([]model.Image, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.repository.ListImages(ctx)
}
func (s *ImageService) Detail(ctx context.Context, id string) (model.ImageDetail, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.repository.InspectImage(ctx, id)
}
func (s *ImageService) Pull(ctx context.Context, reference string) (<-chan model.PullProgress, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return nil, errors.New("enter an image name, for example nginx:latest")
	}
	if strings.ContainsAny(reference, " \t\r\n") {
		return nil, errors.New("image name cannot contain spaces")
	}
	return s.repository.PullImage(ctx, reference)
}
func (s *ImageService) Delete(ctx context.Context, id string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return s.repository.RemoveImage(ctx, id)
}
