package service

import (
	"context"
	"time"

	"dockman/internal/model"
)

type SystemRepository interface {
	SystemInfo(context.Context) (model.SystemInfo, error)
	AnalyzeCleanup(context.Context) (model.CleanupAnalysis, error)
	Prune(context.Context, model.CleanupSelection) (model.CleanupResult, error)
}

func (s *SystemService) AnalyzeCleanup(ctx context.Context) (model.CleanupAnalysis, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	return s.repository.AnalyzeCleanup(ctx)
}

func (s *SystemService) Prune(ctx context.Context, selection model.CleanupSelection) (model.CleanupResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	return s.repository.Prune(ctx, selection)
}

type SystemService struct {
	repository SystemRepository
	timeout    time.Duration
}

func NewSystemService(repository SystemRepository) *SystemService {
	return &SystemService{repository: repository, timeout: 15 * time.Second}
}
func (s *SystemService) Info(ctx context.Context) (model.SystemInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.repository.SystemInfo(ctx)
}
