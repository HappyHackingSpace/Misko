// Package application contains subject and body weight use cases. Subject
// changes need subject:write, weights need weight:write and reads need *:read.
package application

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/subjects/domain"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrNotFound     = errors.New("subject not found")
	ErrCodeTaken    = errors.New("subject code is already in use")
	ErrInUse        = errors.New("subject is allocated or has recorded measurements and cannot be deleted")
	ErrNoChanges    = errors.New("no fields to update")
	ErrInvalidQuery = errors.New("invalid list query")
)

// Changes holds validated fields; nil leaves a field unchanged.
type Changes struct {
	Code    *string
	Species *domain.Species
	Sex     *domain.Sex
}

// Filter is a validated list query whose sort key is allowlisted.
type Filter struct {
	Search  string
	Species domain.Species
	Sex     domain.Sex
	// Unallocated keeps only animals not allocated to any group.
	Unallocated   bool
	Sort          string
	Descending    bool
	Limit, Offset int
}

type Store interface {
	// Create returns ErrCodeTaken for a code already used, ignoring case.
	Create(ctx context.Context, subject domain.Subject) (domain.Subject, error)
	Subject(ctx context.Context, id string) (domain.Subject, error)
	List(ctx context.Context, filter Filter) ([]domain.Subject, int, error)
	Update(ctx context.Context, id string, changes Changes) (domain.Subject, error)
	// Delete returns ErrInUse while the subject is allocated or has weights.
	Delete(ctx context.Context, id string) error
	// CreateWeight returns ErrNotFound for an unknown subject.
	CreateWeight(ctx context.Context, w domain.Weight) (domain.Weight, error)
	// Weights lists a subject's weights, newest first.
	Weights(ctx context.Context, subjectID string) ([]domain.Weight, error)
}

type Service struct {
	store Store
	now   func() time.Time
}

func New(store Store, now func() time.Time) *Service { return &Service{store: store, now: now} }

func (s *Service) Subject(ctx context.Context, actor access.Actor, id string) (domain.Subject, error) {
	if err := actor.Require(access.Read); err != nil {
		return domain.Subject{}, err
	}
	return s.store.Subject(ctx, id)
}

type Input struct {
	Code, Species, Sex string
}

func (s *Service) Create(ctx context.Context, actor access.Actor, in Input) (domain.Subject, error) {
	if err := actor.Require(access.SubjectWrite); err != nil {
		return domain.Subject{}, err
	}
	subject, err := domain.NewSubject(in.Code, in.Species, in.Sex)
	if err != nil {
		return domain.Subject{}, err
	}
	return s.store.Create(ctx, subject)
}

// Patch fields left nil are unchanged.
type Patch struct {
	Code, Species, Sex *string
}

func (s *Service) Update(ctx context.Context, actor access.Actor, id string, p Patch) (domain.Subject, error) {
	if err := actor.Require(access.SubjectWrite); err != nil {
		return domain.Subject{}, err
	}
	if p == (Patch{}) {
		return domain.Subject{}, ErrNoChanges
	}
	var c Changes
	var err error
	if p.Code != nil {
		if c.Code, err = validated(domain.NormalizeCode(*p.Code)); err != nil {
			return domain.Subject{}, err
		}
	}
	if p.Species != nil {
		if c.Species, err = validated(domain.ParseSpecies(*p.Species)); err != nil {
			return domain.Subject{}, err
		}
	}
	if p.Sex != nil {
		if c.Sex, err = validated(domain.ParseSex(*p.Sex)); err != nil {
			return domain.Subject{}, err
		}
	}
	return s.store.Update(ctx, id, c)
}

func (s *Service) Delete(ctx context.Context, actor access.Actor, id string) error {
	if err := actor.Require(access.SubjectWrite); err != nil {
		return err
	}
	return s.store.Delete(ctx, id)
}

// Query is a raw list query. Unallocated is "true" to keep only animals
// without an allocation; "" and "false" do not filter.
type Query struct {
	Search, Species, Sex, Unallocated, Sort, Order string
	Page, PageSize                                 int
}

type Page struct {
	Subjects              []domain.Subject
	Total, Page, PageSize int
}

var sorts = []string{"code", "species", "sex", "createdAt"}

func (s *Service) List(ctx context.Context, actor access.Actor, q Query) (Page, error) {
	if err := actor.Require(access.Read); err != nil {
		return Page{}, err
	}
	f := Filter{Search: strings.TrimSpace(q.Search), Sort: q.Sort}
	if !utf8.ValidString(f.Search) || utf8.RuneCountInString(f.Search) > 100 {
		return Page{}, ErrInvalidQuery
	}
	var err error
	if q.Species != "" {
		if f.Species, err = domain.ParseSpecies(q.Species); err != nil {
			return Page{}, ErrInvalidQuery
		}
	}
	if q.Sex != "" {
		if f.Sex, err = domain.ParseSex(q.Sex); err != nil {
			return Page{}, ErrInvalidQuery
		}
	}
	switch q.Unallocated {
	case "", "false":
	case "true":
		f.Unallocated = true
	default:
		return Page{}, ErrInvalidQuery
	}
	if f.Sort == "" {
		f.Sort = "createdAt"
	}
	if !slices.Contains(sorts, f.Sort) {
		return Page{}, ErrInvalidQuery
	}
	page := 0
	if f.Descending, err = descending(q.Order, f.Sort == "createdAt"); err != nil {
		return Page{}, err
	}
	if page, f.Limit, f.Offset, err = paging(q.Page, q.PageSize); err != nil {
		return Page{}, err
	}
	subjects, total, err := s.store.List(ctx, f)
	if err != nil {
		return Page{}, err
	}
	return Page{Subjects: subjects, Total: total, Page: page, PageSize: f.Limit}, nil
}

type WeightInput struct {
	Grams      string
	MeasuredAt time.Time
}

func (s *Service) RecordWeight(ctx context.Context, actor access.Actor, subjectID string, in WeightInput) (domain.Weight, error) {
	if err := actor.Require(access.WeightWrite); err != nil {
		return domain.Weight{}, err
	}
	measuredAt := in.MeasuredAt
	if !measuredAt.IsZero() {
		measuredAt = measuredAt.UTC().Truncate(time.Microsecond)
	}
	w, err := domain.NewWeight(subjectID, in.Grams, measuredAt, s.now(), actor.UserID)
	if err != nil {
		return domain.Weight{}, err
	}
	return s.store.CreateWeight(ctx, w)
}

// Weights returns ErrNotFound for an unknown subject rather than an empty list.
func (s *Service) Weights(ctx context.Context, actor access.Actor, subjectID string) ([]domain.Weight, error) {
	if err := actor.Require(access.Read); err != nil {
		return nil, err
	}
	if _, err := s.store.Subject(ctx, subjectID); err != nil {
		return nil, err
	}
	return s.store.Weights(ctx, subjectID)
}

func validated[T any](value T, err error) (*T, error) {
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func descending(order string, fallback bool) (bool, error) {
	switch order {
	case "":
		return fallback, nil
	case "asc":
		return false, nil
	case "desc":
		return true, nil
	}
	return false, ErrInvalidQuery
}

// paging defaults to 10 rows per page and caps the page size at 100.
func paging(page, size int) (current, limit, offset int, err error) {
	if page < 0 || page > 1<<20 || size < 0 {
		return 0, 0, 0, ErrInvalidQuery
	}
	current, limit = max(page, 1), min(size, 100)
	if limit == 0 {
		limit = 10
	}
	return current, limit, (current - 1) * limit, nil
}
