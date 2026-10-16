// Package application contains catalog use cases. Changes need study:write
// and reads need *:read.
package application

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/catalog/domain"
	"strings"
	"unicode/utf8"
)

var (
	ErrSubstanceNotFound    = errors.New("substance not found")
	ErrDiseaseModelNotFound = errors.New("disease model not found")
	ErrNameTaken            = errors.New("name is already in use")
	ErrSubstanceInUse       = errors.New("substance is used by a regimen or an administration and cannot be deleted")
	ErrDiseaseModelInUse    = errors.New("disease model is used by an experiment group and cannot be deleted")
	ErrNoChanges            = errors.New("no fields to update")
	ErrInvalidQuery         = errors.New("invalid list query")
)

// SubstanceFilter is a validated list query; empty fields do not filter.
type SubstanceFilter struct {
	Search string
	Kind   domain.SubstanceKind
}

// DiseaseModelFilter is a validated list query; empty fields do not filter.
type DiseaseModelFilter struct {
	Search        string
	InductionType domain.InductionType
}

// SubstanceChanges holds validated fields; nil leaves a field unchanged and an
// empty description clears it.
type SubstanceChanges struct {
	Name        *string
	Kind        *domain.SubstanceKind
	Description *string
}

// DiseaseModelChanges holds validated fields; nil leaves a field unchanged and
// an empty criterion or description clears it.
type DiseaseModelChanges struct {
	Name                  *string
	InductionType         *domain.InductionType
	ConfirmationCriterion *string
	Description           *string
}

type Store interface {
	// Substances lists substances by name.
	Substances(ctx context.Context, filter SubstanceFilter) ([]domain.Substance, error)
	Substance(ctx context.Context, id string) (domain.Substance, error)
	// CreateSubstance returns ErrNameTaken for a name already used, ignoring case.
	CreateSubstance(ctx context.Context, s domain.Substance) (domain.Substance, error)
	UpdateSubstance(ctx context.Context, id string, c SubstanceChanges) (domain.Substance, error)
	// DeleteSubstance returns ErrSubstanceInUse while anything refers to it.
	DeleteSubstance(ctx context.Context, id string) error

	// DiseaseModels lists disease models by name.
	DiseaseModels(ctx context.Context, filter DiseaseModelFilter) ([]domain.DiseaseModel, error)
	DiseaseModel(ctx context.Context, id string) (domain.DiseaseModel, error)
	// CreateDiseaseModel returns ErrNameTaken for a name already used, ignoring case.
	CreateDiseaseModel(ctx context.Context, m domain.DiseaseModel) (domain.DiseaseModel, error)
	UpdateDiseaseModel(ctx context.Context, id string, c DiseaseModelChanges) (domain.DiseaseModel, error)
	// DeleteDiseaseModel returns ErrDiseaseModelInUse while a group refers to it.
	DeleteDiseaseModel(ctx context.Context, id string) error
}

type Service struct{ store Store }

func New(store Store) *Service { return &Service{store: store} }

type SubstanceQuery struct{ Search, Kind string }

func (s *Service) Substances(ctx context.Context, actor access.Actor, q SubstanceQuery) ([]domain.Substance, error) {
	if err := actor.Require(access.Read); err != nil {
		return nil, err
	}
	search, err := searchText(q.Search)
	if err != nil {
		return nil, err
	}
	f := SubstanceFilter{Search: search}
	if q.Kind != "" {
		if f.Kind, err = domain.ParseSubstanceKind(q.Kind); err != nil {
			return nil, ErrInvalidQuery
		}
	}
	return s.store.Substances(ctx, f)
}

func (s *Service) Substance(ctx context.Context, actor access.Actor, id string) (domain.Substance, error) {
	if err := actor.Require(access.Read); err != nil {
		return domain.Substance{}, err
	}
	return s.store.Substance(ctx, id)
}

type SubstanceInput struct{ Name, Kind, Description string }

func (s *Service) CreateSubstance(ctx context.Context, actor access.Actor, in SubstanceInput) (domain.Substance, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.Substance{}, err
	}
	substance, err := domain.NewSubstance(in.Name, in.Kind, in.Description)
	if err != nil {
		return domain.Substance{}, err
	}
	return s.store.CreateSubstance(ctx, substance)
}

// SubstancePatch fields left nil are unchanged; an empty description clears it.
type SubstancePatch struct{ Name, Kind, Description *string }

