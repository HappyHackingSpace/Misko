//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"fmt"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/ratelimit"
	"github.com/HappyHackingSpace/Misko/backend/internal/tests/adapters/postgres"
	"github.com/HappyHackingSpace/Misko/backend/internal/tests/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/tests/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"slices"
	"sync"
	"testing"
	"time"
)

var (
	ctx        = context.Background()
	researcher = access.Actor{UserID: "01a00000-0000-7000-8000-000000000001", Role: access.Researcher}
	technician = access.Actor{UserID: "01a00000-0000-7000-8000-000000000002", Role: access.Technician}
	viewer     = access.Actor{UserID: "01a00000-0000-7000-8000-000000000003", Role: access.Viewer}
	manager    = access.Actor{UserID: "01a00000-0000-7000-8000-000000000004", Role: access.LabManager}
	unknown    = "01a00000-0000-7000-8000-00000000dead"
	day        = 24 * time.Hour
	t0         = time.Now().UTC().Add(-10 * day).Truncate(time.Second)
)

// fixture creates the experiment, enrollment and protocol context with SQL;
// adapters of other domains are not imported.
type fixture struct {
	t    *testing.T
	pool *pgxpool.Pool
	s    *application.Service
	n    int
}

func newFixture(t *testing.T, limiter application.Limiter) *fixture {
	pool := pgtest.Database(t)
	return &fixture{t: t, pool: pool, s: application.New(postgres.NewStore(pool), limiter, time.Now)}
}

func (f *fixture) id(sql string, args ...any) string {
	f.t.Helper()
	var id string
	if err := f.pool.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
		f.t.Fatal(err)
	}
	return id
}

func (f *fixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(ctx, sql, args...); err != nil {
		f.t.Fatal(err)
	}
}

type study struct {
	experiment, subject, enrollment, group, otherGroup, phase, version string
}

// newStudy enrolls a subject at t0 into a group and publishes a protocol whose
// version has an OPEN_FIELD step with 2 trials and an EPM step with 1 trial.
func (f *fixture) newStudy() study {
	f.n++
	var s study
	s.experiment = f.id("INSERT INTO misko.experiments (code, title) VALUES ($1, 'Study') RETURNING id", fmt.Sprintf("E%d", f.n))
	s.subject = f.id("INSERT INTO misko.subjects (code, species, sex) VALUES ($1, 'MOUSE', 'MALE') RETURNING id", fmt.Sprintf("S%d", f.n))
	s.enrollment = f.id("INSERT INTO misko.enrollments (experiment_id, subject_id, enrolled_at) VALUES ($1, $2, $3) RETURNING id", s.experiment, s.subject, t0)
	s.group = f.id("INSERT INTO misko.experiment_groups (experiment_id, name, role) VALUES ($1, 'Vehicle', 'CONTROL') RETURNING id", s.experiment)
	s.otherGroup = f.id("INSERT INTO misko.experiment_groups (experiment_id, name, role) VALUES ($1, 'Drug', 'TREATMENT') RETURNING id", s.experiment)
	f.id("INSERT INTO misko.group_assignments (experiment_id, enrollment_id, group_id, valid_from) VALUES ($1, $2, $3, $4) RETURNING id", s.experiment, s.enrollment, s.group, t0)
	s.phase = f.id("INSERT INTO misko.experiment_phases (experiment_id, name, position) VALUES ($1, 'Baseline', 1) RETURNING id", s.experiment)
	arena := f.id("INSERT INTO misko.environments (name, paradigm_key) VALUES ($1, 'OPEN_FIELD') RETURNING id", fmt.Sprintf("Arena %d", f.n))
	arena1 := f.id(`INSERT INTO misko.environment_revisions (environment_id, paradigm_key, number, paradigm_version, apparatus, created_by)
		VALUES ($1, 'OPEN_FIELD', 1, 1, '{}', $2) RETURNING id`, arena, researcher.UserID)
	maze := f.id("INSERT INTO misko.environments (name, paradigm_key) VALUES ($1, 'EPM') RETURNING id", fmt.Sprintf("Maze %d", f.n))
	maze1 := f.id(`INSERT INTO misko.environment_revisions (environment_id, paradigm_key, number, paradigm_version, apparatus, created_by)
		VALUES ($1, 'EPM', 1, 1, '{}', $2) RETURNING id`, maze, researcher.UserID)
	protocol := f.id("INSERT INTO misko.protocols (experiment_id, name) VALUES ($1, 'Battery') RETURNING id", s.experiment)
	err := pgx.BeginFunc(ctx, f.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO misko.protocol_versions (experiment_id, protocol_id, number, step_count, created_by)
			VALUES ($1, $2, 1, 2, $3) RETURNING id`, s.experiment, protocol, researcher.UserID).Scan(&s.version); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO misko.protocol_steps (protocol_version_id, position, paradigm_key, paradigm_version, environment_revision_id, trial_type, trials, inter_trial_interval_s, session)
			VALUES ($1, 1, 'OPEN_FIELD', 1, $2, 'STANDARD', 2, 0, '{}'), ($1, 2, 'EPM', 1, $3, 'STANDARD', 1, 0, '{}')`, s.version, arena1, maze1)
		return err
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return s
}

