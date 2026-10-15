// Package application contains protocol use cases. Changes need study:write
// and reads need *:read.
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
	ErrExperimentNotFound     = errors.New("experiment not found")
	ErrProtocolNotFound       = errors.New("protocol not found in this experiment")
	ErrVersionNotFound        = errors.New("protocol version not found")
	ErrExperimentClosed       = errors.New("protocols cannot be added to a completed experiment")
	ErrTrialTypeNotSupported  = errors.New("trial type is not defined by the paradigm version")
	ErrUnknownParadigm        = errors.New("unknown paradigm")
	ErrUnknownParadigmVersion = errors.New("unknown paradigm version")
	ErrInvalidSession         = errors.New("session parameters must be defined by the paradigm version and within range")
	ErrNameTaken              = errors.New("name is already in use in this experiment")
	ErrNoChanges              = errors.New("no fields to update")
)

// Catalog reads the hardcoded paradigm definitions.
type Catalog interface {
	// TrialTypes returns ErrUnknownParadigm or ErrUnknownParadigmVersion for unknown definitions.
	TrialTypes(key string, version int) ([]string, error)
	// ResolveSession validates session values against the paradigm version
	// and returns them with defaults filled in. It returns ErrInvalidSession
	// for invalid values.
	ResolveSession(key string, version int, session map[string]float64) (map[string]float64, error)
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
	Position            int
	ParadigmKey         string
	ParadigmVersion     int
	TrialType           string
	Trials              int
	InterTrialIntervalS int
	Session             map[string]float64
	Notes               string
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
	if err := actor.Require(access.StudyWrite); err != nil {
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
	if v.Steps, err = s.resolve(v.Steps); err != nil {
		return Created{}, err
	}
	var out Created
	err = s.store.Transaction(ctx, func(tx Store) error {
		created, err := tx.CreateProtocol(ctx, p)
		if err != nil {
			return err
		}
		v.ProtocolID, v.ExperimentID = created.ID, created.ExperimentID
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
	if err := actor.Require(access.StudyWrite); err != nil {
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
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.Version{}, err
	}
	v, err := newVersion(in, actor.UserID)
	if err != nil {
		return domain.Version{}, err
	}
	if v.Steps, err = s.resolve(v.Steps); err != nil {
		return domain.Version{}, err
	}
	var out domain.Version
	err = s.store.Transaction(ctx, func(tx Store) error {
		p, err := tx.LockProtocol(ctx, experimentID, protocolID)
		if err != nil {
			return err
		}
		v.ProtocolID, v.ExperimentID = p.ID, p.ExperimentID
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
		steps = append(steps, domain.Step(s))
	}
	return domain.NewVersion("", in.Notes, createdBy, steps)
}

// resolve checks every step against the catalog and fills in session
// defaults.
func (s *Service) resolve(steps []domain.Step) ([]domain.Step, error) {
	out := slices.Clone(steps)
	for i, step := range out {
		types, err := s.catalog.TrialTypes(step.ParadigmKey, step.ParadigmVersion)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(types, step.TrialType) {
			return nil, fmt.Errorf("%w: %s", ErrTrialTypeNotSupported, step.TrialType)
		}
		if out[i].Session, err = s.catalog.ResolveSession(step.ParadigmKey, step.ParadigmVersion, step.Session); err != nil {
			return nil, err
		}
	}
	return out, nil
}
