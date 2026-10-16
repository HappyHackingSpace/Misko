package application

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/subjects/domain"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

func newService(store Store) *Service { return New(store, func() time.Time { return now }) }

func TestSubjectPermissions(t *testing.T) {
	code := "M2"
	operations := map[string]struct {
		permission access.Permission
		run        func(*Service, access.Actor) error
	}{
		"list": {access.Read, func(s *Service, a access.Actor) error { _, err := s.List(context.Background(), a, Query{}); return err }},
		"get":  {access.Read, func(s *Service, a access.Actor) error { _, err := s.Subject(context.Background(), a, "id"); return err }},
		"create": {access.SubjectWrite, func(s *Service, a access.Actor) error {
			_, err := s.Create(context.Background(), a, Input{Code: "M1", Species: "MOUSE", Sex: "MALE"})
			return err
		}},
		"update": {access.SubjectWrite, func(s *Service, a access.Actor) error {
			_, err := s.Update(context.Background(), a, "id", Patch{Code: &code})
			return err
		}},
		"delete": {access.SubjectWrite, func(s *Service, a access.Actor) error { return s.Delete(context.Background(), a, "id") }},
		"weights": {access.Read, func(s *Service, a access.Actor) error {
			_, err := s.Weights(context.Background(), a, "id")
			return err
		}},
		"record weight": {access.WeightWrite, func(s *Service, a access.Actor) error {
			_, err := s.RecordWeight(context.Background(), a, "id", WeightInput{Grams: "25.5", MeasuredAt: now})
			return err
		}},
	}
	for _, role := range append(access.Roles(), "ROOT") {
		for name, op := range operations {
			store := &fakeStore{}
			err := op.run(newService(store), access.Actor{UserID: "u", Role: role})
			allowed := role.Allows(access.Read) && role.Allows(op.permission)
			if allowed != (err == nil) || (!allowed && !errors.Is(err, access.ErrForbidden)) {
				t.Errorf("%s %s: err=%v allowed=%v", role, name, err, allowed)
			}
			if !allowed && store.calls != 0 {
				t.Errorf("%s %s: denied operation reached the store", role, name)
			}
		}
	}
	for name, op := range operations {
		if err := op.run(newService(&fakeStore{}), access.Actor{}); !errors.Is(err, access.ErrUnauthenticated) {
			t.Errorf("anonymous %s: %v", name, err)
		}
	}
}

func TestUpdateValidatesProvidedFieldsOnly(t *testing.T) {
	admin := access.Actor{UserID: "u", Role: access.Researcher}
	store := &fakeStore{}
	s := newService(store)
	bad, code, sex := "cat", " M-9 ", "FEMALE"
	if _, err := s.Update(context.Background(), admin, "id", Patch{}); !errors.Is(err, ErrNoChanges) {
		t.Fatalf("empty patch: %v", err)
	}
	if _, err := s.Update(context.Background(), admin, "id", Patch{Species: &bad}); !errors.Is(err, domain.ErrInvalidSpecies) || store.calls != 0 {
		t.Fatalf("invalid species: %v calls=%d", err, store.calls)
	}
	if _, err := s.Update(context.Background(), admin, "id", Patch{Code: &code, Sex: &sex}); err != nil {
		t.Fatal(err)
	}
	c := store.changes
	if *c.Code != "M-9" || *c.Sex != domain.Female || c.Species != nil {
		t.Fatalf("changes=%+v", c)
	}
}

