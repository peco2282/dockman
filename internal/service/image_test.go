package service

import (
	"context"
	"testing"

	"dockman/internal/model"
)

type fakeImageRepository struct{ deleted string }

func (*fakeImageRepository) ListImages(context.Context) ([]model.Image, error) {
	return []model.Image{{ID: "1", Repository: "nginx", Tag: "latest"}}, nil
}
func (*fakeImageRepository) InspectImage(context.Context, string) (model.ImageDetail, error) {
	return model.ImageDetail{ID: "1"}, nil
}
func (*fakeImageRepository) PullImage(context.Context, string) (<-chan model.PullProgress, error) {
	ch := make(chan model.PullProgress, 1)
	ch <- model.PullProgress{Done: true}
	close(ch)
	return ch, nil
}
func (f *fakeImageRepository) RemoveImage(_ context.Context, id string) error {
	f.deleted = id
	return nil
}

func TestImageServiceRejectsEmptyPullReference(t *testing.T) {
	s := NewImageService(&fakeImageRepository{})
	if _, err := s.Pull(context.Background(), "  "); err == nil {
		t.Fatal("Pull() error = nil")
	}
}
func TestImageServiceDeletesSelectedReference(t *testing.T) {
	repository := &fakeImageRepository{}
	s := NewImageService(repository)
	if err := s.Delete(context.Background(), "nginx:latest"); err != nil {
		t.Fatal(err)
	}
	if repository.deleted != "nginx:latest" {
		t.Fatalf("deleted = %q", repository.deleted)
	}
}