func (f *fixture) plan(s study, position int) domain.Test {
	f.t.Helper()
	test, err := f.s.CreateTest(ctx, researcher, s.experiment, application.TestInput{
		EnrollmentID: s.enrollment, PhaseID: s.phase, ProtocolVersionID: s.version, StepPosition: position, ScheduledAt: t0.Add(time.Hour),
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return test
}

func TestTestContextIsValidatedAgainstTheExperiment(t *testing.T) {
	f := newFixture(t, ratelimit.New(20, time.Minute, time.Now))
	s, other := f.newStudy(), f.newStudy()
	test := f.plan(s, 1)
	if test.SubjectID != s.subject || test.GroupID != s.group || test.PhaseID != s.phase || test.ParadigmKey != "OPEN_FIELD" ||
		test.PlannedTrials != 2 || test.Status != domain.Planned || test.CreatedBy != researcher.UserID || !test.ScheduledAt.Equal(t0.Add(time.Hour)) {
		t.Fatalf("test: %+v", test)
	}
	if maze := f.plan(s, 2); maze.ParadigmKey != "EPM" || maze.PlannedTrials != 1 {
		t.Fatalf("second step: %+v", maze)
	}
	valid := application.TestInput{EnrollmentID: s.enrollment, PhaseID: s.phase, ProtocolVersionID: s.version, StepPosition: 1, ScheduledAt: t0.Add(time.Hour)}
	for name, tc := range map[string]struct {
		experiment string
		mutate     func(*application.TestInput)
		want       error
	}{
		"enrollment of another experiment": {s.experiment, func(in *application.TestInput) { in.EnrollmentID = other.enrollment }, application.ErrEnrollmentNotFound},
		"phase of another experiment":      {s.experiment, func(in *application.TestInput) { in.PhaseID = other.phase }, application.ErrPhaseNotFound},
		"protocol of another experiment":   {s.experiment, func(in *application.TestInput) { in.ProtocolVersionID = other.version }, application.ErrProtocolVersionNotFound},
		"step missing from the version":    {s.experiment, func(in *application.TestInput) { in.StepPosition = 3 }, application.ErrStepNotFound},
		"malformed enrollment":             {s.experiment, func(in *application.TestInput) { in.EnrollmentID = "enr" }, application.ErrEnrollmentNotFound},
		"malformed protocol version":       {s.experiment, func(in *application.TestInput) { in.ProtocolVersionID = "v1" }, application.ErrProtocolVersionNotFound},
		"unknown experiment":               {unknown, func(*application.TestInput) {}, application.ErrEnrollmentNotFound},
		"scheduled before enrollment":      {s.experiment, func(in *application.TestInput) { in.ScheduledAt = t0.Add(-time.Hour) }, domain.ErrBeforeEnrollment},
	} {
		in := valid
		tc.mutate(&in)
		if _, err := f.s.CreateTest(ctx, researcher, tc.experiment, in); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}

	// The schema rejects the same combinations if the use case is bypassed.
	for name, sql := range map[string]string{
		"step paradigm differs": `INSERT INTO misko.tests (experiment_id, enrollment_id, subject_id, protocol_version_id, step_position, paradigm_key, paradigm_version, environment_revision_id, planned_trials, scheduled_at, created_by)
			SELECT experiment_id, enrollment_id, subject_id, protocol_version_id, step_position, 'EPM', paradigm_version, environment_revision_id, planned_trials, scheduled_at, created_by FROM misko.tests WHERE id = $1`,
		"subject of another enrollment": `INSERT INTO misko.tests (experiment_id, enrollment_id, subject_id, protocol_version_id, step_position, paradigm_key, paradigm_version, environment_revision_id, planned_trials, scheduled_at, created_by)
			SELECT experiment_id, enrollment_id, '` + other.subject + `', protocol_version_id, step_position, paradigm_key, paradigm_version, environment_revision_id, planned_trials, scheduled_at, created_by FROM misko.tests WHERE id = $1`,
		"group of another experiment": `INSERT INTO misko.tests (experiment_id, enrollment_id, subject_id, group_id, protocol_version_id, step_position, paradigm_key, paradigm_version, environment_revision_id, planned_trials, scheduled_at, created_by)
			SELECT experiment_id, enrollment_id, subject_id, '` + other.group + `', protocol_version_id, step_position, paradigm_key, paradigm_version, environment_revision_id, planned_trials, scheduled_at, created_by FROM misko.tests WHERE id = $1`,
		"started while planned": "UPDATE misko.tests SET started_at = now() WHERE id = $1",
		"context changed":       "UPDATE misko.tests SET scheduled_at = scheduled_at + interval '1 day' WHERE id = $1",
		"skipped to completed":  "UPDATE misko.tests SET status = 'COMPLETED', started_at = now(), completed_at = now() WHERE id = $1",
		"deleted":               "DELETE FROM misko.tests WHERE id = $1",
	} {
		if _, err := f.pool.Exec(ctx, sql, test.ID); err == nil {
			t.Errorf("%s: direct SQL succeeded", name)
		}
	}

	page, err := f.s.ListTests(ctx, viewer, s.experiment, application.TestQuery{ParadigmKey: "EPM"})
	if err != nil || page.Total != 1 || page.Tests[0].ParadigmKey != "EPM" {
		t.Fatalf("filtered list: %+v %v", page, err)
	}
	if page, err := f.s.ListTests(ctx, viewer, s.experiment, application.TestQuery{SubjectID: s.subject, Status: "PLANNED", PageSize: 1}); err != nil || page.Total != 2 || len(page.Tests) != 1 {
		t.Fatalf("paged list: %+v %v", page, err)
	}
	if page, err := f.s.ListTests(ctx, viewer, other.experiment, application.TestQuery{}); err != nil || page.Total != 0 {
		t.Fatalf("another experiment: %+v %v", page, err)
	}
}

func TestLifecycleTransitionsAndTrials(t *testing.T) {
	f := newFixture(t, ratelimit.New(20, time.Minute, time.Now))
	s := f.newStudy()
	test := f.plan(s, 1)

	// Concurrent starts: exactly one wins.
	var wg sync.WaitGroup
	var mu sync.Mutex
	var started int
	var rejected []error
	startAt := time.Now().UTC().Add(-time.Hour)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.s.StartTest(ctx, technician, test.ID, startAt)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				started++
			} else {
				rejected = append(rejected, err)
			}
		}()
	}
	wg.Wait()
	if started != 1 || len(rejected) != 7 || slices.ContainsFunc(rejected, func(err error) bool { return !errors.Is(err, domain.ErrInvalidTransition) }) {
		t.Fatalf("concurrent starts: %d started, rejected %v", started, rejected)
	}

	now := time.Now().UTC()
	end := now.Add(-time.Second)
	first, err := f.s.RecordTrial(ctx, technician, test.ID, domain.TrialInput{Repetition: 1, StartedAt: now.Add(-3 * time.Second), EndedAt: &end})
	if err != nil || first.Number != 1 || first.Attempt != 1 || first.RecordedBy != technician.UserID {
		t.Fatalf("first trial: %+v %v", first, err)
	}
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := f.s.RecordTrial(ctx, technician, test.ID, domain.TrialInput{Repetition: 1, StartedAt: time.Now().UTC()}); err != nil {
				t.Errorf("concurrent repeat: %v", err)
			}
		}()
	}
	wg.Wait()
	if _, err := f.s.RecordTrial(ctx, technician, test.ID, domain.TrialInput{Repetition: 3, StartedAt: time.Now().UTC()}); !errors.Is(err, domain.ErrInvalidRepetition) {
		t.Fatalf("repetition beyond plan: %v", err)
	}
	trials, err := f.s.Trials(ctx, viewer, test.ID)
	if err != nil || len(trials) != 6 {
		t.Fatalf("trials: %d %v", len(trials), err)
	}
	var numbers, attempts []int
	for _, tr := range trials {
		numbers, attempts = append(numbers, tr.Number), append(attempts, tr.Attempt)
	}
	if !slices.Equal(numbers, []int{1, 2, 3, 4, 5, 6}) || !slices.Equal(attempts, []int{1, 2, 3, 4, 5, 6}) || trials[0].ID != first.ID || trials[0].EndedAt == nil || !trials[0].EndedAt.Equal(end.Truncate(time.Microsecond)) {
		t.Fatalf("repeats must append without replacing: %+v", trials)
	}
	for name, sql := range map[string]string{
		"edit trial":      "UPDATE misko.trials SET notes = 'edited' WHERE test_id = $1",
		"delete trial":    "DELETE FROM misko.trials WHERE test_id = $1",
		"back to planned": "UPDATE misko.tests SET status = 'PLANNED', started_at = NULL WHERE id = $1",
		"move group":      "UPDATE misko.tests SET group_id = NULL WHERE id = $1",
	} {
		if _, err := f.pool.Exec(ctx, sql, test.ID); err == nil {
			t.Errorf("%s: direct SQL succeeded", name)
		}
	}

	var completed int
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.s.CompleteTest(ctx, technician, test.ID, time.Time{})
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				completed++
			} else if !errors.Is(err, domain.ErrInvalidTransition) {
				t.Errorf("concurrent completion: %v", err)
			}
		}()
	}
	wg.Wait()
	if completed != 1 {
		t.Fatalf("concurrent completions: %d", completed)
	}
	if _, err := f.s.RecordTrial(ctx, technician, test.ID, domain.TrialInput{Repetition: 2, StartedAt: time.Now().UTC()}); !errors.Is(err, domain.ErrTestNotInProgress) {
		t.Fatalf("trial after completion: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO misko.trials (test_id, number, repetition, attempt, started_at, recorded_by) VALUES ($1, 99, 2, 1, now(), $2)`, test.ID, technician.UserID); err == nil {
		t.Error("a trial was added to a completed test with direct SQL")
	}
	if _, err := f.s.CancelTest(ctx, researcher, test.ID, "mistake"); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("cancel a completed test: %v", err)
	}
	detail, err := f.s.Test(ctx, viewer, test.ID)
	if err != nil || detail.Test.Status != domain.Completed || detail.Test.StartedAt == nil || detail.Test.CompletedAt == nil || len(detail.Trials) != 6 {
		t.Fatalf("detail: %+v %v", detail, err)
	}

	// A test whose subject moved to another group after planning cannot start.
	moved := f.plan(s, 2)
	crossover := time.Now().UTC().Add(-day)
	f.exec("UPDATE misko.group_assignments SET valid_to = $1 WHERE enrollment_id = $2", crossover, s.enrollment)
	f.id("INSERT INTO misko.group_assignments (experiment_id, enrollment_id, group_id, valid_from) VALUES ($1, $2, $3, $4) RETURNING id", s.experiment, s.enrollment, s.otherGroup, crossover)
	if _, err := f.s.StartTest(ctx, technician, moved.ID, time.Time{}); !errors.Is(err, domain.ErrGroupChanged) {
		t.Fatalf("start after crossover: %v", err)
	}
	cancelled, err := f.s.CancelTest(ctx, researcher, moved.ID, "moved to another group")
	if err != nil || cancelled.Status != domain.Cancelled || cancelled.CancelReason != "moved to another group" {
		t.Fatalf("cancel: %+v %v", cancelled, err)
	}
	if _, err := f.s.StartTest(ctx, technician, moved.ID, time.Time{}); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("start a cancelled test: %v", err)
	}
	for name, err := range map[string]error{
		"unknown test":           second(f.s.Test(ctx, viewer, unknown)),
		"malformed test":         second(f.s.StartTest(ctx, technician, "test", time.Time{})),
		"trials of unknown test": second(f.s.Trials(ctx, viewer, unknown)),
	} {
		if !errors.Is(err, application.ErrTestNotFound) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestCommentsBelongToTheirTest(t *testing.T) {
	// The viewer's creates and edits, including rejected edits, use up 5 changes per minute.
	f := newFixture(t, ratelimit.New(5, time.Minute, time.Now))
	s := f.newStudy()
	test, other := f.plan(s, 1), f.plan(s, 2)
	f.exec("INSERT INTO misko.users (id, email, name, role, password_hash) VALUES ($1, 'viewer@lab.io', 'Vera Viewer', 'VIEWER', 'x')", viewer.UserID)

	c, err := f.s.CreateComment(ctx, viewer, test.ID, "Subject looked calm")
	if err != nil || c.AuthorID != viewer.UserID || c.AuthorName != "Vera Viewer" || c.Body != "Subject looked calm" {
		t.Fatalf("viewer comment: %+v %v", c, err)
	}
	for name, tc := range map[string]struct {
		err, want error
	}{
		"edit by another user":        {second(f.s.UpdateComment(ctx, manager, test.ID, c.ID, "changed")), application.ErrCannotEditComment},
		"delete by a researcher":      {f.s.DeleteComment(ctx, researcher, test.ID, c.ID), application.ErrCannotDeleteComment},
		"edit through another test":   {second(f.s.UpdateComment(ctx, viewer, other.ID, c.ID, "changed")), application.ErrCommentNotFound},
		"delete through another test": {f.s.DeleteComment(ctx, manager, other.ID, c.ID), application.ErrCommentNotFound},
		"comment on an unknown test":  {second(f.s.CreateComment(ctx, researcher, unknown, "hello")), application.ErrTestNotFound},
		"comments of an unknown test": {second(f.s.Comments(ctx, researcher, unknown)), application.ErrTestNotFound},
		"malformed comment":           {second(f.s.UpdateComment(ctx, viewer, test.ID, "comment", "changed")), application.ErrCommentNotFound},
	} {
		if !errors.Is(tc.err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, tc.err, tc.want)
		}
	}
	edited, err := f.s.UpdateComment(ctx, viewer, test.ID, c.ID, "Subject looked calm at first")
	if err != nil || edited.Body != "Subject looked calm at first" || !edited.UpdatedAt.After(edited.CreatedAt) {
		t.Fatalf("author edit: %+v %v", edited, err)
	}
	if _, err := f.s.CreateComment(ctx, viewer, test.ID, "third change"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.CreateComment(ctx, viewer, test.ID, "fourth change"); !errors.Is(err, application.ErrRateLimited) {
		t.Fatalf("rate limit: %v", err)
	}
	if _, err := f.s.CreateComment(ctx, researcher, test.ID, "another user"); err != nil {
		t.Fatalf("limits are per user: %v", err)
	}
	f.exec("DELETE FROM misko.users WHERE id = $1", viewer.UserID)
	list, err := f.s.Comments(ctx, technician, test.ID)
	if err != nil || len(list) != 3 || list[0].ID != c.ID || list[0].AuthorName != "" || list[0].AuthorID != viewer.UserID {
		t.Fatalf("comments stay with their test, oldest first: %+v %v", list, err)
	}
	if err := f.s.DeleteComment(ctx, manager, test.ID, c.ID); err != nil {
		t.Fatalf("moderator delete: %v", err)
	}
	if list, _ := f.s.Comments(ctx, technician, test.ID); len(list) != 2 {
		t.Fatalf("after delete: %+v", list)
	}
	if others, _ := f.s.Comments(ctx, technician, other.ID); len(others) != 0 {
		t.Fatalf("another test's comments: %+v", others)
	}
}

func second[T any](_ T, err error) error { return err }