func TestListAllowlistsQuery(t *testing.T) {
	viewer := access.Actor{UserID: "u", Role: access.Viewer}
	store := &fakeStore{}
	s := newService(store)
	for _, q := range []Query{{Sort: "notes"}, {Sort: "birthDate"}, {Order: "up"}, {Species: "CAT"}, {Sex: "f"}, {Unallocated: "yes"}, {Unallocated: "TRUE"}, {Page: -1}, {PageSize: -1}, {Search: strings.Repeat("a", 101)}} {
		if _, err := s.List(context.Background(), viewer, q); !errors.Is(err, ErrInvalidQuery) {
			t.Errorf("%+v: %v", q, err)
		}
	}
	if _, err := s.List(context.Background(), viewer, Query{}); err != nil || store.filter != (Filter{Sort: "createdAt", Descending: true, Limit: 10}) {
		t.Fatalf("defaults: %+v %v", store.filter, err)
	}
	if _, err := s.List(context.Background(), viewer, Query{Unallocated: "false"}); err != nil || store.filter.Unallocated {
		t.Fatalf("unallocated=false filters: %+v %v", store.filter, err)
	}
	page, err := s.List(context.Background(), viewer, Query{Search: " c57 ", Species: "RAT", Sex: "FEMALE", Unallocated: "true", Sort: "code", Page: 2, PageSize: 500})
	want := Filter{Search: "c57", Species: domain.Rat, Sex: domain.Female, Unallocated: true, Sort: "code", Limit: 100, Offset: 100}
	if err != nil || store.filter != want || page.Page != 2 || page.PageSize != 100 {
		t.Fatalf("filter=%+v page=%+v err=%v", store.filter, page, err)
	}
}

func TestRecordWeightValidatesBeforeStoring(t *testing.T) {
	tech := access.Actor{UserID: "tech", Role: access.Technician}
	for _, tc := range []struct {
		in   WeightInput
		want error
	}{
		{WeightInput{Grams: "0", MeasuredAt: now}, domain.ErrInvalidWeight},
		{WeightInput{Grams: "abc", MeasuredAt: now}, domain.ErrInvalidWeight},
		{WeightInput{Grams: "25"}, domain.ErrMissingTime},
		{WeightInput{Grams: "25", MeasuredAt: now.Add(time.Hour)}, domain.ErrFutureTime},
	} {
		store := &fakeStore{}
		if _, err := newService(store).RecordWeight(context.Background(), tech, "subject", tc.in); !errors.Is(err, tc.want) || store.calls != 0 {
			t.Errorf("%+v: err=%v calls=%d want %v", tc.in, err, store.calls, tc.want)
		}
	}
	store := &fakeStore{}
	measured := time.Date(2026, 10, 15, 14, 30, 0, 123456789, time.FixedZone("TRT", 3*3600))
	if _, err := newService(store).RecordWeight(context.Background(), tech, "subject", WeightInput{Grams: " 25.125 ", MeasuredAt: measured}); err != nil {
		t.Fatal(err)
	}
	w := store.weight
	if w.SubjectID != "subject" || w.Milligrams != 25125 || w.RecordedBy != "tech" || !w.MeasuredAt.Equal(measured.Truncate(time.Microsecond)) || w.MeasuredAt.Location() != time.UTC {
		t.Fatalf("stored %+v", w)
	}
}

func TestWeightsOfUnknownSubjectIsNotFound(t *testing.T) {
	store := &fakeStore{subjectErr: ErrNotFound}
	if _, err := newService(store).Weights(context.Background(), access.Actor{UserID: "u", Role: access.Viewer}, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v", err)
	}
}

type fakeStore struct {
	calls      int
	changes    Changes
	filter     Filter
	weight     domain.Weight
	subjectErr error
}

func (f *fakeStore) Create(_ context.Context, s domain.Subject) (domain.Subject, error) {
	f.calls++
	return s, nil
}

func (f *fakeStore) Subject(context.Context, string) (domain.Subject, error) {
	f.calls++
	return domain.Subject{}, f.subjectErr
}

func (f *fakeStore) CreateWeight(_ context.Context, w domain.Weight) (domain.Weight, error) {
	f.calls++
	f.weight = w
	return w, nil
}

func (f *fakeStore) Weights(context.Context, string) ([]domain.Weight, error) {
	f.calls++
	return nil, nil
}

func (f *fakeStore) List(_ context.Context, filter Filter) ([]domain.Subject, int, error) {
	f.calls++
	f.filter = filter
	return nil, 0, nil
}

func (f *fakeStore) Update(_ context.Context, _ string, c Changes) (domain.Subject, error) {
	f.calls++
	f.changes = c
	return domain.Subject{}, nil
}

func (f *fakeStore) Delete(context.Context, string) error {
	f.calls++
	return nil
}
