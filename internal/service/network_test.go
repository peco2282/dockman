package service

import (
	"context"
	"testing"

	"dockman/internal/model"
)

type fakeNetworkRepository struct{ createdName, createdDriver, connected, disconnected, deleted string }

func (*fakeNetworkRepository) ListNetworks(context.Context) ([]model.Network, error) { return nil, nil }
func (*fakeNetworkRepository) InspectNetwork(context.Context, string) (model.Network, error) {
	return model.Network{}, nil
}
func (f *fakeNetworkRepository) CreateNetwork(_ context.Context, name, driver string) (model.Network, error) {
	f.createdName, f.createdDriver = name, driver
	return model.Network{Name: name, Driver: driver}, nil
}
func (f *fakeNetworkRepository) RemoveNetwork(_ context.Context, id string) error {
	f.deleted = id
	return nil
}
func (f *fakeNetworkRepository) ConnectNetwork(_ context.Context, n, c string) error {
	f.connected = n + ":" + c
	return nil
}
func (f *fakeNetworkRepository) DisconnectNetwork(_ context.Context, n, c string) error {
	f.disconnected = n + ":" + c
	return nil
}

func TestNetworkServiceValidatesAndDefaultsDriver(t *testing.T) {
	r := &fakeNetworkRepository{}
	s := NewNetworkService(r)
	if _, err := s.Create(context.Background(), "bad name", ""); err == nil {
		t.Fatal("invalid name accepted")
	}
	if _, err := s.Create(context.Background(), "app-net", ""); err != nil {
		t.Fatal(err)
	}
	if r.createdName != "app-net" || r.createdDriver != "bridge" {
		t.Fatalf("create = %q %q", r.createdName, r.createdDriver)
	}
}

func TestNetworkServiceConnectDisconnect(t *testing.T) {
	r := &fakeNetworkRepository{}
	s := NewNetworkService(r)
	if err := s.Connect(context.Background(), "net", "container"); err != nil {
		t.Fatal(err)
	}
	if err := s.Disconnect(context.Background(), "net", "container"); err != nil {
		t.Fatal(err)
	}
	if r.connected != "net:container" || r.disconnected != "net:container" {
		t.Fatalf("calls = %q %q", r.connected, r.disconnected)
	}
}
