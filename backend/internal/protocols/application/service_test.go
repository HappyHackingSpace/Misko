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

func tankStep(position int) StepInput {
	return StepInput{Position: position, ParadigmKey: "MWM", ParadigmVersion: 1, TrialType: "STANDARD", Trials: 1}
}

func validProtocol() ProtocolInput {
	return ProtocolInput{Name: "Battery", Version: VersionInput{Steps: []StepInput{tankStep(1)}}}
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
		{"create", access.StudyWrite, func(s *Service, a access.Actor) error { return second(s.Create(ctx, a, "exp", validProtocol())) }},
		{"update", access.StudyWrite, func(s *Service, a access.Actor) error {
			return second(s.Update(ctx, a, "exp", "p", Patch{Name: &name}))
		}},
		{"add version", access.StudyWrite, func(s *Service, a access.Actor) error {
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

func TestStepsMustMatchTheirParadigm(t *testing.T) {
	researcher := access.Actor{UserID: "user-1", Role: access.Researcher}
	for name, tc := range map[string]struct {
		mutate func(*StepInput)
		want   error
	}{
		"invalid order":                {func(s *StepInput) { s.Position = 2 }, domain.ErrInvalidStepOrder},
		"trial type of other paradigm": {func(s *StepInput) { s.TrialType = "PROBE" }, ErrTrialTypeNotSupported},
		"unknown paradigm":             {func(s *StepInput) { s.ParadigmKey = "MAZE" }, ErrUnknownParadigm},
		"unknown paradigm version":     {func(s *StepInput) { s.ParadigmVersion = 2 }, ErrUnknownParadigmVersion},
		"session rejected by catalog":  {func(s *StepInput) { s.Session = map[string]float64{"max_sample_gap_s": 99} }, ErrInvalidSession},
	} {
		t.Run(name, func(t *testing.T) {
			store := &fakeStore{}
			in := validProtocol()
			tc.mutate(&in.Version.Steps[0])
			err := guard(func() error { return second(New(store, fakeCatalog{}).Create(ctx, researcher, "exp", in)) })
			if !errors.Is(err, tc.want) || len(store.calls) > 0 {
				t.Fatalf("err=%v calls=%v want %v", err, store.calls, tc.want)
			}
		})
	}
}

func TestVersionCombinesParadigmsWithResolvedSessions(t *testing.T) {
	store := &fakeStore{}
	pole := StepInput{Position: 1, ParadigmKey: "POLE", ParadigmVersion: 1, TrialType: "STANDARD", Trials: 1}
	tank := tankStep(2)
	tank.Trials, tank.InterTrialIntervalS = 3, 600
	tank.Session = map[string]float64{"max_sample_gap_s": 0.25}
	in := ProtocolInput{Name: "Battery", Version: VersionInput{Notes: "baseline", Steps: []StepInput{tank, pole}}}
	created, err := New(store, fakeCatalog{}).Create(ctx, access.Actor{UserID: "user-1", Role: access.LabManager}, "exp", in)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(store.calls, []string{"Transaction", "CreateProtocol", "CreateVersion"}) {
		t.Fatalf("store calls: %v", store.calls)
	}
	v := created.Version
	if v.ProtocolID != "protocol" || v.ExperimentID != "exp" || v.CreatedBy != "user-1" || len(v.Steps) != 2 {
		t.Fatalf("version: %+v", v)
	}
	savedPole, savedTank := v.Steps[0], v.Steps[1]
	if savedPole.ParadigmKey != "POLE" || savedTank.ParadigmKey != "MWM" || savedTank.Trials != 3 || savedTank.InterTrialIntervalS != 600 {
		t.Fatalf("steps: %+v", v.Steps)
	}
	// The catalog fills in session defaults.
	if !maps.Equal(savedTank.Session, map[string]float64{"max_sample_gap_s": 0.25, "default_seen": 1}) || !maps.Equal(savedPole.Session, map[string]float64{"default_seen": 1}) {
		t.Fatalf("resolved session: %v", savedTank.Session)
	}
}

func TestAddVersionLocksTheProtocol(t *testing.T) {
	store := &fakeStore{}
	if _, err := New(store, fakeCatalog{}).AddVersion(ctx, access.Actor{UserID: "u", Role: access.SuperAdmin}, "exp", "protocol", validProtocol().Version); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(store.calls, []string{"Transaction", "LockProtocol", "CreateVersion"}) {
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

// fakeCatalog knows MWM and POLE version 1 with trial type STANDARD. It
// rejects session values above 10 and adds a default.
type fakeCatalog struct{}

func (fakeCatalog) TrialTypes(key string, version int) ([]string, error) {
	if key != "MWM" && key != "POLE" {
		return nil, ErrUnknownParadigm
	}
	if version != 1 {
		return nil, ErrUnknownParadigmVersion
	}
	return []string{"STANDARD"}, nil
}

func (fakeCatalog) ResolveSession(key string, version int, session map[string]float64) (map[string]float64, error) {
	out := maps.Clone(session)
	if out == nil {
		out = map[string]float64{}
	}
	for _, v := range session {
		if v > 10 {
			return nil, ErrInvalidSession
		}
	}
	out["default_seen"] = 1
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
