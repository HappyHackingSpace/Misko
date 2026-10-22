// Package application exposes the read-only paradigm catalog. There are no
// write use cases: definitions change only through code and new versions.
package application

import (
	"context"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/paradigms/domain"
)

// Summary describes a paradigm without its full contract.
type Summary struct {
	Key               string
	Name, Description domain.Text
	Category          domain.Category
	Species           []string
	Versions          []int
	LatestVersion     int
}

type Service struct{}

func New() *Service { return &Service{} }

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
			Key: p.Key, Name: p.Name, Description: p.Description, Category: p.Category, Species: p.Species,
			Versions: versions, LatestVersion: p.Version,
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
}

func (s *Service) Latest(_ context.Context, actor access.Actor, key string) (Manifest, error) {
	if err := actor.Require(access.Read); err != nil {
		return Manifest{}, err
	}
	p, err := domain.Latest(key)
	if err != nil {
		return Manifest{}, err
	}
	return manifest(p), nil
}

func (s *Service) Version(_ context.Context, actor access.Actor, key string, version int) (Manifest, error) {
	if err := actor.Require(access.Read); err != nil {
		return Manifest{}, err
	}
	p, err := domain.Version(key, version)
	if err != nil {
		return Manifest{}, err
	}
	return manifest(p), nil
}

func manifest(p domain.Paradigm) Manifest {
	return Manifest{Paradigm: p, MetricEngineVersion: domain.MetricEngineVersion, ResultSchemaVersion: domain.ResultSchemaVersion}
}
