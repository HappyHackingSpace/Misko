package application

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/tests/domain"
	"slices"
	"testing"
	"time"
)

var (
	ctx      = context.Background()
	now      = time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	clock    = func() time.Time { return now }
	enrolled = now.Add(-10 * 24 * time.Hour)
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
	return TestInput{EnrollmentID: "enr", PhaseID: "phase", ProtocolVersionID: "v1", StepPosition: 1, ScheduledAt: now.Add(-time.Hour)}
}

type allowAll struct{}

func (allowAll) Allow(string) bool { return true }

func TestEveryOperationRequiresItsPermission(t *testing.T) {
	operations := []struct {
		name       string
		permission access.Permission
		run        func(*Service, access.Actor) error
	}{
		{"list tests", access.Read, func(s *Service, a access.Actor) error { return second(s.ListTests(ctx, a, "exp", TestQuery{})) }},
		{"create test", access.TestWrite, func(s *Service, a access.Actor) error { return second(s.CreateTest(ctx, a, "exp", validTest())) }},
		{"get test", access.Read, func(s *Service, a access.Actor) error { return second(s.Test(ctx, a, "test")) }},
		{"start test", access.TestRun, func(s *Service, a access.Actor) error { return second(s.StartTest(ctx, a, "test", now)) }},
		{"complete test", access.TestRun, func(s *Service, a access.Actor) error { return second(s.CompleteTest(ctx, a, "test", now)) }},
		{"cancel test", access.TestWrite, func(s *Service, a access.Actor) error { return second(s.CancelTest(ctx, a, "test", "unwell")) }},
		{"list trials", access.Read, func(s *Service, a access.Actor) error { return second(s.Trials(ctx, a, "test")) }},
		{"record trial", access.TestRun, func(s *Service, a access.Actor) error {
			return second(s.RecordTrial(ctx, a, "test", domain.TrialInput{Repetition: 1, StartedAt: now}))
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

func TestCreateTakesItsContextFromTheExperiment(t *testing.T) {
	store := &fakeStore{group: "group"}
	researcher := access.Actor{UserID: "user-1", Role: access.Researcher}
	created, err := New(store, allowAll{}, clock).CreateTest(ctx, researcher, "exp", validTest())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(store.calls, []string{"Transaction", "Enrollment", "Phase", "ProtocolStep", "GroupAt", "CreateTest"}) {
		t.Fatalf("store calls: %v", store.calls)
	}
	if created.SubjectID != "subject" || created.GroupID != "group" || created.ParadigmKey != "EPM" || created.EnvironmentRevisionID != "rev" ||
		created.PlannedTrials != 2 || created.CreatedBy != "user-1" || created.Status != domain.Planned {
		t.Fatalf("test: %+v", created)
	}

	for name, tc := range map[string]struct {
		mutate func(*TestInput)
		want   error
	}{
		"enrollment of another experiment": {func(in *TestInput) { in.EnrollmentID = "foreign" }, ErrEnrollmentNotFound},
		"phase of another experiment":      {func(in *TestInput) { in.PhaseID = "foreign" }, ErrPhaseNotFound},
		"protocol of another experiment":   {func(in *TestInput) { in.ProtocolVersionID = "foreign" }, ErrProtocolVersionNotFound},
		"step missing from the version":    {func(in *TestInput) { in.StepPosition = 9 }, ErrStepNotFound},
		"scheduled before enrollment":      {func(in *TestInput) { in.ScheduledAt = enrolled.Add(-time.Hour) }, domain.ErrBeforeEnrollment},
		"no schedule":                      {func(in *TestInput) { in.ScheduledAt = time.Time{} }, domain.ErrMissingTime},
		"step position zero":               {func(in *TestInput) { in.StepPosition = 0 }, ErrStepNotFound},
	} {
		store := &fakeStore{group: "group"}
		in := validTest()
		tc.mutate(&in)
		if _, err := New(store, allowAll{}, clock).CreateTest(ctx, researcher, "exp", in); !errors.Is(err, tc.want) || slices.Contains(store.calls, "CreateTest") {
			t.Errorf("%s: err=%v calls=%v want %v", name, err, store.calls, tc.want)
		}
	}
	noPhase := validTest()
	noPhase.PhaseID = ""
	store = &fakeStore{}
	if _, err := New(store, allowAll{}, clock).CreateTest(ctx, researcher, "exp", noPhase); err != nil || slices.Contains(store.calls, "Phase") {
		t.Fatalf("test without phase: %v %v", err, store.calls)
	}
}

// Transitions lock the test first, so concurrent requests see its current status.
func TestTransitionsWorkOnTheLockedTest(t *testing.T) {
	technician := access.Actor{UserID: "tech", Role: access.Technician}
	store := &fakeStore{test: plannedTest(), group: "group"}
	started, err := New(store, allowAll{}, clock).StartTest(ctx, technician, "test", time.Time{})
	if err != nil || started.Status != domain.InProgress || !started.StartedAt.Equal(now) {
		t.Fatalf("start with the current time: %+v %v", started, err)
	}
	if !slices.Equal(store.calls, []string{"Transaction", "LockTest", "LockEnrollment", "GroupAt", "UpdateTestStatus"}) || store.updatedFrom != domain.Planned {
		t.Fatalf("start calls: %v from %s", store.calls, store.updatedFrom)
	}

	moved := &fakeStore{test: plannedTest(), group: "other-group"}
	if _, err := New(moved, allowAll{}, clock).StartTest(ctx, technician, "test", now.Add(-time.Minute)); !errors.Is(err, domain.ErrGroupChanged) || slices.Contains(moved.calls, "UpdateTestStatus") {
		t.Fatalf("group changed: %v %v", err, moved.calls)
	}

	running := &fakeStore{test: started}
	trial, err := New(running, allowAll{}, clock).RecordTrial(ctx, technician, "test", domain.TrialInput{Repetition: 1, StartedAt: now})
	if err != nil || trial.Attempt != 2 || trial.Number != 2 || !slices.Equal(running.calls, []string{"Transaction", "LockTest", "Trials", "CreateTrial"}) {
		t.Fatalf("trial: %+v %v %v", trial, err, running.calls)
	}
	running.calls = nil
	completed, err := New(running, allowAll{}, clock).CompleteTest(ctx, technician, "test", time.Time{})
	if err != nil || completed.Status != domain.Completed || !slices.Equal(running.calls, []string{"Transaction", "LockTest", "Trials", "UpdateTestStatus"}) || running.updatedFrom != domain.InProgress {
		t.Fatalf("complete: %+v %v %v", completed, err, running.calls)
	}

	cancelling := &fakeStore{test: plannedTest()}
	cancelled, err := New(cancelling, allowAll{}, clock).CancelTest(ctx, access.Actor{UserID: "r", Role: access.Researcher}, "test", " unwell ")
	if err != nil || cancelled.Status != domain.Cancelled || cancelled.CancelReason != "unwell" {
		t.Fatalf("cancel: %+v %v", cancelled, err)
	}
	if err := guard(func() error {
		return second(New(&fakeStore{}, allowAll{}, clock).ListTests(ctx, technician, "exp", TestQuery{Status: "DONE"}))
	}); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("unknown status filter: %v", err)
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
		ExperimentID: "exp", EnrollmentID: "enr", SubjectID: "subject", EnrolledAt: enrolled, GroupID: "group",
		ProtocolVersionID: "v1", StepPosition: 1, ParadigmKey: "EPM", ParadigmVersion: 1, EnvironmentRevisionID: "rev", PlannedTrials: 2,
	}, now.Add(-time.Hour), "", "user")
	if err != nil {
		panic(err)
	}
	t.ID = "test"
	return t
}

// fakeStore implements only what these tests use; any other call panics and
// guard reports it as a store call.
type fakeStore struct {
	Store
	calls       []string
	group       string
	test        domain.Test
	updatedFrom domain.Status
}

func (f *fakeStore) record(name string) { f.calls = append(f.calls, name) }

func (f *fakeStore) Transaction(_ context.Context, fn func(Store) error) error {
	f.record("Transaction")
	return fn(f)
}

func (f *fakeStore) Enrollment(_ context.Context, _, id string) (EnrollmentRef, error) {
	f.record("Enrollment")
	if id == "foreign" {
		return EnrollmentRef{}, ErrEnrollmentNotFound
	}
	return EnrollmentRef{ID: id, ExperimentID: "exp", SubjectID: "subject", EnrolledAt: enrolled}, nil
}

func (f *fakeStore) LockEnrollment(ctx context.Context, experimentID, id string) (EnrollmentRef, error) {
	f.record("LockEnrollment")
	return EnrollmentRef{ID: id, ExperimentID: experimentID, SubjectID: "subject", EnrolledAt: enrolled}, nil
}

func (f *fakeStore) Phase(_ context.Context, _, id string) error {
	f.record("Phase")
	if id == "foreign" {
		return ErrPhaseNotFound
	}
	return nil
}

func (f *fakeStore) ProtocolStep(_ context.Context, _, versionID string, position int) (StepRef, error) {
	f.record("ProtocolStep")
	switch {
	case versionID == "foreign":
		return StepRef{}, ErrProtocolVersionNotFound
	case position != 1:
		return StepRef{}, ErrStepNotFound
	}
	return StepRef{ProtocolVersionID: versionID, Position: 1, ParadigmKey: "EPM", ParadigmVersion: 1, EnvironmentRevisionID: "rev", Trials: 2}, nil
}

func (f *fakeStore) GroupAt(context.Context, string, time.Time) (string, error) {
	f.record("GroupAt")
	return f.group, nil
}

func (f *fakeStore) CreateTest(_ context.Context, t domain.Test) (domain.Test, error) {
	f.record("CreateTest")
	t.ID = "test"
	return t, nil
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

func (f *fakeStore) Trials(context.Context, string) ([]domain.Trial, error) {
	f.record("Trials")
	return []domain.Trial{{ID: "t1", TestID: "test", Number: 1, Repetition: 1, Attempt: 1, StartedAt: now.Add(-time.Minute)}}, nil
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
