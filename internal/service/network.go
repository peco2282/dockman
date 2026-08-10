package service

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"dockman/internal/model"
)

var networkNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]+$`)

type NetworkRepository interface {
	ListNetworks(context.Context) ([]model.Network, error)
	InspectNetwork(context.Context, string) (model.Network, error)
	CreateNetwork(context.Context, string, string) (model.Network, error)
	RemoveNetwork(context.Context, string) error
	ConnectNetwork(context.Context, string, string) error
	DisconnectNetwork(context.Context, string, string) error
}

type NetworkService struct {
	repository NetworkRepository
	timeout    time.Duration
}

func NewNetworkService(repository NetworkRepository) *NetworkService {
	return &NetworkService{repository: repository, timeout: 15 * time.Second}
}

func (s *NetworkService) List(ctx context.Context) ([]model.Network, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.repository.ListNetworks(ctx)
}

func (s *NetworkService) Detail(ctx context.Context, id string) (model.Network, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.repository.InspectNetwork(ctx, id)
}

func (s *NetworkService) Create(ctx context.Context, name, driver string) (model.Network, error) {
	name, driver = strings.TrimSpace(name), strings.TrimSpace(driver)
	if !networkNamePattern.MatchString(name) {
		return model.Network{}, errors.New("use at least 2 letters, numbers, dots, dashes, or underscores")
	}
	if driver == "" {
		driver = "bridge"
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.repository.CreateNetwork(ctx, name, driver)
}

func (s *NetworkService) Delete(ctx context.Context, id string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return s.repository.RemoveNetwork(ctx, id)
}

func (s *NetworkService) Connect(ctx context.Context, networkID, containerID string) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.repository.ConnectNetwork(ctx, networkID, containerID)
}

func (s *NetworkService) Disconnect(ctx context.Context, networkID, containerID string) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.repository.DisconnectNetwork(ctx, networkID, containerID)
}