func (s *Service) UpdateSubstance(ctx context.Context, actor access.Actor, id string, p SubstancePatch) (domain.Substance, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.Substance{}, err
	}
	if p == (SubstancePatch{}) {
		return domain.Substance{}, ErrNoChanges
	}
	var c SubstanceChanges
	var err error
	if c.Name, err = optional(p.Name, domain.NormalizeName); err != nil {
		return domain.Substance{}, err
	}
	if c.Kind, err = optional(p.Kind, domain.ParseSubstanceKind); err != nil {
		return domain.Substance{}, err
	}
	if c.Description, err = optional(p.Description, domain.NormalizeDescription); err != nil {
		return domain.Substance{}, err
	}
	return s.store.UpdateSubstance(ctx, id, c)
}

func (s *Service) DeleteSubstance(ctx context.Context, actor access.Actor, id string) error {
	if err := actor.Require(access.StudyWrite); err != nil {
		return err
	}
	return s.store.DeleteSubstance(ctx, id)
}

type DiseaseModelQuery struct{ Search, InductionType string }

func (s *Service) DiseaseModels(ctx context.Context, actor access.Actor, q DiseaseModelQuery) ([]domain.DiseaseModel, error) {
	if err := actor.Require(access.Read); err != nil {
		return nil, err
	}
	search, err := searchText(q.Search)
	if err != nil {
		return nil, err
	}
	f := DiseaseModelFilter{Search: search}
	if q.InductionType != "" {
		if f.InductionType, err = domain.ParseInductionType(q.InductionType); err != nil {
			return nil, ErrInvalidQuery
		}
	}
	return s.store.DiseaseModels(ctx, f)
}

func (s *Service) DiseaseModel(ctx context.Context, actor access.Actor, id string) (domain.DiseaseModel, error) {
	if err := actor.Require(access.Read); err != nil {
		return domain.DiseaseModel{}, err
	}
	return s.store.DiseaseModel(ctx, id)
}

type DiseaseModelInput struct{ Name, InductionType, ConfirmationCriterion, Description string }

func (s *Service) CreateDiseaseModel(ctx context.Context, actor access.Actor, in DiseaseModelInput) (domain.DiseaseModel, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.DiseaseModel{}, err
	}
	model, err := domain.NewDiseaseModel(in.Name, in.InductionType, in.ConfirmationCriterion, in.Description)
	if err != nil {
		return domain.DiseaseModel{}, err
	}
	return s.store.CreateDiseaseModel(ctx, model)
}

// DiseaseModelPatch fields left nil are unchanged; an empty criterion or
// description clears it.
type DiseaseModelPatch struct{ Name, InductionType, ConfirmationCriterion, Description *string }

func (s *Service) UpdateDiseaseModel(ctx context.Context, actor access.Actor, id string, p DiseaseModelPatch) (domain.DiseaseModel, error) {
	if err := actor.Require(access.StudyWrite); err != nil {
		return domain.DiseaseModel{}, err
	}
	if p == (DiseaseModelPatch{}) {
		return domain.DiseaseModel{}, ErrNoChanges
	}
	var c DiseaseModelChanges
	var err error
	if c.Name, err = optional(p.Name, domain.NormalizeName); err != nil {
		return domain.DiseaseModel{}, err
	}
	if c.InductionType, err = optional(p.InductionType, domain.ParseInductionType); err != nil {
		return domain.DiseaseModel{}, err
	}
	if c.ConfirmationCriterion, err = optional(p.ConfirmationCriterion, domain.NormalizeConfirmationCriterion); err != nil {
		return domain.DiseaseModel{}, err
	}
	if c.Description, err = optional(p.Description, domain.NormalizeDescription); err != nil {
		return domain.DiseaseModel{}, err
	}
	return s.store.UpdateDiseaseModel(ctx, id, c)
}

func (s *Service) DeleteDiseaseModel(ctx context.Context, actor access.Actor, id string) error {
	if err := actor.Require(access.StudyWrite); err != nil {
		return err
	}
	return s.store.DeleteDiseaseModel(ctx, id)
}

// searchText trims a name search and caps it at 100 characters.
func searchText(raw string) (string, error) {
	search := strings.TrimSpace(raw)
	if !utf8.ValidString(search) || utf8.RuneCountInString(search) > 100 {
		return "", ErrInvalidQuery
	}
	return search, nil
}

func optional[T any](raw *string, parse func(string) (T, error)) (*T, error) {
	if raw == nil {
		return nil, nil
	}
	value, err := parse(*raw)
	if err != nil {
		return nil, err
	}
	return &value, nil
}
