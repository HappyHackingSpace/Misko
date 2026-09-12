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

var clock = func() time.Time { return time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC) }

func TestSubjectPermissions(t *testing.T) {
	code := "M2"
	operations := map[string]struct {
		write bool
		run   func(*Service, access.Actor) error
	}{
		"list": {false, func(s *Service, a access.Actor) error { _, err := s.List(context.Background(), a, Query{}); return err }},
		"get":  {false, func(s *Service, a access.Actor) error { _, err := s.Subject(context.Background(), a, "id"); return err }},
		"create": {true, func(s *Service, a access.Actor) error {
			_, err := s.Create(context.Background(), a, Input{Code: "M1", Species: "MOUSE", Sex: "MALE"})
			return err
		}},
		"update": {true, func(s *Service, a access.Actor) error {
			_, err := s.Update(context.Background(), a, "id", Patch{Code: &code})
			return err
		}},
		"delete": {true, func(s *Service, a access.Actor) error { return s.Delete(context.Background(), a, "id") }},
	}
	for _, role := range append(access.Roles(), "ROOT") {
		for name, op := range operations {
			store := &fakeStore{}
			err := op.run(New(store, clock), access.Actor{UserID: "u", Role: role})
			allowed := role.Allows(access.Read) && (!op.write || role.Allows(access.SubjectWrite))
			if allowed != (err == nil) || (!allowed && !errors.Is(err, access.ErrForbidden)) {
				t.Errorf("%s %s: err=%v allowed=%v", role, name, err, allowed)
			}
			if !allowed && store.calls != 0 {
				t.Errorf("%s %s: denied operation reached the store", role, name)
			}
		}
	}
	for name, op := range operations {
		if err := op.run(New(&fakeStore{}, clock), access.Actor{}); !errors.Is(err, access.ErrUnauthenticated) {
			t.Errorf("anonymous %s: %v", name, err)
		}
	}
}

func TestUpdateValidatesProvidedFieldsOnly(t *testing.T) {
	admin := access.Actor{UserID: "u", Role: access.Researcher}
	store := &fakeStore{}
	s := New(store, clock)
	bad, empty, code, date := "cat", "", " M-9 ", "2026-01-02"
	if _, err := s.Update(context.Background(), admin, "id", Patch{}); !errors.Is(err, ErrNoChanges) {
		t.Fatalf("empty patch: %v", err)
	}
	if _, err := s.Update(context.Background(), admin, "id", Patch{Species: &bad}); !errors.Is(err, domain.ErrInvalidSpecies) || store.calls != 0 {
		t.Fatalf("invalid species: %v calls=%d", err, store.calls)
	}
	if _, err := s.Update(context.Background(), admin, "id", Patch{Code: &code, BirthDate: &empty, Notes: &empty}); err != nil {
		t.Fatal(err)
	}
	c := store.changes
	if *c.Code != "M-9" || !c.ClearBirthDate || c.BirthDate != nil || *c.Notes != "" || c.Species != nil {
		t.Fatalf("changes=%+v", c)
	}
	if _, err := s.Update(context.Background(), admin, "id", Patch{BirthDate: &date}); err != nil || store.changes.ClearBirthDate || !store.changes.BirthDate.Equal(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("set birth date: %+v %v", store.changes, err)
	}
}

func TestListAllowlistsQuery(t *testing.T) {
	viewer := access.Actor{UserID: "u", Role: access.Viewer}
	store := &fakeStore{}
	s := New(store, clock)
	for _, q := range []Query{{Sort: "notes"}, {Order: "up"}, {Species: "CAT"}, {Sex: "f"}, {Page: -1}, {PageSize: -1}, {Search: strings.Repeat("a", 101)}} {
		if _, err := s.List(context.Background(), viewer, q); !errors.Is(err, ErrInvalidQuery) {
			t.Errorf("%+v: %v", q, err)
		}
	}
	if _, err := s.List(context.Background(), viewer, Query{}); err != nil || store.filter != (Filter{Sort: "createdAt", Descending: true, Limit: 10}) {
		t.Fatalf("defaults: %+v %v", store.filter, err)
	}
	page, err := s.List(context.Background(), viewer, Query{Search: " c57 ", Species: "RAT", Sex: "FEMALE", Sort: "code", Page: 2, PageSize: 500})
	want := Filter{Search: "c57", Species: domain.Rat, Sex: domain.Female, Sort: "code", Limit: 100, Offset: 100}
	if err != nil || store.filter != want || page.Page != 2 || page.PageSize != 100 {
		t.Fatalf("filter=%+v page=%+v err=%v", store.filter, page, err)
	}
}

type fakeStore struct {
	calls   int
	changes Changes
	filter  Filter
}

func (f *fakeStore) Create(_ context.Context, s domain.Subject) (domain.Subject, error) {
	f.calls++
	return s, nil
}

func (f *fakeStore) Subject(context.Context, string) (domain.Subject, error) {
	f.calls++
	return domain.Subject{}, nil
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
