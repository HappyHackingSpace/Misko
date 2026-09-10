// Package application contains environment use cases. Changes need
// apparatus:write and reads need *:read.
package application

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/environments/domain"
)

var (
	ErrEnvironmentNotFound    = errors.New("environment not found")
	ErrRevisionNotFound       = errors.New("environment revision not found")
	ErrNameTaken              = errors.New("name is already in use")
	ErrNoChanges              = errors.New("no fields to update")
	ErrInvalidQuery           = errors.New("invalid list query")
	ErrUnknownParadigm        = errors.New("unknown paradigm")
	ErrUnknownParadigmVersion = errors.New("unknown paradigm version")
	ErrInvalidApparatus       = errors.New("apparatus parameters must match the paradigm version: every measurement given, within range and consistent")
)

// Catalog checks measurements against the hardcoded paradigm definitions.
type Catalog interface {
	// CheckApparatus returns ErrUnknownParadigm, ErrUnknownParadigmVersion or ErrInvalidApparatus.
	CheckApparatus(key string, version int, apparatus map[string]float64) error
}

// Changes holds validated fields; nil leaves a field unchanged and empty notes clear them.
type Changes struct {
	Name, Notes *string
}

type Store interface {
	Transaction(ctx context.Context, fn func(Store) error) error
	// Environments lists environments by name, optionally for one paradigm.
	Environments(ctx context.Context, paradigmKey string) ([]domain.Environment, error)
	Environment(ctx context.Context, id string) (domain.Environment, error)
	CreateEnvironment(ctx context.Context, e domain.Environment) (domain.Environment, error)
	UpdateEnvironment(ctx context.Context, id string, c Changes) (domain.Environment, error)
	// LockEnvironment locks the environment until the transaction ends, so
	// revisions of one environment are numbered one at a time.
	LockEnvironment(ctx context.Context, id string) (domain.Environment, error)
	// CreateRevision stores r with the next number of its environment.
	CreateRevision(ctx context.Context, r domain.Revision) (domain.Revision, error)
	Revisions(ctx context.Context, environmentID string) ([]domain.Revision, error)
	Revision(ctx context.Context, environmentID string, number int) (domain.Revision, error)
}

type Service struct {
	store   Store
	catalog Catalog
}

func New(store Store, catalog Catalog) *Service { return &Service{store: store, catalog: catalog} }

type RevisionInput struct {
	ParadigmVersion int
	Apparatus       map[string]float64
	Notes           string
}

type EnvironmentInput struct {
	Name, ParadigmKey, Notes string
	Revision                 RevisionInput
}

type Patch struct {
	Name, Notes *string
}

type Created struct {
	Environment domain.Environment
	Revision    domain.Revision
}

func (s *Service) List(ctx context.Context, actor access.Actor, paradigmKey string) ([]domain.Environment, error) {
	if err := actor.Require(access.Read); err != nil {
		return nil, err
	}
	if paradigmKey != "" && domain.ValidateParadigmKey(paradigmKey) != nil {
		return nil, ErrInvalidQuery
	}
	return s.store.Environments(ctx, paradigmKey)
}

func (s *Service) Get(ctx context.Context, actor access.Actor, id string) (domain.Environment, error) {
	if err := actor.Require(access.Read); err != nil {
		return domain.Environment{}, err
	}
	return s.store.Environment(ctx, id)
}

// Create stores an environment together with its first revision.
func (s *Service) Create(ctx context.Context, actor access.Actor, in EnvironmentInput) (Created, error) {
	if err := actor.Require(access.ApparatusWrite); err != nil {
		return Created{}, err
	}
	e, err := domain.NewEnvironment(in.Name, in.ParadigmKey, in.Notes)
	if err != nil {
		return Created{}, err
	}
	r, err := domain.NewRevision("", e.ParadigmKey, in.Revision.ParadigmVersion, in.Revision.Apparatus, in.Revision.Notes, actor.UserID)
	if err != nil {
		return Created{}, err
	}
	if err := s.catalog.CheckApparatus(r.ParadigmKey, r.ParadigmVersion, r.Apparatus); err != nil {
		return Created{}, err
	}
	var out Created
	err = s.store.Transaction(ctx, func(tx Store) error {
		created, err := tx.CreateEnvironment(ctx, e)
		if err != nil {
			return err
		}
		r.EnvironmentID = created.ID
		revision, err := tx.CreateRevision(ctx, r)
		if err != nil {
			return err
		}
		created.LatestRevision = revision.Number
		out = Created{Environment: created, Revision: revision}
		return nil
	})
	if err != nil {
		return Created{}, err
	}
	return out, nil
}

// Update changes the name or notes. The paradigm of an environment never
// changes; measurements change only through a new revision.
func (s *Service) Update(ctx context.Context, actor access.Actor, id string, p Patch) (domain.Environment, error) {
	if err := actor.Require(access.ApparatusWrite); err != nil {
		return domain.Environment{}, err
	}
	if p == (Patch{}) {
		return domain.Environment{}, ErrNoChanges
	}
	var c Changes
	if p.Name != nil {
		name, err := domain.NormalizeName(*p.Name)
		if err != nil {
			return domain.Environment{}, err
		}
		c.Name = &name
	}
	if p.Notes != nil {
		notes, err := domain.NormalizeNotes(*p.Notes)
		if err != nil {
			return domain.Environment{}, err
		}
		c.Notes = &notes
	}
	return s.store.UpdateEnvironment(ctx, id, c)
}

// AddRevision stores new measurements for the environment's own paradigm.
// Existing revisions, and everything that references them, stay unchanged.
func (s *Service) AddRevision(ctx context.Context, actor access.Actor, id string, in RevisionInput) (domain.Revision, error) {
	if err := actor.Require(access.ApparatusWrite); err != nil {
		return domain.Revision{}, err
	}
	if in.ParadigmVersion < 1 {
		return domain.Revision{}, domain.ErrInvalidParadigmVersion
	}
	if _, err := domain.NormalizeNotes(in.Notes); err != nil {
		return domain.Revision{}, err
	}
	var out domain.Revision
	err := s.store.Transaction(ctx, func(tx Store) error {
		e, err := tx.LockEnvironment(ctx, id)
		if err != nil {
			return err
		}
		r, err := domain.NewRevision(e.ID, e.ParadigmKey, in.ParadigmVersion, in.Apparatus, in.Notes, actor.UserID)
		if err != nil {
			return err
		}
		if err := s.catalog.CheckApparatus(r.ParadigmKey, r.ParadigmVersion, r.Apparatus); err != nil {
			return err
		}
		out, err = tx.CreateRevision(ctx, r)
		return err
	})
	if err != nil {
		return domain.Revision{}, err
	}
	return out, nil
}

func (s *Service) Revisions(ctx context.Context, actor access.Actor, id string) ([]domain.Revision, error) {
	if err := actor.Require(access.Read); err != nil {
		return nil, err
	}
	if _, err := s.store.Environment(ctx, id); err != nil {
		return nil, err
	}
	return s.store.Revisions(ctx, id)
}

func (s *Service) Revision(ctx context.Context, actor access.Actor, id string, number int) (domain.Revision, error) {
	if err := actor.Require(access.Read); err != nil {
		return domain.Revision{}, err
	}
	if number < 1 {
		return domain.Revision{}, ErrRevisionNotFound
	}
	return s.store.Revision(ctx, id, number)
}
