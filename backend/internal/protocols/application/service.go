// Package application contains protocol use cases. Changes need
// apparatus:write and reads need *:read.
package application

import (
	"context"
	"errors"
	"fmt"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/protocols/domain"
	"slices"
)

var (
	ErrExperimentNotFound          = errors.New("experiment not found")
	ErrProtocolNotFound            = errors.New("protocol not found in this experiment")
	ErrVersionNotFound             = errors.New("protocol version not found")
	ErrEnvironmentRevisionNotFound = errors.New("environment revision not found")
	ErrIncompatibleEnvironment     = errors.New("environment revision is set up for another paradigm or paradigm version")
	ErrTrialTypeNotSupported       = errors.New("trial type is not defined by the paradigm version")
	ErrUnknownParadigm             = errors.New("unknown paradigm")
	ErrUnknownParadigmVersion      = errors.New("unknown paradigm version")
	ErrInvalidSession              = errors.New("session parameters must be defined by the paradigm version, within range and consistent with the environment")
	ErrNameTaken                   = errors.New("name is already in use in this experiment")
	ErrNoChanges                   = errors.New("no fields to update")
)

// RevisionRef is the part of an environment revision a step needs.
type RevisionRef struct {
	ID              string
	EnvironmentID   string
	Number          int
	ParadigmKey     string
	ParadigmVersion int
	Apparatus       map[string]float64
}

// Catalog reads the hardcoded paradigm definitions.
type Catalog interface {
	// TrialTypes returns ErrUnknownParadigm or ErrUnknownParadigmVersion for unknown definitions.
	TrialTypes(key string, version int) ([]string, error)
	// ResolveSession validates session values against the paradigm version and
	// the environment's apparatus, and returns them with defaults filled in. It
	// returns ErrInvalidSession for invalid values.
	ResolveSession(key string, version int, apparatus, session map[string]float64) (map[string]float64, error)
}

// Changes holds validated fields; nil leaves a field unchanged and an empty
// description clears it.
type Changes struct {
	Name, Description *string
}

// Store persists protocols. Scoped lookups return the matching not-found error
// for records of another experiment.
type Store interface {
	Transaction(ctx context.Context, fn func(Store) error) error
	Protocols(ctx context.Context, experimentID string) ([]domain.Protocol, error)
	Protocol(ctx context.Context, experimentID, protocolID string) (domain.Protocol, error)
	CreateProtocol(ctx context.Context, p domain.Protocol) (domain.Protocol, error)
	UpdateProtocol(ctx context.Context, experimentID, protocolID string, c Changes) (domain.Protocol, error)
	// LockProtocol locks the protocol until the transaction ends, so versions
	// are numbered one at a time.
	LockProtocol(ctx context.Context, experimentID, protocolID string) (domain.Protocol, error)
	EnvironmentRevision(ctx context.Context, id string) (RevisionRef, error)
	// CreateVersion stores v and its steps with the next number of its protocol.
	CreateVersion(ctx context.Context, v domain.Version) (domain.Version, error)
	Versions(ctx context.Context, experimentID, protocolID string) ([]domain.Version, error)
	Version(ctx context.Context, experimentID, protocolID string, number int) (domain.Version, error)
}

type Service struct {
	store   Store
	catalog Catalog
}

func New(store Store, catalog Catalog) *Service { return &Service{store: store, catalog: catalog} }

type StepInput struct {
	Position              int
	ParadigmKey           string
	ParadigmVersion       int
	EnvironmentRevisionID string
	TrialType             string
	Trials                int
	InterTrialIntervalS   int
	Session               map[string]float64
	Notes                 string
}

type VersionInput struct {
	Notes string
	Steps []StepInput
}

type ProtocolInput struct {
	Name, Description string
	Version           VersionInput
}

type Patch struct {
	Name, Description *string
}

type Created struct {
	Protocol domain.Protocol
	Version  domain.Version
}

func (s *Service) List(ctx context.Context, actor access.Actor, experimentID string) ([]domain.Protocol, error) {
	if err := actor.Require(access.Read); err != nil {
		return nil, err
	}
	return s.store.Protocols(ctx, experimentID)
}

func (s *Service) Get(ctx context.Context, actor access.Actor, experimentID, protocolID string) (domain.Protocol, error) {
	if err := actor.Require(access.Read); err != nil {
		return domain.Protocol{}, err
	}
	return s.store.Protocol(ctx, experimentID, protocolID)
}

// Create stores a protocol in an experiment together with its first version.
func (s *Service) Create(ctx context.Context, actor access.Actor, experimentID string, in ProtocolInput) (Created, error) {
	if err := actor.Require(access.ApparatusWrite); err != nil {
		return Created{}, err
	}
	p, err := domain.NewProtocol(experimentID, in.Name, in.Description)
	if err != nil {
		return Created{}, err
	}
	v, err := newVersion(in.Version, actor.UserID)
	if err != nil {
		return Created{}, err
	}
	var out Created
	err = s.store.Transaction(ctx, func(tx Store) error {
		steps, err := s.resolve(ctx, tx, v.Steps)
		if err != nil {
			return err
		}
		created, err := tx.CreateProtocol(ctx, p)
		if err != nil {
			return err
		}
		v.ProtocolID, v.ExperimentID, v.Steps = created.ID, created.ExperimentID, steps
		version, err := tx.CreateVersion(ctx, v)
		if err != nil {
			return err
		}
		created.LatestVersion = version.Number
		out = Created{Protocol: created, Version: version}
		return nil
	})
	if err != nil {
		return Created{}, err
	}
	return out, nil
}

