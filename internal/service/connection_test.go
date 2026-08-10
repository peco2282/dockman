package service

import (
	"context"
	"errors"
	"testing"
)

type fakeConnector struct{ err error }

func (f fakeConnector) Connect(context.Context) error { return f.err }

func TestConnectionServiceCheck(t *testing.T) {
	s := NewConnectionService(fakeConnector{})
	if err := s.Check(context.Background()); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
}

func TestConnectionServiceClassifiesPermissionError(t *testing.T) {
	s := NewConnectionService(fakeConnector{err: errors.New("permission denied")})
	err := s.Check(context.Background())
	var connectionErr *ConnectionError
	if !errors.As(err, &connectionErr) || connectionErr.Kind != ConnectionPermissionDenied {
		t.Fatalf("Check() error = %v, want permission error", err)
	}
}
