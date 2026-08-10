package service

import (
	"context"
	"testing"

	"dockman/internal/model"
)

type fakeVolumeRepository struct {
	created string
	deleted string
}

func (*fakeVolumeRepository) ListVolumes(context.Context) ([]model.Volume, error) { return nil, nil }
func (*fakeVolumeRepository) InspectVolume(context.Context, string) (model.Volume, error) {
	return model.Volume{}, nil
}
func (f *fakeVolumeRepository) CreateVolume(_ context.Context, name string) (model.Volume, error) {
	f.created = name
	return model.Volume{Name: name}, nil
}
func (f *fakeVolumeRepository) RemoveVolume(_ context.Context, name string) error {
	f.deleted = name
	return nil
}

func TestVolumeServiceValidatesName(t *testing.T) {
	s := NewVolumeService(&fakeVolumeRepository{})
	for _, name := range []string{"", "a", "bad name", "/data"} {
		if _, err := s.Create(context.Background(), name); err == nil {
			t.Errorf("Create(%q) error = nil", name)
		}
	}
}
func TestVolumeServiceRequiresExactDeleteConfirmation(t *testing.T) {
	repository := &fakeVolumeRepository{}
	s := NewVolumeService(repository)
	if err := s.Delete(context.Background(), "minecraft-data", "minecraft"); err == nil {
		t.Fatal("Delete() error = nil")
	}
	if repository.deleted != "" {
		t.Fatalf("deleted = %q", repository.deleted)
	}
	if err := s.Delete(context.Background(), "minecraft-data", "minecraft-data"); err != nil {
		t.Fatal(err)
	}
}

func TestVolumeDeleteConfirmationShortensAnonymousHash(t *testing.T) {
	name := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if got := VolumeDeleteConfirmation(name); got != "89abcdef" {
		t.Fatalf("VolumeDeleteConfirmation() = %q", got)
	}
	repository := &fakeVolumeRepository{}
	s := NewVolumeService(repository)
	if err := s.Delete(context.Background(), name, "89abcdef"); err != nil {
		t.Fatal(err)
	}
}

func TestVolumeDeleteConfirmationKeepsNamedVolume(t *testing.T) {
	if got := VolumeDeleteConfirmation("minecraft-data"); got != "minecraft-data" {
		t.Fatalf("VolumeDeleteConfirmation() = %q", got)
	}
}