// Update changes the name or description. Steps change only through a new version.
func (s *Service) Update(ctx context.Context, actor access.Actor, experimentID, protocolID string, p Patch) (domain.Protocol, error) {
	if err := actor.Require(access.ApparatusWrite); err != nil {
		return domain.Protocol{}, err
	}
	if p == (Patch{}) {
		return domain.Protocol{}, ErrNoChanges
	}
	var c Changes
	if p.Name != nil {
		name, err := domain.NormalizeName(*p.Name)
		if err != nil {
			return domain.Protocol{}, err
		}
		c.Name = &name
	}
	if p.Description != nil {
		description, err := domain.NormalizeDescription(*p.Description)
		if err != nil {
			return domain.Protocol{}, err
		}
		c.Description = &description
	}
	return s.store.UpdateProtocol(ctx, experimentID, protocolID, c)
}

// AddVersion stores the next version. Earlier versions stay unchanged.
func (s *Service) AddVersion(ctx context.Context, actor access.Actor, experimentID, protocolID string, in VersionInput) (domain.Version, error) {
	if err := actor.Require(access.ApparatusWrite); err != nil {
		return domain.Version{}, err
	}
	v, err := newVersion(in, actor.UserID)
	if err != nil {
		return domain.Version{}, err
	}
	var out domain.Version
	err = s.store.Transaction(ctx, func(tx Store) error {
		p, err := tx.LockProtocol(ctx, experimentID, protocolID)
		if err != nil {
			return err
		}
		steps, err := s.resolve(ctx, tx, v.Steps)
		if err != nil {
			return err
		}
		v.ProtocolID, v.ExperimentID, v.Steps = p.ID, p.ExperimentID, steps
		out, err = tx.CreateVersion(ctx, v)
		return err
	})
	if err != nil {
		return domain.Version{}, err
	}
	return out, nil
}

func (s *Service) Versions(ctx context.Context, actor access.Actor, experimentID, protocolID string) ([]domain.Version, error) {
	if err := actor.Require(access.Read); err != nil {
		return nil, err
	}
	if _, err := s.store.Protocol(ctx, experimentID, protocolID); err != nil {
		return nil, err
	}
	return s.store.Versions(ctx, experimentID, protocolID)
}

func (s *Service) Version(ctx context.Context, actor access.Actor, experimentID, protocolID string, number int) (domain.Version, error) {
	if err := actor.Require(access.Read); err != nil {
		return domain.Version{}, err
	}
	if number < 1 {
		return domain.Version{}, ErrVersionNotFound
	}
	return s.store.Version(ctx, experimentID, protocolID, number)
}

func newVersion(in VersionInput, createdBy string) (domain.Version, error) {
	steps := make([]domain.Step, 0, len(in.Steps))
	for _, s := range in.Steps {
		steps = append(steps, domain.Step{
			Position: s.Position, ParadigmKey: s.ParadigmKey, ParadigmVersion: s.ParadigmVersion, EnvironmentRevisionID: s.EnvironmentRevisionID,
			TrialType: s.TrialType, Trials: s.Trials, InterTrialIntervalS: s.InterTrialIntervalS, Session: s.Session, Notes: s.Notes,
		})
	}
	return domain.NewVersion("", in.Notes, createdBy, steps)
}

// resolve checks every step against the catalog and its environment revision
// and fills in session defaults and the revision it references.
func (s *Service) resolve(ctx context.Context, tx Store, steps []domain.Step) ([]domain.Step, error) {
	out := slices.Clone(steps)
	for i, step := range out {
		types, err := s.catalog.TrialTypes(step.ParadigmKey, step.ParadigmVersion)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(types, step.TrialType) {
			return nil, fmt.Errorf("%w: %s", ErrTrialTypeNotSupported, step.TrialType)
		}
		revision, err := tx.EnvironmentRevision(ctx, step.EnvironmentRevisionID)
		if err != nil {
			return nil, err
		}
		if revision.ParadigmKey != step.ParadigmKey || revision.ParadigmVersion != step.ParadigmVersion {
			return nil, ErrIncompatibleEnvironment
		}
		session, err := s.catalog.ResolveSession(step.ParadigmKey, step.ParadigmVersion, revision.Apparatus, step.Session)
		if err != nil {
			return nil, err
		}
		out[i].Session, out[i].EnvironmentID, out[i].EnvironmentRevision = session, revision.EnvironmentID, revision.Number
	}
	return out, nil
}
