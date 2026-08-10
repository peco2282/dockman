package service

import (
	"context"
	"testing"

	"dockman/internal/model"
)

type fakeSystemRepository struct{}

func (fakeSystemRepository) SystemInfo(context.Context) (model.SystemInfo, error) {
	return model.SystemInfo{DockerVersion: "29.0.0", Containers: 12, Running: 8, CPUs: 4}, nil
}
func (fakeSystemRepository) AnalyzeCleanup(context.Context) (model.CleanupAnalysis, error) {
	return model.CleanupAnalysis{Items: []model.CleanupItem{{Kind: model.CleanupImages, Size: 42}}}, nil
}
func (fakeSystemRepository) Prune(context.Context, model.CleanupSelection) (model.CleanupResult, error) {
	return model.CleanupResult{SpaceReclaimed: 42}, nil
}
func TestSystemServiceInfo(t *testing.T) {
	service := NewSystemService(fakeSystemRepository{})
	info, err := service.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.DockerVersion != "29.0.0" || info.Running != 8 || info.CPUs != 4 {
		t.Fatalf("info = %#v", info)
	}
}

func TestSystemServiceCleanup(t *testing.T) {
	service := NewSystemService(fakeSystemRepository{})
	analysis, err := service.AnalyzeCleanup(context.Background())
	if err != nil || len(analysis.Items) != 1 {
		t.Fatalf("analysis = %#v, err = %v", analysis, err)
	}
	result, err := service.Prune(context.Background(), model.CleanupSelection{Images: true})
	if err != nil || result.SpaceReclaimed != 42 {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
}
