//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"fmt"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/experiments/adapters/postgres"
	"github.com/HappyHackingSpace/Misko/backend/internal/experiments/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/experiments/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"sync"
	"testing"
	"time"
)

var (
	ctx        = context.Background()
	researcher = access.Actor{UserID: "researcher", Role: access.Researcher}
	day        = 24 * time.Hour
	t0         = time.Now().UTC().Add(-10 * day).Truncate(time.Second)
)

type fixture struct {
	t     *testing.T
	pool  *pgxpool.Pool
	s     *application.Service
	codes int
}

func newFixture(t *testing.T) *fixture {
	pool := pgtest.Database(t)
	return &fixture{t: t, pool: pool, s: application.New(postgres.NewStore(pool))}
}

// subject inserts a subject row directly; the subjects module is not imported
// because adapters of different domains must stay independent.
func (f *fixture) subject() string {
	f.t.Helper()
	f.codes++
	var id string
	if err := f.pool.QueryRow(ctx, "INSERT INTO misko.subjects (code, species, sex) VALUES ($1, 'MOUSE', 'FEMALE') RETURNING id", fmt.Sprintf("S%d", f.codes)).Scan(&id); err != nil {
		f.t.Fatal(err)
	}
	return id
}

func (f *fixture) experiment(code string, requiresControl bool) domain.Experiment {
	f.t.Helper()
	e, err := f.s.CreateExperiment(ctx, researcher, application.ExperimentInput{Code: code, Title: "Study " + code, RequiresControl: requiresControl})
	if err != nil {
		f.t.Fatal(err)
	}
	return e
}

func (f *fixture) group(experimentID, name, role string, target int) domain.Group {
	f.t.Helper()
	g, err := f.s.CreateGroup(ctx, researcher, experimentID, application.GroupInput{Name: name, Role: role, TargetSize: target})
	if err != nil {
		f.t.Fatal(err)
	}
	return g
}

func (f *fixture) enroll(experimentID, subjectID, groupID string, at time.Time) domain.Enrollment {
	f.t.Helper()
	summary, err := f.s.Enroll(ctx, researcher, experimentID, application.EnrollInput{SubjectID: subjectID, GroupID: groupID, EnrolledAt: at})
	if err != nil {
		f.t.Fatal(err)
	}
	return summary.Enrollment
}

func (f *fixture) sizes(experimentID string) map[string][2]int {
	f.t.Helper()
	sizes, err := f.s.Groups(ctx, researcher, experimentID)
	if err != nil {
		f.t.Fatal(err)
	}
	out := map[string][2]int{}
	for _, size := range sizes {
		out[size.Group.Name] = [2]int{size.Group.TargetSize, size.ActiveSubjects}
	}
	return out
}

func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

