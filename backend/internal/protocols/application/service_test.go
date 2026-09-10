package application

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/protocols/domain"
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

func openFieldStep(position int) StepInput {
	return StepInput{Position: position, ParadigmKey: "OPEN_FIELD", ParadigmVersion: 1, EnvironmentRevisionID: "arena-r1", TrialType: "STANDARD", Trials: 1}
}

func validProtocol() ProtocolInput {
	return ProtocolInput{Name: "Battery", Version: VersionInput{Steps: []StepInput{openFieldStep(1)}}}
}

func TestEveryOperationRequiresItsPermission(t *testing.T) {
	name := "Renamed"
	operations := []struct {
		name       string
		permission access.Permission
		run        func(*Service, access.Actor) error
	}{
		{"list", access.Read, func(s *Service, a access.Actor) error { return second(s.List(ctx, a, "exp")) }},
		{"get", access.Read, func(s *Service, a access.Actor) error { return second(s.Get(ctx, a, "exp", "p")) }},
		{"create", access.ApparatusWrite, func(s *Service, a access.Actor) error { return second(s.Create(ctx, a, "exp", validProtocol())) }},
		{"update", access.ApparatusWrite, func(s *Service, a access.Actor) error {
			return second(s.Update(ctx, a, "exp", "p", Patch{Name: &name}))
		}},
		{"add version", access.ApparatusWrite, func(s *Service, a access.Actor) error {
			return second(s.AddVersion(ctx, a, "exp", "p", validProtocol().Version))
		}},
		{"versions", access.Read, func(s *Service, a access.Actor) error { return second(s.Versions(ctx, a, "exp", "p")) }},
		{"version", access.Read, func(s *Service, a access.Actor) error { return second(s.Version(ctx, a, "exp", "p", 1)) }},
	}
	for _, role := range append(access.Roles(), "ROOT") {
		for _, op := range operations {
			err := guard(func() error { return op.run(New(&fakeStore{}, fakeCatalog{}), access.Actor{UserID: "u", Role: role}) })
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
		if err := guard(func() error { return op.run(New(&fakeStore{}, fakeCatalog{}), access.Actor{}) }); !errors.Is(err, access.ErrUnauthenticated) {
			t.Errorf("anonymous %s: %v", op.name, err)
		}
	}
}

func TestStepsMustMatchTheirEnvironmentAndParadigm(t *testing.T) {
	researcher := access.Actor{UserID: "user-1", Role: access.Researcher}
	for name, tc := range map[string]struct {
		mutate func(*StepInput)
		want   error
	}{
		"invalid order":                 {func(s *StepInput) { s.Position = 2 }, domain.ErrInvalidStepOrder},
		"environment of other paradigm": {func(s *StepInput) { s.EnvironmentRevisionID = "maze-r1" }, ErrIncompatibleEnvironment},
		"environment of other version":  {func(s *StepInput) { s.EnvironmentRevisionID = "arena-v2" }, ErrIncompatibleEnvironment},
		"unknown environment revision":  {func(s *StepInput) { s.EnvironmentRevisionID = "missing" }, ErrEnvironmentRevisionNotFound},
		"trial type of other paradigm":  {func(s *StepInput) { s.TrialType = "PROBE" }, ErrTrialTypeNotSupported},
		"unknown paradigm":              {func(s *StepInput) { s.ParadigmKey = "MAZE" }, ErrUnknownParadigm},
		"session rejected by catalog":   {func(s *StepInput) { s.Session = map[string]float64{"max_sample_gap_s": 99} }, ErrInvalidSession},
	} {
		t.Run(name, func(t *testing.T) {
			store := &fakeStore{}
			in := validProtocol()
			tc.mutate(&in.Version.Steps[0])
			err := guard(func() error { return second(New(store, fakeCatalog{}).Create(ctx, researcher, "exp", in)) })
			if !errors.Is(err, tc.want) || slices.Contains(store.calls, "CreateProtocol") || slices.Contains(store.calls, "CreateVersion") {
				t.Fatalf("err=%v calls=%v want %v", err, store.calls, tc.want)
			}
		})
	}
}

func TestVersionCombinesParadigmsWithResolvedSessions(t *testing.T) {
	store := &fakeStore{}
	epm := StepInput{Position: 1, ParadigmKey: "EPM", ParadigmVersion: 1, EnvironmentRevisionID: "maze-r1", TrialType: "STANDARD", Trials: 1}
	openField := openFieldStep(2)
	openField.Trials, openField.InterTrialIntervalS = 3, 600
	openField.Session = map[string]float64{"max_sample_gap_s": 0.25}
	in := ProtocolInput{Name: "Battery", Version: VersionInput{Notes: "baseline", Steps: []StepInput{openField, epm}}}
	created, err := New(store, fakeCatalog{}).Create(ctx, access.Actor{UserID: "user-1", Role: access.LabManager}, "exp", in)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(store.calls, []string{"Transaction", "EnvironmentRevision", "EnvironmentRevision", "CreateProtocol", "CreateVersion"}) {
		t.Fatalf("store calls: %v", store.calls)
	}
	v := created.Version
	if v.ProtocolID != "protocol" || v.ExperimentID != "exp" || v.CreatedBy != "user-1" || len(v.Steps) != 2 {
		t.Fatalf("version: %+v", v)
	}
	maze, arena := v.Steps[0], v.Steps[1]
	if maze.ParadigmKey != "EPM" || arena.ParadigmKey != "OPEN_FIELD" || arena.Trials != 3 || arena.InterTrialIntervalS != 600 ||
		arena.EnvironmentID != "arena" || arena.EnvironmentRevision != 1 {
		t.Fatalf("steps: %+v", v.Steps)
	}
	// The catalog resolves session defaults against the revision's apparatus.
	if !maps.Equal(arena.Session, map[string]float64{"max_sample_gap_s": 0.25, "arena_width_cm_seen": 50}) {
		t.Fatalf("resolved session: %v", arena.Session)
	}
}

func TestAddVersionLocksTheProtocol(t *testing.T) {
	store := &fakeStore{}
	if _, err := New(store, fakeCatalog{}).AddVersion(ctx, access.Actor{UserID: "u", Role: access.SuperAdmin}, "exp", "protocol", validProtocol().Version); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(store.calls, []string{"Transaction", "LockProtocol", "EnvironmentRevision", "CreateVersion"}) {
		t.Fatalf("store calls: %v", store.calls)
	}
	empty := &fakeStore{}
	if err := guard(func() error {
		return second(New(empty, fakeCatalog{}).Update(ctx, access.Actor{UserID: "u", Role: access.SuperAdmin}, "exp", "p", Patch{}))
	}); !errors.Is(err, ErrNoChanges) || len(empty.calls) > 0 {
		t.Fatalf("empty patch: %v %v", err, empty.calls)
	}
	if err := guard(func() error {
		return second(New(empty, fakeCatalog{}).Version(ctx, access.Actor{UserID: "u", Role: access.Viewer}, "exp", "p", 0))
	}); !errors.Is(err, ErrVersionNotFound) || len(empty.calls) > 0 {
		t.Fatalf("version zero: %v %v", err, empty.calls)
	}
}

// fakeCatalog knows OPEN_FIELD and EPM version 1 with trial type STANDARD. It
// rejects session values above 10 and reports the apparatus it was given.
type fakeCatalog struct{}

func (fakeCatalog) TrialTypes(key string, version int) ([]string, error) {
	if key != "OPEN_FIELD" && key != "EPM" {
		return nil, ErrUnknownParadigm
	}
	if version != 1 {
		return nil, ErrUnknownParadigmVersion
	}
	return []string{"STANDARD"}, nil
}

func (fakeCatalog) ResolveSession(key string, version int, apparatus, session map[string]float64) (map[string]float64, error) {
	out := maps.Clone(session)
	if out == nil {
		out = map[string]float64{}
	}
	for _, v := range session {
		if v > 10 {
			return nil, ErrInvalidSession
		}
	}
	if width, ok := apparatus["arena_width_cm"]; ok {
		out["arena_width_cm_seen"] = width
	}
	return out, nil
}

// fakeStore implements only what these tests use; any other call panics and
// guard reports it as a store call.
type fakeStore struct {
	Store
	calls []string
}

func (f *fakeStore) Transaction(_ context.Context, fn func(Store) error) error {
	f.calls = append(f.calls, "Transaction")
	return fn(f)
}

func (f *fakeStore) EnvironmentRevision(_ context.Context, id string) (RevisionRef, error) {
	f.calls = append(f.calls, "EnvironmentRevision")
	switch id {
	case "arena-r1":
		return RevisionRef{ID: id, EnvironmentID: "arena", Number: 1, ParadigmKey: "OPEN_FIELD", ParadigmVersion: 1, Apparatus: map[string]float64{"arena_width_cm": 50}}, nil
	case "arena-v2":
		return RevisionRef{ID: id, EnvironmentID: "arena", Number: 2, ParadigmKey: "OPEN_FIELD", ParadigmVersion: 2}, nil
	case "maze-r1":
		return RevisionRef{ID: id, EnvironmentID: "maze", Number: 1, ParadigmKey: "EPM", ParadigmVersion: 1}, nil
	}
	return RevisionRef{}, ErrEnvironmentRevisionNotFound
}

func (f *fakeStore) CreateProtocol(_ context.Context, p domain.Protocol) (domain.Protocol, error) {
	f.calls = append(f.calls, "CreateProtocol")
	p.ID = "protocol"
	return p, nil
}

func (f *fakeStore) LockProtocol(_ context.Context, experimentID, protocolID string) (domain.Protocol, error) {
	f.calls = append(f.calls, "LockProtocol")
	return domain.Protocol{ID: protocolID, ExperimentID: experimentID, Name: "Battery", LatestVersion: 1}, nil
}

func (f *fakeStore) CreateVersion(_ context.Context, v domain.Version) (domain.Version, error) {
	f.calls = append(f.calls, "CreateVersion")
	return v, nil
}

func second[T any](_ T, err error) error { return err }
