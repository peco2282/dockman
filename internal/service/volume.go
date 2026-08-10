package service

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"dockman/internal/model"
)

var volumeNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]+$`)
var anonymousVolumePattern = regexp.MustCompile(`^[a-f0-9]{32,}$`)

type VolumeRepository interface {
	ListVolumes(context.Context) ([]model.Volume, error)
	InspectVolume(context.Context, string) (model.Volume, error)
	CreateVolume(context.Context, string) (model.Volume, error)
	RemoveVolume(context.Context, string) error
}
type VolumeService struct {
	repository VolumeRepository
	timeout    time.Duration
}

func NewVolumeService(repository VolumeRepository) *VolumeService {
	return &VolumeService{repository: repository, timeout: 15 * time.Second}
}
func (s *VolumeService) List(ctx context.Context) ([]model.Volume, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.repository.ListVolumes(ctx)
}
func (s *VolumeService) Detail(ctx context.Context, name string) (model.Volume, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.repository.InspectVolume(ctx, name)
}
func (s *VolumeService) Create(ctx context.Context, name string) (model.Volume, error) {
	name = strings.TrimSpace(name)
	if !volumeNamePattern.MatchString(name) {
		return model.Volume{}, errors.New("use at least 2 letters, numbers, dots, dashes, or underscores")
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.repository.CreateVolume(ctx, name)
}
func (s *VolumeService) Delete(ctx context.Context, name, confirmation string) error {
	if confirmation != VolumeDeleteConfirmation(name) {
		return errors.New("confirmation does not match the requested value")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return s.repository.RemoveVolume(ctx, name)
}

// VolumeDeleteConfirmation shortens confirmation only for long hexadecimal
// names generated for anonymous volumes. Named volumes still require the full
// name so a familiar, meaningful name cannot be deleted by a short prefix.
func VolumeDeleteConfirmation(name string) string {
	if anonymousVolumePattern.MatchString(name) {
		return name[len(name)-8:]
	}
	return name
}