func TestPhasesGroupsAndEnrollmentsStayInTheirExperiment(t *testing.T) {
	f := newFixture(t)
	a, b := f.experiment("A", false), f.experiment("B", false)
	phaseB, err := f.s.CreatePhase(ctx, researcher, b.ID, application.PhaseInput{Name: "Baseline", Position: 1})
	if err != nil {
		t.Fatal(err)
	}
	groupA, groupB := f.group(a.ID, "Vehicle", "CONTROL", 0), f.group(b.ID, "Vehicle", "CONTROL", 0)
	name := "Renamed"
	if _, err := f.s.UpdatePhase(ctx, researcher, a.ID, phaseB.ID, application.PhasePatch{Name: &name}); !errors.Is(err, application.ErrPhaseNotFound) {
		t.Errorf("phase of another experiment updated: %v", err)
	}
	if err := f.s.DeletePhase(ctx, researcher, a.ID, phaseB.ID); !errors.Is(err, application.ErrPhaseNotFound) {
		t.Errorf("phase of another experiment deleted: %v", err)
	}
	if _, err := f.s.UpdateGroup(ctx, researcher, a.ID, groupB.ID, application.GroupPatch{Name: &name}); !errors.Is(err, application.ErrGroupNotFound) {
		t.Errorf("group of another experiment updated: %v", err)
	}
	if err := f.s.DeleteGroup(ctx, researcher, a.ID, groupB.ID); !errors.Is(err, application.ErrGroupNotFound) {
		t.Errorf("group of another experiment deleted: %v", err)
	}

	subject := f.subject()
	if _, err := f.s.Enroll(ctx, researcher, a.ID, application.EnrollInput{SubjectID: subject, GroupID: groupB.ID, EnrolledAt: t0}); !errors.Is(err, domain.ErrGroupNotInExperiment) {
		t.Fatalf("enrolled into a foreign group: %v", err)
	}
	if page, err := f.s.ListEnrollments(ctx, researcher, a.ID, application.EnrollmentQuery{}); err != nil || page.Total != 0 {
		t.Fatalf("failed enrollment not rolled back: %+v %v", page, err)
	}
	enrollment := f.enroll(a.ID, subject, "", t0)
	if _, err := f.s.Assign(ctx, researcher, a.ID, enrollment.ID, application.AssignInput{GroupID: groupB.ID, EffectiveFrom: t0}); !errors.Is(err, domain.ErrGroupNotInExperiment) {
		t.Fatalf("assigned a foreign group: %v", err)
	}
	if _, err := f.s.Enrollment(ctx, researcher, b.ID, enrollment.ID); !errors.Is(err, application.ErrEnrollmentNotFound) {
		t.Fatalf("enrollment read through another experiment: %v", err)
	}
	if _, err := f.s.Assign(ctx, researcher, b.ID, enrollment.ID, application.AssignInput{GroupID: groupB.ID, EffectiveFrom: t0}); !errors.Is(err, application.ErrEnrollmentNotFound) {
		t.Fatalf("assigned through another experiment: %v", err)
	}
	// The composite foreign keys hold even when the use cases are bypassed.
	for _, experimentID := range []string{a.ID, b.ID} {
		_, err := f.pool.Exec(ctx, "INSERT INTO misko.group_assignments (experiment_id, enrollment_id, group_id, valid_from) VALUES ($1, $2, $3, now())", experimentID, enrollment.ID, groupB.ID)
		if pgCode(err) != "23503" {
			t.Errorf("database accepted a cross-experiment assignment (experiment %s): %v", experimentID, err)
		}
	}
	if _, err := f.pool.Exec(ctx, "INSERT INTO misko.group_assignments (experiment_id, enrollment_id, group_id, valid_from) VALUES ($1, $2, $3, now())", a.ID, enrollment.ID, groupA.ID); err != nil {
		t.Fatalf("same-experiment control insert rejected: %v", err)
	}
	for _, id := range []string{"", "not-a-uuid"} {
		if _, err := f.s.Experiment(ctx, researcher, id); !errors.Is(err, application.ErrExperimentNotFound) {
			t.Errorf("Experiment(%q): %v", id, err)
		}
		if _, err := f.s.Phases(ctx, researcher, id); !errors.Is(err, application.ErrExperimentNotFound) {
			t.Errorf("Phases(%q): %v", id, err)
		}
	}
}

