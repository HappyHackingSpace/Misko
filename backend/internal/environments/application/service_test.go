package application

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/environments/domain"
	"maps"
	"slices"
	"testing"
)

var ctx = context.Background()

var errStoreCalled = errors.New("store called")

// guard converts a call on the nil embedded store into errStoreCalled.
func guard(fn func() error) (err error) {
	defer func() {
		if recover() != nil {
			err = errStoreCalled
		}
	}()
	return fn()
}

func validEnvironment() EnvironmentInput {
	return EnvironmentInput{Name: "Arena A", ParadigmKey: "OPEN_FIELD", Revision: RevisionInput{ParadigmVersion: 1, Apparatus: map[string]float64{"arena_width_cm": 50}}}
}

func TestEveryOperationRequiresItsPermission(t *testing.T) {
	name := "Arena B"
	operations := []struct {
		name       string
		permission access.Permission
		run        func(*Service, access.Actor) error
	}{
		{"list", access.Read, func(s *Service, a access.Actor) error { return second(s.List(ctx, a, "")) }},
		{"get", access.Read, func(s *Service, a access.Actor) error { return second(s.Get(ctx, a, "env")) }},
		{"create", access.ApparatusWrite, func(s *Service, a access.Actor) error { return second(s.Create(ctx, a, validEnvironment())) }},
		{"update", access.ApparatusWrite, func(s *Service, a access.Actor) error { return second(s.Update(ctx, a, "env", Patch{Name: &name})) }},
		{"add revision", access.ApparatusWrite, func(s *Service, a access.Actor) error {
			return second(s.AddRevision(ctx, a, "env", validEnvironment().Revision))
		}},
		{"revisions", access.Read, func(s *Service, a access.Actor) error { return second(s.Revisions(ctx, a, "env")) }},
		{"all revisions", access.Read, func(s *Service, a access.Actor) error { return second(s.AllRevisions(ctx, a)) }},
		{"revision", access.Read, func(s *Service, a access.Actor) error { return second(s.Revision(ctx, a, "env", 1)) }},
	}
	for _, role := range append(access.Roles(), "ROOT") {
		for _, op := range operations {
			err := guard(func() error { return op.run(New(&fakeStore{}, &fakeCatalog{}), access.Actor{UserID: "u", Role: role}) })
			if role.Allows(op.permission) {
				if errors.Is(err, access.ErrForbidden) {
					t.Errorf("%s %s: denied", role, op.name)
				}
				continue
			}
			if !errors.Is(err, access.ErrForbidden) {
				t.Errorf("%s %s: err=%v want forbidden before any store call", role, op.name, err)
			}
		}
	}
	for _, op := range operations {
		if err := guard(func() error { return op.run(New(&fakeStore{}, &fakeCatalog{}), access.Actor{}) }); !errors.Is(err, access.ErrUnauthenticated) {
			t.Errorf("anonymous %s: %v", op.name, err)
		}
	}
}

func TestInvalidEnvironmentsNeverReachTheStore(t *testing.T) {
	manager := access.Actor{UserID: "u", Role: access.LabManager}
	input := func(mutate func(*EnvironmentInput)) EnvironmentInput {
		in := validEnvironment()
		mutate(&in)
		return in
	}
	for name, tc := range map[string]struct {
		run  func(*Service) error
		want error
	}{
		"blank name": {func(s *Service) error {
			return second(s.Create(ctx, manager, input(func(in *EnvironmentInput) { in.Name = " " })))
		}, domain.ErrInvalidName},
		"malformed paradigm key": {func(s *Service) error {
			return second(s.Create(ctx, manager, input(func(in *EnvironmentInput) { in.ParadigmKey = "open field" })))
		}, domain.ErrInvalidParadigmKey},
		"geometry rejected by the catalog": {func(s *Service) error {
			return second(s.Create(ctx, manager, input(func(in *EnvironmentInput) { in.Revision.Apparatus = map[string]float64{"arena_width_cm": 999} })))
		}, ErrInvalidApparatus},
		"unknown paradigm version": {func(s *Service) error {
			return second(s.Create(ctx, manager, input(func(in *EnvironmentInput) { in.Revision.ParadigmVersion = 7 })))
		}, ErrUnknownParadigmVersion},
		"empty patch":          {func(s *Service) error { return second(s.Update(ctx, manager, "env", Patch{})) }, ErrNoChanges},
		"malformed list query": {func(s *Service) error { return second(s.List(ctx, manager, "open field")) }, ErrInvalidQuery},
		"revision number zero": {func(s *Service) error { return second(s.Revision(ctx, manager, "env", 0)) }, ErrRevisionNotFound},
	} {
		store := &fakeStore{}
		err := guard(func() error { return tc.run(New(store, &fakeCatalog{})) })
		if !errors.Is(err, tc.want) || len(store.calls) > 0 {
			t.Errorf("%s: err=%v calls=%v want %v", name, err, store.calls, tc.want)
		}
	}
}

