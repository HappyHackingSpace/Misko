package application

import (
	"context"
	"sync/atomic"
	"time"
)

// Dependency checks infrastructure without exposing its implementation to the use case.
type Dependency interface{ Check(context.Context) error }

type Service struct {
	dependency Dependency
	timeout    time.Duration
	draining   atomic.Bool
}

func New(dependency Dependency, timeout time.Duration) *Service {
	return &Service{dependency: dependency, timeout: timeout}
}

func (s *Service) BeginDrain() { s.draining.Store(true) }

func (s *Service) Ready(ctx context.Context) bool {
	if s.draining.Load() {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	err := s.dependency.Check(ctx)
	return err == nil && ctx.Err() == nil && !s.draining.Load()
}