func TestAssignmentHistoryTargetsAndConstraints(t *testing.T) {
	f := newFixture(t)
	e := f.experiment("EXP-1", true)
	control, treated := f.group(e.ID, "Vehicle", "CONTROL", 2), f.group(e.ID, "Drug", "TREATMENT", 2)
	if _, err := f.s.CreatePhase(ctx, researcher, e.ID, application.PhaseInput{Name: "Baseline", Position: 1}); err != nil {
		t.Fatal(err)
	}
	s1, s2, s3 := f.subject(), f.subject(), f.subject()
	e1, e2, e3 := f.enroll(e.ID, s1, control.ID, t0), f.enroll(e.ID, s2, control.ID, t0), f.enroll(e.ID, s3, treated.ID, t0)
	if got := f.sizes(e.ID); got["Vehicle"] != [2]int{2, 2} || got["Drug"] != [2]int{2, 1} {
		t.Fatalf("initial target/actual: %v", got)
	}

	moved, err := f.s.Assign(ctx, researcher, e.ID, e2.ID, application.AssignInput{GroupID: treated.ID, EffectiveFrom: t0.Add(5 * day)})
	if err != nil || !moved.ValidFrom.Equal(t0.Add(5*day)) || moved.ValidTo != nil {
		t.Fatalf("crossover: %+v %v", moved, err)
	}
	if got := f.sizes(e.ID); got["Vehicle"] != [2]int{2, 1} || got["Drug"] != [2]int{2, 2} {
		t.Fatalf("after crossover: %v", got)
	}
	detail, err := f.s.Enrollment(ctx, researcher, e.ID, e2.ID)
	if err != nil || len(detail.Assignments) != 2 || detail.CurrentGroupID != treated.ID ||
		detail.Assignments[0].GroupID != control.ID || detail.Assignments[0].ValidTo == nil || !detail.Assignments[0].ValidTo.Equal(t0.Add(5*day)) {
		t.Fatalf("history: %+v %v", detail, err)
	}
	if page, err := f.s.ListEnrollments(ctx, researcher, e.ID, application.EnrollmentQuery{GroupID: treated.ID}); err != nil || page.Total != 2 {
		t.Fatalf("filter by current group: %+v %v", page, err)
	}
	if page, err := f.s.ListEnrollments(ctx, researcher, e.ID, application.EnrollmentQuery{SubjectID: s1}); err != nil || page.Total != 1 || page.Enrollments[0].CurrentGroupID != control.ID {
		t.Fatalf("filter by subject: %+v %v", page, err)
	}

	for name, tc := range map[string]struct {
		enrollment string
		group      string
		from       time.Time
		want       error
	}{
		"same group":        {e2.ID, treated.ID, t0.Add(6 * day), domain.ErrAlreadyInGroup},
		"before enrollment": {e1.ID, treated.ID, t0.Add(-time.Hour), domain.ErrAssignmentBeforeEnrollment},
		"rewrites history":  {e2.ID, control.ID, t0.Add(4 * day), domain.ErrAssignmentOutOfOrder},
	} {
		if _, err := f.s.Assign(ctx, researcher, e.ID, tc.enrollment, application.AssignInput{GroupID: tc.group, EffectiveFrom: tc.from}); !errors.Is(err, tc.want) {
			t.Errorf("%s: %v", name, err)
		}
	}

	// A future assignment is recorded but does not change today's counts.
	if _, err := f.s.Assign(ctx, researcher, e.ID, e1.ID, application.AssignInput{GroupID: treated.ID, EffectiveFrom: time.Now().Add(2 * day)}); err != nil {
		t.Fatal(err)
	}
	if got := f.sizes(e.ID); got["Vehicle"] != [2]int{2, 1} || got["Drug"] != [2]int{2, 2} {
		t.Fatalf("future assignment counted early: %v", got)
	}

	// Measuring the treated subject at a healthy baseline does not move it.
	if _, err := f.s.CreatePhase(ctx, researcher, e.ID, application.PhaseInput{Name: "Post-treatment", Position: 2}); err != nil {
		t.Fatal(err)
	}
	if detail, err := f.s.Enrollment(ctx, researcher, e.ID, e3.ID); err != nil || detail.CurrentGroupID != treated.ID || len(detail.Assignments) != 1 {
		t.Fatalf("treated subject after baseline phase: %+v %v", detail, err)
	}
	_, err = f.pool.Exec(ctx, "INSERT INTO misko.group_assignments (experiment_id, enrollment_id, group_id, valid_from) VALUES ($1, $2, $3, $4)", e.ID, e3.ID, control.ID, t0.Add(day))
	if pgCode(err) != "23P01" {
		t.Fatalf("database accepted an overlapping assignment: %v", err)
	}

	for name, err := range map[string]error{
		"duplicate enrollment": second(f.s.Enroll(ctx, researcher, e.ID, application.EnrollInput{SubjectID: s1, EnrolledAt: t0})),
		"unknown subject":      second(f.s.Enroll(ctx, researcher, e.ID, application.EnrollInput{SubjectID: "00000000-0000-0000-0000-000000000000", EnrolledAt: t0})),
		"unknown experiment":   second(f.s.Enroll(ctx, researcher, "00000000-0000-0000-0000-000000000000", application.EnrollInput{SubjectID: s1, EnrolledAt: t0})),
		"duplicate position":   second(f.s.CreatePhase(ctx, researcher, e.ID, application.PhaseInput{Name: "Other", Position: 1})),
		"duplicate phase name": second(f.s.CreatePhase(ctx, researcher, e.ID, application.PhaseInput{Name: "BASELINE", Position: 3})),
		"duplicate group name": second(f.s.CreateGroup(ctx, researcher, e.ID, application.GroupInput{Name: "drug", Role: "TREATMENT"})),
		"duplicate code":       second(f.s.CreateExperiment(ctx, researcher, application.ExperimentInput{Code: "exp-1", Title: "Again"})),
		"group in use":         f.s.DeleteGroup(ctx, researcher, e.ID, control.ID),
	} {
		want := map[string]error{
			"duplicate enrollment": application.ErrAlreadyEnrolled, "unknown subject": application.ErrSubjectNotFound,
			"unknown experiment": application.ErrExperimentNotFound, "duplicate position": application.ErrPhaseConflict,
			"duplicate phase name": application.ErrPhaseConflict, "duplicate group name": application.ErrGroupNameTaken,
			"duplicate code": application.ErrCodeTaken, "group in use": application.ErrGroupInUse,
		}[name]
		if !errors.Is(err, want) {
			t.Errorf("%s: err=%v want %v", name, err, want)
		}
	}
	unused := f.group(e.ID, "Spare", "TREATMENT", 0)
	if err := f.s.DeleteGroup(ctx, researcher, e.ID, unused.ID); err != nil {
		t.Fatalf("unused group: %v", err)
	}

	other := f.experiment("EXP-2", false)
	f.enroll(other.ID, s3, "", t0)
	if list, err := f.s.SubjectEnrollments(ctx, researcher, s3); err != nil || len(list) != 2 {
		t.Fatalf("subject in two experiments: %+v %v", list, err)
	}

	// The control requirement is part of the experiment configuration.
	requires := f.experiment("EXP-3", true)
	f.group(requires.ID, "Drug", "TREATMENT", 5)
	if detail, err := f.s.Experiment(ctx, researcher, requires.ID); err != nil || detail.ControlRequirementMet {
		t.Fatalf("control requirement without control group: %+v %v", detail, err)
	}
	f.group(requires.ID, "Vehicle", "CONTROL", 5)
	if detail, err := f.s.Experiment(ctx, researcher, requires.ID); err != nil || !detail.ControlRequirementMet {
		t.Fatalf("control requirement with control group: %+v %v", detail, err)
	}
}

