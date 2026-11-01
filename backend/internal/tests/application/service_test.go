package application

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/tests/domain"
	"slices"
	"strings"
	"testing"
	"time"
)

var (
	ctx      = context.Background()
	now      = time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	clock    = func() time.Time { return now }
	daySeven = time.Date(2026, 9, 8, 22, 0, 0, 0, time.UTC)
)

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

func validTest() TestInput {
	return TestInput{TimepointID: "day7", ProtocolVersionID: "v1", StepPosition: 1, ScheduledAt: now.Add(-time.Hour),
		Subjects: []domain.Participant{{AllocationID: "a1", EnvironmentRevisionID: "box1"}, {AllocationID: "a2", EnvironmentRevisionID: "box2"}}}
}

type allowAll struct{}

func (allowAll) Allow(string) bool { return true }

func TestEveryOperationRequiresItsPermission(t *testing.T) {
	participant := domain.Participant{AllocationID: "a3", EnvironmentRevisionID: "box3"}
	operations := []struct {
		name       string
		permission access.Permission
		run        func(*Service, access.Actor) error
	}{
		{"list tests", access.Read, func(s *Service, a access.Actor) error { return second(s.ListTests(ctx, a, "exp", TestQuery{})) }},
		{"list all tests", access.Read, func(s *Service, a access.Actor) error { return second(s.ListAllTests(ctx, a, TestQuery{})) }},
		{"create test", access.TestWrite, func(s *Service, a access.Actor) error { return second(s.CreateTest(ctx, a, "exp", validTest())) }},
		{"add animal", access.TestWrite, func(s *Service, a access.Actor) error { return second(s.AddSubject(ctx, a, "test", participant)) }},
		{"remove animal", access.TestWrite, func(s *Service, a access.Actor) error { return second(s.RemoveSubject(ctx, a, "test", "s1")) }},
		{"get test", access.Read, func(s *Service, a access.Actor) error { return second(s.Test(ctx, a, "test")) }},
		{"start test", access.TestRun, func(s *Service, a access.Actor) error { return second(s.StartTest(ctx, a, "test", now)) }},
		{"complete test", access.TestRun, func(s *Service, a access.Actor) error { return second(s.CompleteTest(ctx, a, "test", now)) }},
		{"cancel test", access.TestWrite, func(s *Service, a access.Actor) error { return second(s.CancelTest(ctx, a, "test", "unwell")) }},
		{"list trials", access.Read, func(s *Service, a access.Actor) error { return second(s.Trials(ctx, a, "test", "s1")) }},
		{"record trial", access.TestRun, func(s *Service, a access.Actor) error {
			return second(s.RecordTrial(ctx, a, "test", "s1", domain.TrialInput{Repetition: 1, StartedAt: now}))
		}},
		{"list comments", access.Read, func(s *Service, a access.Actor) error { return second(s.Comments(ctx, a, "test")) }},
		{"create comment", access.Read, func(s *Service, a access.Actor) error { return second(s.CreateComment(ctx, a, "test", "hello")) }},
		{"update comment", access.Read, func(s *Service, a access.Actor) error { return second(s.UpdateComment(ctx, a, "test", "c", "hello")) }},
		{"delete comment", access.Read, func(s *Service, a access.Actor) error { return s.DeleteComment(ctx, a, "test", "c") }},
	}
	for _, role := range append(access.Roles(), "ROOT") {
		for _, op := range operations {
			err := guard(func() error {
				return op.run(New(&fakeStore{}, allowAll{}, clock), access.Actor{UserID: "u", Role: role})
			})
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
		if err := guard(func() error { return op.run(New(&fakeStore{}, allowAll{}, clock), access.Actor{}) }); !errors.Is(err, access.ErrUnauthenticated) {
			t.Errorf("anonymous %s: %v", op.name, err)
		}
	}
}

var researcher = access.Actor{UserID: "user-1", Role: access.Researcher}

func TestCreateTakesItsContextFromTheExperiment(t *testing.T) {
	store := newStore()
	created, err := New(store, allowAll{}, clock).CreateTest(ctx, researcher, "exp", validTest())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Transaction", "ExperimentStatus", "Timepoint", "ProtocolStep", "Arenas", "LockAllocations", "ScheduledAllocations", "CreateTest"}
	if !slices.Equal(store.calls, want) {
		t.Fatalf("store calls: %v", store.calls)
	}
	if created.ParadigmKey != "MWM" || created.PlannedTrials != 2 || created.CreatedBy != "user-1" || created.Status != domain.Planned ||
		created.TimepointID != "day7" || len(created.Subjects) != 2 || created.Subjects[1].EnvironmentRevisionID != "box2" {
		t.Fatalf("test: %+v", created)
	}
	if !slices.Equal(store.checked, []string{"a1", "a2"}) || store.slot != (Slot{"day7", "v1", 1}) || store.except != "" {
		t.Fatalf("conflict check: %v %+v %q", store.checked, store.slot, store.except)
	}

	unscheduled := validTest()
	unscheduled.ScheduledAt = time.Time{}
	if created, err := New(newStore(), allowAll{}, clock).CreateTest(ctx, researcher, "exp", unscheduled); err != nil || !created.ScheduledAt.Equal(daySeven) {
		t.Fatalf("default schedule is day 0 plus the study day: %v %v", created.ScheduledAt, err)
	}

	for name, tc := range map[string]struct {
		store  func(*fakeStore)
		mutate func(*TestInput)
		want   error
	}{
		"draft experiment":                {func(f *fakeStore) { f.status = "DRAFT" }, nil, ErrExperimentNotRunning},
		"completed experiment":            {func(f *fakeStore) { f.status = "COMPLETED" }, nil, ErrExperimentNotRunning},
		"timepoint of another experiment": {nil, func(in *TestInput) { in.TimepointID = "foreign" }, ErrTimepointNotFound},
		"protocol of another experiment":  {nil, func(in *TestInput) { in.ProtocolVersionID = "foreign" }, ErrProtocolVersionNotFound},
		"step missing from the version":   {nil, func(in *TestInput) { in.StepPosition = 9 }, ErrStepNotFound},
		"step position zero":              {nil, func(in *TestInput) { in.StepPosition = 0 }, ErrStepNotFound},
		"no animals":                      {nil, func(in *TestInput) { in.Subjects = nil }, domain.ErrNoSubjects},
		"same arena twice":                {nil, func(in *TestInput) { in.Subjects[1].EnvironmentRevisionID = "box1" }, domain.ErrDuplicateArena},
		"unknown arena":                   {nil, func(in *TestInput) { in.Subjects[1].EnvironmentRevisionID = "missing" }, ErrArenaNotFound},
		"arena of another paradigm":       {nil, func(in *TestInput) { in.Subjects[1].EnvironmentRevisionID = "maze" }, ErrArenaParadigmMismatch},
		"arena of another version":        {nil, func(in *TestInput) { in.Subjects[1].EnvironmentRevisionID = "box-v2" }, ErrArenaParadigmMismatch},
		"animal of another experiment":    {nil, func(in *TestInput) { in.Subjects[1].AllocationID = "foreign" }, ErrAllocationNotFound},
		"excluded animal":                 {nil, func(in *TestInput) { in.Subjects[1].AllocationID = "excluded" }, ErrAnimalExcluded},
		"animal already in the slot":      {func(f *fakeStore) { f.taken = []string{"a2"} }, nil, ErrAnimalAlreadyScheduled},
	} {
		store := newStore()
		if tc.store != nil {
			tc.store(store)
		}
		in := validTest()
		if tc.mutate != nil {
			tc.mutate(&in)
		}
		if _, err := New(store, allowAll{}, clock).CreateTest(ctx, researcher, "exp", in); !errors.Is(err, tc.want) || slices.Contains(store.calls, "CreateTest") {
			t.Errorf("%s: err=%v calls=%v want %v", name, err, store.calls, tc.want)
		}
	}
}

func TestAnimalsJoinAndLeaveAPlannedTest(t *testing.T) {
	store := newStore()
	_, err := New(store, allowAll{}, clock).AddSubject(ctx, researcher, "test", domain.Participant{AllocationID: "a3", EnvironmentRevisionID: "box3"})
	if err != nil || !slices.Equal(store.calls, []string{"Transaction", "LockTest", "Arenas", "LockAllocations", "ScheduledAllocations", "AddTestSubject", "Test"}) {
		t.Fatalf("add: %v %v", err, store.calls)
	}
	if store.added.AllocationID != "a3" || store.added.TestID != "test" || store.added.CreatedBy != "user-1" || store.except != "test" || store.slot != (Slot{"day7", "v1", 1}) {
		t.Fatalf("added: %+v except=%q slot=%+v", store.added, store.except, store.slot)
	}
	for name, tc := range map[string]struct {
		store func(*fakeStore)
		in    domain.Participant
		want  error
	}{
		"occupied arena":      {nil, domain.Participant{AllocationID: "a3", EnvironmentRevisionID: "box1"}, domain.ErrDuplicateArena},
		"animal already in":   {nil, domain.Participant{AllocationID: "a1", EnvironmentRevisionID: "box3"}, domain.ErrDuplicateAnimal},
		"test already begun":  {func(f *fakeStore) { f.test.Status = domain.InProgress }, domain.Participant{AllocationID: "a3", EnvironmentRevisionID: "box3"}, domain.ErrNotPlanned},
		"scheduled elsewhere": {func(f *fakeStore) { f.taken = []string{"a3"} }, domain.Participant{AllocationID: "a3", EnvironmentRevisionID: "box3"}, ErrAnimalAlreadyScheduled},
	} {
		store := newStore()
		if tc.store != nil {
			tc.store(store)
		}
		if _, err := New(store, allowAll{}, clock).AddSubject(ctx, researcher, "test", tc.in); !errors.Is(err, tc.want) || slices.Contains(store.calls, "AddTestSubject") {
			t.Errorf("%s: err=%v calls=%v want %v", name, err, store.calls, tc.want)
		}
	}

	store = newStore()
	if _, err := New(store, allowAll{}, clock).RemoveSubject(ctx, researcher, "test", "s2"); err != nil || store.removed != "s2" {
		t.Fatalf("remove: %v %q", err, store.removed)
	}
	for name, tc := range map[string]struct {
		store   func(*fakeStore)
		subject string
		want    error
	}{
		"unknown animal":     {nil, "missing", ErrTestSubjectNotFound},
		"last animal":        {func(f *fakeStore) { f.test.Subjects = f.test.Subjects[:1] }, "s1", domain.ErrLastSubject},
		"test already begun": {func(f *fakeStore) { f.test.Status = domain.InProgress }, "s1", domain.ErrNotPlanned},
	} {
		store := newStore()
		if tc.store != nil {
			tc.store(store)
		}
		if _, err := New(store, allowAll{}, clock).RemoveSubject(ctx, researcher, "test", tc.subject); !errors.Is(err, tc.want) || store.removed != "" {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}
}

// Transitions lock the test first, so concurrent requests see its current status.
func TestTransitionsWorkOnTheLockedTest(t *testing.T) {
	technician := access.Actor{UserID: "tech", Role: access.Technician}
	store := newStore()
	started, err := New(store, allowAll{}, clock).StartTest(ctx, technician, "test", time.Time{})
	if err != nil || started.Status != domain.InProgress || !started.StartedAt.Equal(now) {
		t.Fatalf("start with the current time: %+v %v", started, err)
	}
	if !slices.Equal(store.calls, []string{"Transaction", "LockTest", "UpdateTestStatus"}) || store.updatedFrom != domain.Planned {
		t.Fatalf("start calls: %v from %s", store.calls, store.updatedFrom)
	}
	excluded := newStore()
	excluded.test.Subjects[1].Excluded = true
	if _, err := New(excluded, allowAll{}, clock).StartTest(ctx, technician, "test", time.Time{}); !errors.Is(err, domain.ErrSubjectExcluded) {
		t.Fatalf("start with an excluded animal: %v", err)
	}

	running := newStore()
	running.test = started
	trial, err := New(running, allowAll{}, clock).RecordTrial(ctx, technician, "test", "s1", domain.TrialInput{Repetition: 1, StartedAt: now})
	if err != nil || trial.Attempt != 2 || trial.Number != 2 || trial.TestSubjectID != "s1" || !slices.Equal(running.calls, []string{"Transaction", "LockTest", "Trials", "CreateTrial"}) {
		t.Fatalf("trial: %+v %v %v", trial, err, running.calls)
	}
	if _, err := New(running, allowAll{}, clock).RecordTrial(ctx, technician, "test", "other", domain.TrialInput{Repetition: 1, StartedAt: now}); !errors.Is(err, ErrTestSubjectNotFound) {
		t.Fatalf("trial of an animal of another test: %v", err)
	}
	trials, err := New(running, allowAll{}, clock).Trials(ctx, technician, "test", "s2")
	if err != nil || len(trials) != 1 || trials[0].TestSubjectID != "s2" {
		t.Fatalf("trials of one animal: %+v %v", trials, err)
	}
	running.calls = nil
	completed, err := New(running, allowAll{}, clock).CompleteTest(ctx, technician, "test", time.Time{})
	if err != nil || completed.Status != domain.Completed || !slices.Equal(running.calls, []string{"Transaction", "LockTest", "Trials", "UpdateTestStatus"}) || running.updatedFrom != domain.InProgress {
		t.Fatalf("complete: %+v %v %v", completed, err, running.calls)
	}

	cancelling := newStore()
	cancelled, err := New(cancelling, allowAll{}, clock).CancelTest(ctx, researcher, "test", " unwell ")
	if err != nil || cancelled.Status != domain.Cancelled || cancelled.CancelReason != "unwell" {
		t.Fatalf("cancel: %+v %v", cancelled, err)
	}
}

func TestListQueriesAreValidated(t *testing.T) {
	viewer := access.Actor{UserID: "v", Role: access.Viewer}
	for name, q := range map[string]TestQuery{
		"unknown status":  {Status: "DONE"},
		"unknown in list": {Status: "PLANNED,DONE"},
		"unknown order":   {Order: "up"},
		"malformed date":  {From: "11/09/2026"},
		"reversed range":  {From: "2026-09-12", To: "2026-09-11"},
		"negative page":   {Page: -1},
		"negative size":   {PageSize: -1},
		"long search":     {Search: strings.Repeat("x", 101)},
		"NUL in search":   {Search: "M\x0001"},
	} {
		store := newStore()
		if _, err := New(store, allowAll{}, clock).ListAllTests(ctx, viewer, q); !errors.Is(err, ErrInvalidQuery) || len(store.calls) > 0 {
			t.Errorf("%s: %v %v", name, err, store.calls)
		}
	}
	store := newStore()
	page, err := New(store, allowAll{}, clock).ListTests(ctx, viewer, "exp", TestQuery{ExperimentID: "ignored", Status: "PLANNED", From: "2026-09-01", To: "2026-09-30", PageSize: 500})
	f := store.filter
	if err != nil || f.ExperimentID != "exp" || !slices.Equal(f.Statuses, []domain.Status{domain.Planned}) || f.From != "2026-09-01" || f.To != "2026-09-30" || f.NewestFirst || f.Limit != 100 || page.PageSize != 100 {
		t.Fatalf("experiment list: %+v %+v %v", f, page, err)
	}
	if _, err := New(store, allowAll{}, clock).ListAllTests(ctx, viewer, TestQuery{TimepointID: "day7", Search: " M1 "}); err != nil || !store.filter.NewestFirst || store.filter.TimepointID != "day7" || store.filter.Search != "M1" || store.filter.Limit != 20 {
		t.Fatalf("worklist: %+v %v", store.filter, err)
	}
	// Overdue work is planned or in progress; the worklist can ask for both,
	// earliest first.
	q := TestQuery{Status: "PLANNED, IN_PROGRESS", Order: "asc"}
	if _, err := New(store, allowAll{}, clock).ListAllTests(ctx, viewer, q); err != nil || store.filter.NewestFirst ||
		!slices.Equal(store.filter.Statuses, []domain.Status{domain.Planned, domain.InProgress}) {
		t.Fatalf("overdue worklist: %+v %v", store.filter, err)
	}
}

func TestCommentOwnershipAndRateLimit(t *testing.T) {
	viewer := access.Actor{UserID: "viewer", Role: access.Viewer}
	store := &fakeStore{}
	c, err := New(store, allowAll{}, clock).CreateComment(ctx, viewer, "test", "  looks fine\x00 ")
	if err != nil || c.AuthorID != "viewer" || c.Body != "looks fine" || c.TestID != "test" {
		t.Fatalf("viewer comment: %+v %v", c, err)
	}
	for name, tc := range map[string]struct {
		run  func(*Service) error
		want error
	}{
		"edit another author's comment": {func(s *Service) error {
			return second(s.UpdateComment(ctx, access.Actor{UserID: "other", Role: access.SuperAdmin}, "test", "c", "changed"))
		}, ErrCannotEditComment},
		"researcher deletes another author's comment": {func(s *Service) error {
			return s.DeleteComment(ctx, access.Actor{UserID: "other", Role: access.Researcher}, "test", "c")
		}, ErrCannotDeleteComment},
		"comment of another test": {func(s *Service) error {
			return second(s.UpdateComment(ctx, access.Actor{UserID: "author", Role: access.Viewer}, "other-test", "c", "changed"))
		}, ErrCommentNotFound},
		"empty body": {func(s *Service) error { return second(s.CreateComment(ctx, viewer, "test", " \x01 ")) }, domain.ErrInvalidComment},
	} {
		store := &fakeStore{}
		if err := tc.run(New(store, allowAll{}, clock)); !errors.Is(err, tc.want) || slices.Contains(store.calls, "UpdateComment") || slices.Contains(store.calls, "DeleteComment") || slices.Contains(store.calls, "CreateComment") {
			t.Errorf("%s: err=%v calls=%v want %v", name, err, store.calls, tc.want)
		}
	}
	for _, actor := range []access.Actor{{UserID: "author", Role: access.Viewer}, {UserID: "moderator", Role: access.LabManager}} {
		store := &fakeStore{}
		if err := New(store, allowAll{}, clock).DeleteComment(ctx, actor, "test", "c"); err != nil || !slices.Contains(store.calls, "DeleteComment") {
			t.Errorf("%s deletes: %v", actor.UserID, err)
		}
	}
	edited, err := New(&fakeStore{}, allowAll{}, clock).UpdateComment(ctx, access.Actor{UserID: "author", Role: access.Viewer}, "test", "c", " changed ")
	if err != nil || edited.Body != "changed" {
		t.Fatalf("author edit: %+v %v", edited, err)
	}

	limiter := &countingLimiter{max: 2}
	s := New(&fakeStore{}, limiter, clock)
	for i := range 2 {
		if _, err := s.CreateComment(ctx, viewer, "test", "hello"); err != nil {
			t.Fatalf("comment %d: %v", i+1, err)
		}
	}
	limited := &fakeStore{}
	s = New(limited, limiter, clock)
	if _, err := s.CreateComment(ctx, viewer, "test", "hello"); !errors.Is(err, ErrRateLimited) || len(limited.calls) > 0 {
		t.Fatalf("third comment: %v %v", err, limited.calls)
	}
	if _, err := s.UpdateComment(ctx, access.Actor{UserID: "viewer", Role: access.Viewer}, "test", "c", "x"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("edits share the limit: %v", err)
	}
	if !slices.Equal(limiter.keys, []string{"viewer", "viewer", "viewer", "viewer"}) {
		t.Fatalf("limited by user: %v", limiter.keys)
	}
}

type countingLimiter struct {
	max  int
	keys []string
}

func (l *countingLimiter) Allow(key string) bool {
	l.keys = append(l.keys, key)
	return len(l.keys) <= l.max
}

func plannedTest() domain.Test {
	t, err := domain.NewTest(domain.Context{
		ExperimentID: "exp", TimepointID: "day7", ProtocolVersionID: "v1", StepPosition: 1, ParadigmKey: "MWM", ParadigmVersion: 1, PlannedTrials: 2,
	}, now.Add(-time.Hour), "", "user", []domain.Participant{{AllocationID: "a1", EnvironmentRevisionID: "box1"}, {AllocationID: "a2", EnvironmentRevisionID: "box2"}})
	if err != nil {
		panic(err)
	}
	t.ID = "test"
	t.Subjects[0].ID, t.Subjects[1].ID = "s1", "s2"
	return t
}

func newStore() *fakeStore { return &fakeStore{status: Running, test: plannedTest()} }

// fakeStore implements only what these tests use; any other call panics and
// guard reports it as a store call.
type fakeStore struct {
	Store
	calls       []string
	status      string
	test        domain.Test
	taken       []string
	checked     []string
	slot        Slot
	except      string
	added       domain.TestSubject
	removed     string
	filter      TestFilter
	updatedFrom domain.Status
}

func (f *fakeStore) record(name string) { f.calls = append(f.calls, name) }

func (f *fakeStore) Transaction(_ context.Context, fn func(Store) error) error {
	f.record("Transaction")
	return fn(f)
}

func (f *fakeStore) ExperimentStatus(context.Context, string) (string, error) {
	f.record("ExperimentStatus")
	return f.status, nil
}

func (f *fakeStore) Timepoint(_ context.Context, _, id string) (TimepointRef, error) {
	f.record("Timepoint")
	if id == "foreign" {
		return TimepointRef{}, ErrTimepointNotFound
	}
	return TimepointRef{ID: id, StudyDay: 7, DefaultScheduledAt: daySeven}, nil
}

func (f *fakeStore) ProtocolStep(_ context.Context, _, versionID string, position int) (StepRef, error) {
	f.record("ProtocolStep")
	switch {
	case versionID == "foreign":
		return StepRef{}, ErrProtocolVersionNotFound
	case position != 1:
		return StepRef{}, ErrStepNotFound
	}
	return StepRef{ProtocolVersionID: versionID, Position: 1, ParadigmKey: "MWM", ParadigmVersion: 1, Trials: 2}, nil
}

func (f *fakeStore) Arenas(_ context.Context, ids []string) ([]ArenaRef, error) {
	f.record("Arenas")
	var out []ArenaRef
	for _, id := range ids {
		switch id {
		case "missing":
		case "maze":
			out = append(out, ArenaRef{ID: id, ParadigmKey: "POLE", ParadigmVersion: 1})
		case "box-v2":
			out = append(out, ArenaRef{ID: id, ParadigmKey: "MWM", ParadigmVersion: 2})
		default:
			out = append(out, ArenaRef{ID: id, ParadigmKey: "MWM", ParadigmVersion: 1})
		}
	}
	return out, nil
}

func (f *fakeStore) LockAllocations(_ context.Context, _ string, ids []string) ([]AllocationRef, error) {
	f.record("LockAllocations")
	var out []AllocationRef
	for _, id := range ids {
		if id != "foreign" {
			out = append(out, AllocationRef{ID: id, SubjectID: "subject-" + id, Excluded: id == "excluded"})
		}
	}
	return out, nil
}

func (f *fakeStore) ScheduledAllocations(_ context.Context, slot Slot, ids []string, except string) ([]string, error) {
	f.record("ScheduledAllocations")
	f.slot, f.checked, f.except = slot, ids, except
	return f.taken, nil
}

func (f *fakeStore) CreateTest(_ context.Context, t domain.Test) (domain.Test, error) {
	f.record("CreateTest")
	t.ID = "test"
	return t, nil
}

func (f *fakeStore) AddTestSubject(_ context.Context, _ string, s domain.TestSubject, _ string, _ int) error {
	f.record("AddTestSubject")
	f.added = s
	return nil
}

func (f *fakeStore) RemoveTestSubject(_ context.Context, _, id string) error {
	f.record("RemoveTestSubject")
	f.removed = id
	return nil
}

func (f *fakeStore) Test(context.Context, string) (domain.Test, error) {
	f.record("Test")
	return f.test, nil
}

func (f *fakeStore) LockTest(context.Context, string) (domain.Test, error) {
	f.record("LockTest")
	return f.test, nil
}

func (f *fakeStore) UpdateTestStatus(_ context.Context, t domain.Test, from domain.Status) (domain.Test, error) {
	f.record("UpdateTestStatus")
	f.updatedFrom = from
	return t, nil
}

func (f *fakeStore) ListTests(_ context.Context, filter TestFilter) ([]domain.Test, int, error) {
	f.record("ListTests")
	f.filter = filter
	return []domain.Test{f.test}, 1, nil
}

func (f *fakeStore) Trials(context.Context, string) ([]domain.Trial, error) {
	f.record("Trials")
	return []domain.Trial{
		{ID: "t1", TestID: "test", TestSubjectID: "s1", Number: 1, Repetition: 1, Attempt: 1, StartedAt: now.Add(-time.Minute)},
		{ID: "t2", TestID: "test", TestSubjectID: "s2", Number: 1, Repetition: 1, Attempt: 1, StartedAt: now.Add(-time.Minute)},
	}, nil
}

func (f *fakeStore) CreateTrial(_ context.Context, tr domain.Trial) (domain.Trial, error) {
	f.record("CreateTrial")
	return tr, nil
}

func (f *fakeStore) Comment(_ context.Context, testID, commentID string) (domain.Comment, error) {
	f.record("Comment")
	if testID != "test" {
		return domain.Comment{}, ErrCommentNotFound
	}
	return domain.Comment{ID: commentID, TestID: testID, AuthorID: "author", Body: "original"}, nil
}

func (f *fakeStore) CreateComment(_ context.Context, c domain.Comment) (domain.Comment, error) {
	f.record("CreateComment")
	return c, nil
}

func (f *fakeStore) UpdateComment(_ context.Context, testID, commentID, body string) (domain.Comment, error) {
	f.record("UpdateComment")
	return domain.Comment{ID: commentID, TestID: testID, AuthorID: "author", Body: body}, nil
}

func (f *fakeStore) DeleteComment(context.Context, string, string) error {
	f.record("DeleteComment")
	return nil
}

func second[T any](_ T, err error) error { return err }
