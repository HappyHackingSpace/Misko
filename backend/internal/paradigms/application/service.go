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
	// automated reports whether a worker can analyze a paradigm version. No
	// worker capability is registered yet, so every paradigm is unsupported.
	automated func(key string, version int) bool
}

func New() *Service {
	return &Service{automated: func(string, int) bool { return false }}
}

func (s *Service) List(_ context.Context, actor access.Actor) ([]Summary, error) {
	if err := actor.Require(access.Read); err != nil {
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
			Versions: versions, LatestVersion: p.Version, AutomatedAnalysis: s.automated(p.Key, p.Version),
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

func (s *Service) Latest(_ context.Context, actor access.Actor, key string) (Manifest, error) {
	if err := actor.Require(access.Read); err != nil {
		return Manifest{}, err
	}
	p, err := domain.Latest(key)
	if err != nil {
		return Manifest{}, err
	}
	return s.manifest(p), nil
}

func (s *Service) Version(_ context.Context, actor access.Actor, key string, version int) (Manifest, error) {
	if err := actor.Require(access.Read); err != nil {
		return Manifest{}, err
	}
	p, err := domain.Version(key, version)
	if err != nil {
		return Manifest{}, err
	}
	return s.manifest(p), nil
}

func (s *Service) manifest(p domain.Paradigm) Manifest {
	return Manifest{
		Paradigm: p, MetricEngineVersion: domain.MetricEngineVersion, ResultSchemaVersion: domain.ResultSchemaVersion,
		AutomatedAnalysis: s.automated(p.Key, p.Version),
	}
}
