// Package application contains laboratory use cases for the singleton laboratory.
package application

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/laboratory/domain"
)

var (
	ErrNotInitialized = errors.New("laboratory has not been initialized")
	ErrNoChanges      = errors.New("no fields to update")
)

// Change holds validated fields; nil leaves a field unchanged and an empty Code clears it.
type Change struct {
	Name, Code, Timezone *string
}

type Store interface {
	// Laboratory returns ErrNotInitialized before installation setup.
	Laboratory(ctx context.Context) (domain.Laboratory, error)
	// Create inserts the singleton unless one exists and reports whether it did.
	Create(ctx context.Context, lab domain.Laboratory) (bool, error)
	// Update applies a change atomically.
	Update(ctx context.Context, change Change) (domain.Laboratory, error)
}

type Service struct{ store Store }

func New(store Store) *Service { return &Service{store: store} }

func (s *Service) Laboratory(ctx context.Context, actor access.Actor) (domain.Laboratory, error) {
	if err := actor.Require(access.Read); err != nil {
		return domain.Laboratory{}, err
	}
	return s.store.Laboratory(ctx)
}

// PublicName is the unauthenticated branding shown on the sign-in screen.
func (s *Service) PublicName(ctx context.Context) (string, error) {
	lab, err := s.store.Laboratory(ctx)
	return lab.Name, err
}

type Update struct {
	Name, Code, Timezone *string
}

func (s *Service) Update(ctx context.Context, actor access.Actor, in Update) (domain.Laboratory, error) {
	if err := actor.Require(access.LabConfigure); err != nil {
		return domain.Laboratory{}, err
	}
	if in.Name == nil && in.Code == nil && in.Timezone == nil {
		return domain.Laboratory{}, ErrNoChanges
	}
	var change Change
	if in.Name != nil {
		name, err := domain.NormalizeName(*in.Name)
		if err != nil {
			return domain.Laboratory{}, err
		}
		change.Name = &name
	}
	if in.Code != nil {
		code, err := domain.NormalizeCode(*in.Code)
		if err != nil {
			return domain.Laboratory{}, err
		}
		change.Code = &code
	}
	if in.Timezone != nil {
		if err := domain.ValidateTimezone(*in.Timezone); err != nil {
			return domain.Laboratory{}, err
		}
		change.Timezone = in.Timezone
	}
	return s.store.Update(ctx, change)
}

// Initialize creates the singleton laboratory once. Later calls report false
// and change nothing.
func (s *Service) Initialize(ctx context.Context, name, code, timezone string) (bool, error) {
	lab, err := domain.NewLaboratory(name, code, timezone)
	if err != nil {
		return false, err
	}
	return s.store.Create(ctx, lab)
}