func TestCreateStoresTheFirstRevisionInOneTransaction(t *testing.T) {
	store, catalog := &fakeStore{}, &fakeCatalog{}
	researcher := access.Actor{UserID: "user-1", Role: access.Researcher}
	created, err := New(store, catalog).Create(ctx, researcher, validEnvironment())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(store.calls, []string{"Transaction", "CreateEnvironment", "CreateRevision"}) {
		t.Fatalf("store calls: %v", store.calls)
	}
	r := created.Revision
	if r.EnvironmentID != "env" || r.ParadigmKey != "OPEN_FIELD" || r.CreatedBy != "user-1" || r.Apparatus["arena_width_cm"] != 50 {
		t.Fatalf("revision: %+v", r)
	}
	if !slices.Equal(catalog.checked, []string{"OPEN_FIELD"}) {
		t.Fatalf("catalog: %v", catalog.checked)
	}
}

// A revision always uses the environment's own paradigm and is numbered while
// the environment is locked.
func TestAddRevisionUsesTheEnvironmentParadigmUnderLock(t *testing.T) {
	store, catalog := &fakeStore{paradigm: "EPM"}, &fakeCatalog{}
	r, err := New(store, catalog).AddRevision(ctx, access.Actor{UserID: "user-1", Role: access.SuperAdmin}, "env", RevisionInput{ParadigmVersion: 1, Apparatus: map[string]float64{"arm_length_cm": 35}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(store.calls, []string{"Transaction", "LockEnvironment", "CreateRevision"}) || r.ParadigmKey != "EPM" || !slices.Equal(catalog.checked, []string{"EPM"}) {
		t.Fatalf("calls=%v revision=%+v catalog=%v", store.calls, r, catalog.checked)
	}
	store = &fakeStore{paradigm: "EPM"}
	_, err = New(store, &fakeCatalog{}).AddRevision(ctx, access.Actor{UserID: "user-1", Role: access.SuperAdmin}, "env", RevisionInput{ParadigmVersion: 1, Apparatus: map[string]float64{"arm_length_cm": 999}})
	if !errors.Is(err, ErrInvalidApparatus) || slices.Contains(store.calls, "CreateRevision") {
		t.Fatalf("invalid revision: err=%v calls=%v", err, store.calls)
	}
}

// fakeCatalog accepts version 1 with values up to 100.
type fakeCatalog struct{ checked []string }

func (c *fakeCatalog) CheckApparatus(key string, version int, apparatus map[string]float64) error {
	c.checked = append(c.checked, key)
	if version != 1 {
		return ErrUnknownParadigmVersion
	}
	for v := range maps.Values(apparatus) {
		if v > 100 {
			return ErrInvalidApparatus
		}
	}
	return nil
}

// fakeStore implements only what these tests use; any other call panics and
// guard reports it as a store call.
type fakeStore struct {
	Store
	calls    []string
	paradigm string
}

func (f *fakeStore) Transaction(_ context.Context, fn func(Store) error) error {
	f.calls = append(f.calls, "Transaction")
	return fn(f)
}

func (f *fakeStore) CreateEnvironment(_ context.Context, e domain.Environment) (domain.Environment, error) {
	f.calls = append(f.calls, "CreateEnvironment")
	e.ID = "env"
	return e, nil
}

func (f *fakeStore) LockEnvironment(_ context.Context, id string) (domain.Environment, error) {
	f.calls = append(f.calls, "LockEnvironment")
	return domain.Environment{ID: id, Name: "Maze", ParadigmKey: f.paradigm, LatestRevision: 1}, nil
}

func (f *fakeStore) CreateRevision(_ context.Context, r domain.Revision) (domain.Revision, error) {
	f.calls = append(f.calls, "CreateRevision")
	return r, nil
}

func second[T any](_ T, err error) error { return err }
