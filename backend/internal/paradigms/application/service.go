// Package application exposes the read-only paradigm catalog. There are no
// write use cases: definitions change only through code and new versions.
package application

import (
	"context"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/paradigms/domain"
)

// Summary describes a paradigm without its full contract. AutomatedAnalysis
// reports real worker support, which is separate from catalog presence.
type Summary struct {
	Key               string
	Name              string
	Category          domain.Category
	Species           []string
	Versions          []int
	LatestVersion     int
	AutomatedAnalysis bool
}

type Service struct {
	// capabilities tells which paradigm versions a worker can analyze. A nil
	// source means no worker exists, so every paradigm is unsupported.
	capabilities Capabilities
}

// Capability is a paradigm version an active worker can analyze.
type Capability struct {
	Key     string
	Version int
}

// Capabilities lists the paradigm versions with at least one active worker.
type Capabilities interface {
	Active(ctx context.Context) ([]Capability, error)
}

func New(capabilities Capabilities) *Service {
	return &Service{capabilities: capabilities}
}

func (s *Service) automated(ctx context.Context) (map[Capability]bool, error) {
	out := map[Capability]bool{}
	if s.capabilities == nil {
		return out, nil
	}
	active, err := s.capabilities.Active(ctx)
	if err != nil {
		return nil, err
	}
	for _, c := range active {
		out[c] = true
	}
	return out, nil
}

func (s *Service) List(ctx context.Context, actor access.Actor) ([]Summary, error) {
	if err := actor.Require(access.Read); err != nil {
		return nil, err
	}
	supported, err := s.automated(ctx)
	if err != nil {
		return nil, err
	}
	var out []Summary
	for _, p := range domain.All() {
		versions, err := domain.Versions(p.Key)
		if err != nil {
			return nil, err
		}
		out = append(out, Summary{
			Key: p.Key, Name: p.Name, Category: p.Category, Species: p.Species,
			Versions: versions, LatestVersion: p.Version, AutomatedAnalysis: supported[Capability{p.Key, p.Version}],
		})
	}
	return out, nil
}

// Manifest is a paradigm version with the engine and result schema versions a
// worker must target.
type Manifest struct {
	Paradigm            domain.Paradigm
	MetricEngineVersion int
	ResultSchemaVersion int
	AutomatedAnalysis   bool
}

func (s *Service) Latest(ctx context.Context, actor access.Actor, key string) (Manifest, error) {
	if err := actor.Require(access.Read); err != nil {
		return Manifest{}, err
	}
	p, err := domain.Latest(key)
	if err != nil {
		return Manifest{}, err
	}
	return s.manifest(ctx, p)
}

func (s *Service) Version(ctx context.Context, actor access.Actor, key string, version int) (Manifest, error) {
	if err := actor.Require(access.Read); err != nil {
		return Manifest{}, err
	}
	p, err := domain.Version(key, version)
	if err != nil {
		return Manifest{}, err
	}
	return s.manifest(ctx, p)
}

func (s *Service) manifest(ctx context.Context, p domain.Paradigm) (Manifest, error) {
	supported, err := s.automated(ctx)
	if err != nil {
		return Manifest{}, err
	}
	return Manifest{
		Paradigm: p, MetricEngineVersion: domain.MetricEngineVersion, ResultSchemaVersion: domain.ResultSchemaVersion,
		AutomatedAnalysis: supported[Capability{p.Key, p.Version}],
	}, nil
}