func TestConcurrentEnrollmentAndAssignment(t *testing.T) {
	f := newFixture(t)
	e := f.experiment("EXP", false)
	control, treated, other := f.group(e.ID, "Vehicle", "CONTROL", 0), f.group(e.ID, "Drug", "TREATMENT", 0), f.group(e.ID, "Other", "TREATMENT", 0)
	subject := f.subject()

	results := race(8, func(int) error {
		_, err := f.s.Enroll(ctx, researcher, e.ID, application.EnrollInput{SubjectID: subject, GroupID: control.ID, EnrolledAt: t0})
		return err
	})
	if results[nil] != 1 || results[application.ErrAlreadyEnrolled] != 7 {
		t.Fatalf("concurrent enrollment: %v", results)
	}
	page, err := f.s.ListEnrollments(ctx, researcher, e.ID, application.EnrollmentQuery{SubjectID: subject})
	if err != nil || page.Total != 1 {
		t.Fatalf("enrollments: %+v %v", page, err)
	}
	enrollment := page.Enrollments[0].Enrollment

	from := t0.Add(3 * day)
	results = race(8, func(i int) error {
		group := []string{treated.ID, other.ID}[i%2]
		_, err := f.s.Assign(ctx, researcher, e.ID, enrollment.ID, application.AssignInput{GroupID: group, EffectiveFrom: from})
		return err
	})
	if results[nil] != 1 || results[domain.ErrAlreadyInGroup]+results[domain.ErrAssignmentOutOfOrder] != 7 {
		t.Fatalf("concurrent assignment: %v", results)
	}
	if detail, err := f.s.Enrollment(ctx, researcher, e.ID, enrollment.ID); err != nil || len(detail.Assignments) != 2 {
		t.Fatalf("history after concurrent assignment: %+v %v", detail, err)
	}

	// Writers that bypass the use case still cannot create overlapping periods.
	fresh := f.enroll(e.ID, f.subject(), "", t0)
	results = race(2, func(int) error {
		_, err := f.pool.Exec(ctx, "INSERT INTO misko.group_assignments (experiment_id, enrollment_id, group_id, valid_from) VALUES ($1, $2, $3, $4)", e.ID, fresh.ID, control.ID, t0)
		// A concurrent exclusion check may also surface as a detected deadlock;
		// either way the database rejects one of the inserts.
		if code := pgCode(err); code == "23P01" || code == "40P01" {
			return errOverlap
		}
		return err
	})
	if results[nil] != 1 || results[errOverlap] != 1 {
		t.Fatalf("raw concurrent overlap: %v", results)
	}
	var rows int
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM misko.group_assignments WHERE enrollment_id = $1", fresh.ID).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("assignments after raw race: %d %v", rows, err)
	}
}

// race starts n calls together and counts results by sentinel error.
func race(n int, call func(int) error) map[error]int {
	var mu sync.Mutex
	var wg sync.WaitGroup
	counts := map[error]int{}
	start := make(chan struct{})
	sentinels := []error{application.ErrAlreadyEnrolled, domain.ErrAlreadyInGroup, domain.ErrAssignmentOutOfOrder}
	for i := range n {
		wg.Go(func() {
			<-start
			err := call(i)
			for _, sentinel := range sentinels {
				if errors.Is(err, sentinel) {
					err = sentinel
				}
			}
			mu.Lock()
			defer mu.Unlock()
			counts[err]++
		})
	}
	close(start)
	wg.Wait()
	return counts
}

var errOverlap = errors.New("overlapping assignment rejected")

func second[T any](_ T, err error) error { return err }
