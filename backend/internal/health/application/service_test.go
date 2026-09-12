package application

import (
	"context"
	"errors"
	"testing"
	"time"
)

type dependencyFunc func(context.Context) error

func (f dependencyFunc) Check(ctx context.Context) error { return f(ctx) }

func TestReadinessReflectsDependencyAndDrain(t *testing.T) {
	for _, failed := range []bool{false, true} {
		s := New(dependencyFunc(func(context.Context) error {
			if failed {
				return errors.New("database password should not reach HTTP")
			}
			return nil
		}), time.Second)
		if got := s.Ready(context.Background()); got == failed {
			t.Fatalf("failed=%v ready=%v", failed, got)
		}
		s.BeginDrain()
		if s.Ready(context.Background()) {
			t.Fatal("draining process is ready")
		}
	}
}

func TestReadinessDeadlineAndCancellation(t *testing.T) {
	s := New(dependencyFunc(func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }), 20*time.Millisecond)
	if s.Ready(context.Background()) {
		t.Fatal("timed-out dependency marked ready")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if s.Ready(ctx) {
		t.Fatal("cancelled probe marked ready")
	}
}
