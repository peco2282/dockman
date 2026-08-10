package service

import (
	"context"
	"errors"
	"strings"
	"time"
)

type DockerConnector interface {
	Connect(context.Context) error
}

type DockerContextConnector interface {
	ListContexts(context.Context) ([]string, string, error)
	SwitchContext(context.Context, string) error
}

type ConnectionErrorKind int

const (
	ConnectionUnavailable ConnectionErrorKind = iota
	ConnectionPermissionDenied
)

type ConnectionError struct {
	Kind ConnectionErrorKind
	Err  error
}

func (s *ConnectionService) ListContexts(ctx context.Context) ([]string, string, error) {
	c, ok := s.connector.(DockerContextConnector)
	if !ok {
		return nil, "", errors.New("Docker contexts are unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return c.ListContexts(ctx)
}

func (s *ConnectionService) SwitchContext(ctx context.Context, name string) error {
	c, ok := s.connector.(DockerContextConnector)
	if !ok {
		return errors.New("Docker contexts are unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return c.SwitchContext(ctx, name)
}

func (e *ConnectionError) Error() string { return e.Err.Error() }
func (e *ConnectionError) Unwrap() error { return e.Err }

type ConnectionService struct {
	connector DockerConnector
	timeout   time.Duration
}

func NewConnectionService(connector DockerConnector) *ConnectionService {
	return &ConnectionService{connector: connector, timeout: 5 * time.Second}
}

func (s *ConnectionService) Check(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	err := s.connector.Connect(ctx)
	if err == nil {
		return nil
	}
	kind := ConnectionUnavailable
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "permission denied") || strings.Contains(message, "access is denied") {
		kind = ConnectionPermissionDenied
	}
	return &ConnectionError{Kind: kind, Err: err}
}
