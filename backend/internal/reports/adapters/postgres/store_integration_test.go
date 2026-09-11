//go:build integration

package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
	"github.com/HappyHackingSpace/Misko/backend/internal/reports/adapters/postgres"
	"github.com/HappyHackingSpace/Misko/backend/internal/reports/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/reports/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	ctx    = context.Background()
	author = "01a00000-0000-7000-8000-000000000001"
	viewer = access.Actor{UserID: author, Role: access.Viewer}
	day    = time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
)

type querier interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// world writes a research history with SQL that passes every trigger of the
// owning domains: runs are published the way the analysis store does it.
type world struct {
	t      *testing.T
	db     querier
	worker string
	envA   string
	envB   string
}

func (w *world) id(sql string, args ...any) string {
	w.t.Helper()
	var out string
	if err := w.db.QueryRow(ctx, sql, args...).Scan(&out); err != nil {
		w.t.Fatalf("%v\n%s", err, sql)
	}
	return out
}

func (w *world) exec(sql string, args ...any) {
	w.t.Helper()
	if _, err := w.db.Exec(ctx, sql, args...); err != nil {
		w.t.Fatalf("%v\n%s", err, sql)
	}
}

func newWorld(t *testing.T, db querier) *world {
	w := &world{t: t, db: db}
	w.worker = w.id(`INSERT INTO misko.analysis_workers (name, model_version, token_sha256, created_by)
		VALUES ('tracker', 'tracker 1', decode(md5('a') || md5('b'), 'hex'), $1) RETURNING id`, author)
	revision := func(name string) string {
		env := w.id("INSERT INTO misko.environments (name, paradigm_key) VALUES ($1, 'OPEN_FIELD') RETURNING id", name)
		return w.id(`INSERT INTO misko.environment_revisions (environment_id, paradigm_key, number, paradigm_version, apparatus, created_by)
			VALUES ($1, 'OPEN_FIELD', 1, 1, '{"arena_width_cm": 50, "arena_height_cm": 50, "center_fraction": 0.5}', $2) RETURNING id`, env, author)
	}
	w.envA, w.envB = revision("Arena A"), revision("Arena B")
	return w
}

type experiment struct {
	id, version, control, treated, phase string
}

func (w *world) experiment(code string) experiment {
	e := experiment{id: w.id("INSERT INTO misko.experiments (code, title) VALUES ($1, 'Study') RETURNING id", code)}
	e.control = w.id("INSERT INTO misko.experiment_groups (experiment_id, name, role) VALUES ($1, 'Vehicle', 'CONTROL') RETURNING id", e.id)
	e.treated = w.id("INSERT INTO misko.experiment_groups (experiment_id, name, role) VALUES ($1, 'Drug', 'TREATMENT') RETURNING id", e.id)
	e.phase = w.id("INSERT INTO misko.experiment_phases (experiment_id, name, position) VALUES ($1, 'Baseline', 1) RETURNING id", e.id)
	protocol := w.id("INSERT INTO misko.protocols (experiment_id, name) VALUES ($1, 'Battery') RETURNING id", e.id)
	tx, ok := w.db.(*pgxpool.Pool)
	if !ok {
		w.t.Fatal("experiments are created on the pool")
	}
	if err := pgx.BeginFunc(ctx, tx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, "INSERT INTO misko.protocol_versions (experiment_id, protocol_id, number, step_count, created_by) VALUES ($1, $2, 1, 2, $3) RETURNING id",
			e.id, protocol, author).Scan(&e.version); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO misko.protocol_steps (protocol_version_id, position, paradigm_key, paradigm_version, environment_revision_id, trial_type, trials, inter_trial_interval_s, session)
			VALUES ($1, 1, 'OPEN_FIELD', 1, $2, 'STANDARD', 2, 0, '{}'), ($1, 2, 'OPEN_FIELD', 1, $3, 'STANDARD', 2, 0, '{}')`, e.version, w.envA, w.envB)
		return err
	}); err != nil {
		w.t.Fatal(err)
	}
	return e
}

func (w *world) subject(code string) string {
	return w.id("INSERT INTO misko.subjects (code, species, sex) VALUES ($1, 'MOUSE', 'FEMALE') RETURNING id", code)
}

type test struct {
	id, experiment, subject, enrollment, recording, video string
}

// test schedules an in-progress test on step 1 (arena A) or 2 (arena B) with one
// verified recording. group and phase may be empty.
func (w *world) test(e experiment, subject, group, phase string, step int, scheduled time.Time) test {
	t := test{experiment: e.id, subject: subject}
	if err := w.db.QueryRow(ctx, "SELECT id FROM misko.enrollments WHERE experiment_id = $1 AND subject_id = $2", e.id, subject).Scan(&t.enrollment); errors.Is(err, pgx.ErrNoRows) {
		t.enrollment = w.id("INSERT INTO misko.enrollments (experiment_id, subject_id, enrolled_at) VALUES ($1, $2, $3) RETURNING id", e.id, subject, day.AddDate(0, -1, 0))
	} else if err != nil {
		w.t.Fatal(err)
	}
	revision := map[int]string{1: w.envA, 2: w.envB}[step]
	t.id = w.id(`INSERT INTO misko.tests (experiment_id, enrollment_id, subject_id, group_id, phase_id, protocol_version_id, step_position, paradigm_key,
			paradigm_version, environment_revision_id, planned_trials, scheduled_at, created_by)
		VALUES ($1, $2, $3, NULLIF($4, '')::uuid, NULLIF($5, '')::uuid, $6, $7, 'OPEN_FIELD', 1, $8, 2, $9, $10) RETURNING id`,
		e.id, t.enrollment, subject, group, phase, e.version, step, revision, scheduled, author)
	w.exec("UPDATE misko.tests SET status = 'IN_PROGRESS', started_at = scheduled_at WHERE id = $1", t.id)
	t.video = w.id(`INSERT INTO misko.video_assets (kind, bucket, object_name, content_type, size_bytes, crc32c, status, generation, verified_at, created_by)
		VALUES ('ORIGINAL', 'misko-test', 'tests/' || $1::text, 'video/mp4', 100, 1, 'VERIFIED', 42, now(), $2) RETURNING id`, t.id, author)
	t.recording = w.id("INSERT INTO misko.test_recordings (experiment_id, test_id, video_asset_id, created_by) VALUES ($1, $2, $3, $4) RETURNING id",
		e.id, t.id, t.video, author)
	return t
}

func (w *world) trial(t test, number int) string {
	return w.id("INSERT INTO misko.trials (test_id, number, repetition, attempt, started_at, recorded_by) VALUES ($1, $2, $2, 1, $3, $4) RETURNING id",
		t.id, number, day, author)
}

type event struct {
	kind, trial string
}

// run publishes a succeeded run with the given metrics; a nil value is a
// missing result.
func (w *world) run(t test, engine int, finished time.Time, metrics map[string]*float64, events ...event) string {
	trigger := "MANUAL"
	var automatic bool
	if err := w.db.QueryRow(ctx, "SELECT NOT EXISTS (SELECT 1 FROM misko.analysis_runs WHERE recording_id = $1 AND trigger = 'AUTOMATIC')", t.recording).Scan(&automatic); err != nil {
		w.t.Fatal(err)
	}
	if automatic {
		trigger = "AUTOMATIC"
	}
	run := w.id(`INSERT INTO misko.analysis_runs (experiment_id, test_id, recording_id, source_asset_id, source_generation, source_crc32c, clip_start_us,
			paradigm_key, paradigm_version, metric_engine_version, result_schema_version, environment_revision_id, protocol_version_id, parameters,
			trigger, status, attempt, max_attempts, worker_id, lease_expires_at, available_at)
		SELECT t.experiment_id, t.id, $2, $3, 42, 1, 0, t.paradigm_key, t.paradigm_version, $4, 1, t.environment_revision_id, t.protocol_version_id, '{}',
			$5, 'RUNNING', 1, 3, $6, now() + interval '1 hour', now()
		FROM misko.tests t WHERE t.id = $1 RETURNING id`, t.id, t.recording, t.video, engine, trigger, w.worker)
	object := "runs/" + run + "/attempts/1/overlay.mp4"
	w.exec("INSERT INTO misko.analysis_output_uploads (run_id, attempt, object_name, kind, content_type, size_bytes, crc32c) VALUES ($1, 1, $2, 'ANALYZED_VIDEO', 'video/mp4', 10, 1)", run, object)
	analyzed := w.id(`INSERT INTO misko.video_assets (kind, bucket, object_name, content_type, size_bytes, crc32c, status, generation, verified_at, created_by)
		VALUES ('ANALYZED', 'misko-test', $1, 'video/mp4', 10, 1, 'VERIFIED', 7, now(), $2) RETURNING id`, object, w.worker)
	w.exec(`INSERT INTO misko.analysis_artifacts (run_id, attempt, object_name, kind, bucket, generation, size_bytes, crc32c, content_type, video_asset_id)
		VALUES ($1, 1, $2, 'ANALYZED_VIDEO', 'misko-test', 7, 10, 1, 'video/mp4', $3)`, run, object, analyzed)
	w.exec(`INSERT INTO misko.analysis_video_pairs (run_id, attempt, source_asset_id, source_generation, analyzed_asset_id, source_offset_us, output_offset_us, time_mapping_version)
		VALUES ($1, 1, $2, 42, $3, 0, 0, 'identity-v1')`, run, t.video, analyzed)
	for key, value := range metrics {
		reason := ""
		if value == nil {
			reason = "NOT_OBSERVED"
		}
		w.exec("INSERT INTO misko.metric_results (run_id, attempt, metric_key, unit, value, missing_reason) VALUES ($1, 1, $2, 'cm', $3, NULLIF($4, ''))", run, key, value, reason)
	}
	for i, e := range events {
		w.exec(`INSERT INTO misko.analysis_events (run_id, attempt, test_id, event_type, kind, start_us, end_us, confidence, trial_id)
			VALUES ($1, 1, $2, $3, 'INTERVAL', $4::bigint, $4::bigint + 1000, 0.9, NULLIF($5, '')::uuid)`, run, t.id, e.kind, i*10_000, e.trial)
	}
	w.exec("UPDATE misko.analysis_runs SET status = 'SUCCEEDED', lease_expires_at = NULL, model_version = 'tracker 1', finished_at = $2 WHERE id = $1", run, finished)
	return run
}

func cm(v float64) *float64 { return &v }

type scenario struct {
	pool                           *pgxpool.Pool
	e1, e2                         experiment
	s1, s2, s3, s4, s5             string
	t1, t2, t3, t4, t5, t6, t7, t8 test
	oldRun, newRun, t5engine2      string
	diseaseModel, substance, trial string
}

// newScenario: in E1, S1 (control) has T1 (analyzed twice: 100 then 110) and T2
// (130); S2 has T3 in arena B (80, control) and T6 without a group (70); S3
// (treated, disease model) has a missing result in T4; S4 (treated, substance)
// has T5 with engine 1 (50) and engine 2 (55) and T8 in E2 (1). S5 is in E2
// only (999).
func newScenario(t *testing.T) *scenario {
	pool := pgtest.Database(t)
	w := newWorld(t, pool)
	s := &scenario{pool: pool, e1: w.experiment("E1"), e2: w.experiment("E2")}
	s.s1, s.s2, s.s3, s.s4, s.s5 = w.subject("S1"), w.subject("S2"), w.subject("S3"), w.subject("S4"), w.subject("S5")
	s.t1 = w.test(s.e1, s.s1, s.e1.control, s.e1.phase, 1, day)
	s.t2 = w.test(s.e1, s.s1, s.e1.control, s.e1.phase, 1, day.AddDate(0, 0, 1))
	s.t3 = w.test(s.e1, s.s2, s.e1.control, s.e1.phase, 2, day)
	s.t4 = w.test(s.e1, s.s3, s.e1.treated, s.e1.phase, 1, day)
	s.t5 = w.test(s.e1, s.s4, s.e1.treated, s.e1.phase, 1, day)
	s.t6 = w.test(s.e1, s.s2, "", "", 1, day.AddDate(0, 0, 2))
	s.t7 = w.test(s.e2, s.s5, s.e2.control, "", 1, day)
	s.trial = w.trial(s.t3, 1)

	at := func(hour int) time.Time { return day.Add(time.Duration(hour) * time.Hour) }
	s.oldRun = w.run(s.t1, 1, at(10), map[string]*float64{"distance_cm": cm(100), "immobile_time_s": cm(3)}, event{kind: "in_center"})
	s.newRun = w.run(s.t1, 1, at(11), map[string]*float64{"distance_cm": cm(110), "immobile_time_s": cm(4)}, event{kind: "in_center"})
	w.run(s.t2, 1, at(10), map[string]*float64{"distance_cm": cm(130)})
	w.run(s.t3, 1, at(10), map[string]*float64{"distance_cm": cm(80)}, event{kind: "in_center", trial: s.trial}, event{kind: "immobile"})
	w.run(s.t4, 1, at(10), map[string]*float64{"distance_cm": nil})
	w.run(s.t5, 1, at(10), map[string]*float64{"distance_cm": cm(50)})
	s.t5engine2 = w.run(s.t5, 2, at(12), map[string]*float64{"distance_cm": cm(55)})
	w.run(s.t6, 1, at(10), map[string]*float64{"distance_cm": cm(70)})
	w.run(s.t7, 1, at(10), map[string]*float64{"distance_cm": cm(999)})

	s.diseaseModel = w.id("INSERT INTO misko.disease_models (name) VALUES ('Model') RETURNING id")
	w.exec(`INSERT INTO misko.subject_conditions (subject_id, disease_model_id, disease_model_name, status, observed_at, recorded_by)
		VALUES ($1, $2, 'Model', 'INDUCED', $3, $4)`, s.s3, s.diseaseModel, day.AddDate(0, 0, -7), author)
	s.substance = w.id("INSERT INTO misko.substances (name) VALUES ('Drug X') RETURNING id")
	w.exec(`INSERT INTO misko.administrations (experiment_id, enrollment_id, subject_id, substance_id, substance_name, amount_micro, unit, route, administered_at, recorded_by)
		VALUES ($1, $2, $3, $4, 'Drug X', 1000000, 'mg', 'ORAL', $5, $6)`, s.e1.id, s.t5.enrollment, s.s4, s.substance, day.AddDate(0, 0, -1), author)
	// S4 is also tested in E2, where it never received the substance.
	s.t8 = w.test(s.e2, s.s4, "", "", 1, day)
	w.run(s.t8, 1, at(10), map[string]*float64{"distance_cm": cm(1)})
	return s
}

func metrics(t *testing.T, s *application.Service, q application.Query) []domain.MetricRow {
	t.Helper()
	q.PageSize = 100
	page, err := s.Metrics(ctx, viewer, q)
	if err != nil {
		t.Fatalf("%+v: %v", q, err)
	}
	if page.Total != len(page.Rows) {
		t.Fatalf("%+v: total %d for %d rows", q, page.Total, len(page.Rows))
	}
	return page.Rows
}

func TestFiltersSelectionAndProvenance(t *testing.T) {
	sc := newScenario(t)
	s := application.New(postgres.NewStore(sc.pool), 1000)
	distance := application.Query{ExperimentID: sc.e1.id, MetricKey: "distance_cm"}

	rows := metrics(t, s, distance)
	if len(rows) != 7 {
		t.Fatalf("latest rows: %d", len(rows))
	}
	for _, r := range rows {
		if r.RunID == sc.oldRun || !r.Latest {
			t.Fatalf("a superseded run was counted: %+v", r.Provenance)
		}
		if r.TestID == sc.t4.id && (r.Value != nil || r.MissingReason != "NOT_OBSERVED") {
			t.Fatalf("a missing result became a value: %+v", r)
		}
	}
	all := metrics(t, s, application.Query{ExperimentID: sc.e1.id, MetricKey: "distance_cm", Selection: "all"})
	old := metrics(t, s, application.Query{RunID: sc.oldRun, MetricKey: "distance_cm"})
	if len(all) != 8 || len(old) != 1 || old[0].Latest || *old[0].Value != 100 {
		t.Fatalf("all runs %d, explicit old run %+v", len(all), old)
	}

	count := func(q application.Query) int { return len(metrics(t, s, q)) }
	for name, tc := range map[string]struct {
		q    application.Query
		want int
	}{
		"subject":             {application.Query{MetricKey: "distance_cm", SubjectID: sc.s1}, 2},
		"group":               {application.Query{MetricKey: "distance_cm", GroupID: sc.e1.treated}, 3},
		"phase":               {application.Query{MetricKey: "distance_cm", PhaseID: sc.e1.phase}, 6},
		"test":                {application.Query{MetricKey: "distance_cm", TestID: sc.t2.id}, 1},
		"environment":         {application.Query{MetricKey: "distance_cm", EnvironmentID: environment(t, sc.pool, sc.t3.id)}, 1},
		"video":               {application.Query{MetricKey: "distance_cm", VideoID: sc.t3.video}, 1},
		"disease model":       {application.Query{MetricKey: "distance_cm", DiseaseModelID: sc.diseaseModel}, 1},
		"substance":           {application.Query{MetricKey: "distance_cm", SubstanceID: sc.substance}, 2},
		"substance elsewhere": {application.Query{MetricKey: "distance_cm", SubstanceID: sc.substance, ExperimentID: sc.e2.id}, 0},
		"paradigm":            {application.Query{MetricKey: "distance_cm", ParadigmKey: "OPEN_FIELD", ParadigmVersion: 1}, 9},
		"other paradigm":      {application.Query{MetricKey: "distance_cm", ParadigmKey: "LIGHT_DARK"}, 0},
		"engine version":      {application.Query{MetricKey: "distance_cm", MetricEngineVersion: 2}, 1},
		"other experiment":    {application.Query{MetricKey: "distance_cm", ExperimentID: sc.e2.id}, 2},
		"every metric of E1":  {application.Query{ExperimentID: sc.e1.id}, 8},
		"unknown id":          {application.Query{SubjectID: "01a00000-0000-7000-8000-00000000ffff"}, 0},
	} {
		if got := count(tc.q); got != tc.want {
			t.Errorf("%s: %d rows, want %d", name, got, tc.want)
		}
	}

	var t3 domain.MetricRow
	for _, r := range rows {
		if r.TestID == sc.t3.id {
			t3 = r
		}
	}
	if t3.SubjectCode != "S2" || t3.ExperimentCode != "E1" || t3.GroupName != "Vehicle" || t3.GroupRole != "CONTROL" || t3.PhaseName != "Baseline" ||
		t3.EnvironmentName != "Arena B" || t3.EnvironmentRevision != 1 || t3.TrialCount != 1 || t3.RecordingID != sc.t3.recording ||
		t3.SourceVideoID != sc.t3.video || t3.Trigger != "AUTOMATIC" || t3.ModelVersion != "tracker 1" || t3.Unit != "cm" ||
		!t3.RunFinishedAt.Equal(day.Add(10*time.Hour)) || !t3.ScheduledAt.Equal(day) || t3.TestStatus != "IN_PROGRESS" {
		t.Fatalf("provenance: %+v", t3)
	}

	events, err := s.Events(ctx, viewer, application.Query{ExperimentID: sc.e1.id, PageSize: 100})
	if err != nil || events.Total != 3 {
		t.Fatalf("latest events: %+v %v", events, err)
	}
	trial, err := s.Events(ctx, viewer, application.Query{TestID: sc.t3.id, EventType: "in_center"})
	if err != nil || len(trial.Rows) != 1 || trial.Rows[0].TrialID != sc.trial || trial.Rows[0].TrialNumber != 1 || trial.Rows[0].TrialRepetition != 1 || trial.Rows[0].TrialAttempt != 1 {
		t.Fatalf("event trial provenance: %+v %v", trial.Rows, err)
	}
}

func environment(t *testing.T, pool *pgxpool.Pool, testID string) string {
	var id string
	if err := pool.QueryRow(ctx, "SELECT r.environment_id FROM misko.tests t JOIN misko.environment_revisions r ON r.id = t.environment_revision_id WHERE t.id = $1", testID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestPagingIsStableForEverySort(t *testing.T) {
	sc := newScenario(t)
	s := application.New(postgres.NewStore(sc.pool), 1000)
	for _, sort := range []string{"scheduledAt", "subjectCode", "experimentCode", "metricKey", "value", "runFinishedAt"} {
		for _, order := range []string{"asc", "desc"} {
			q := application.Query{Selection: "all", Sort: sort, Order: order}
			want := metrics(t, s, q)
			var got []string
			for page := 1; ; page++ {
				q.Page, q.PageSize = page, 3
				p, err := s.Metrics(ctx, viewer, q)
				if err != nil {
					t.Fatal(err)
				}
				if p.Total != len(want) {
					t.Fatalf("%s %s: total %d, want %d", sort, order, p.Total, len(want))
				}
				if len(p.Rows) == 0 {
					break
				}
				for _, r := range p.Rows {
					got = append(got, r.RunID+"/"+r.MetricKey)
				}
			}
			var expected []string
			for _, r := range want {
				expected = append(expected, r.RunID+"/"+r.MetricKey)
			}
			if !slices.Equal(got, expected) {
				t.Fatalf("%s %s: pages %v, single query %v", sort, order, got, expected)
			}
		}
	}
	bySubject := metrics(t, s, application.Query{Sort: "subjectCode", Order: "desc", MetricKey: "distance_cm"})
	if bySubject[0].SubjectCode != "S5" || bySubject[len(bySubject)-1].SubjectCode != "S1" {
		t.Fatalf("subject order: %s ... %s", bySubject[0].SubjectCode, bySubject[len(bySubject)-1].SubjectCode)
	}
	byValue := metrics(t, s, application.Query{Sort: "value", ExperimentID: sc.e1.id, MetricKey: "distance_cm"})
	if *byValue[0].Value != 50 || byValue[len(byValue)-1].Value != nil {
		t.Fatalf("missing values sort last: %v ... %v", byValue[0].Value, byValue[len(byValue)-1].Value)
	}
}

func TestGroupComparisonCountsAnimalsOnceAndRefusesMixedVersions(t *testing.T) {
	sc := newScenario(t)
	s := application.New(postgres.NewStore(sc.pool), 1000)
	q := application.Query{ExperimentID: sc.e1.id, MetricKey: "distance_cm"}
	if _, err := s.Summary(ctx, viewer, q); !errors.Is(err, domain.ErrIncompatibleVersions) {
		t.Fatalf("engine 1 and engine 2 results were compared: %v", err)
	}
	q.MetricEngineVersion = 1
	summary, err := s.Summary(ctx, viewer, q)
	if err != nil || len(summary.Groups) != 3 {
		t.Fatalf("summary: %+v %v", summary, err)
	}
	control, treated, ungrouped := summary.Groups[0], summary.Groups[1], summary.Groups[2]
	// S1 is one animal with two tests (110 after reanalysis, 130): it contributes 120.
	if control.GroupID != sc.e1.control || control.Tests != 3 || control.Subjects != 2 || *control.Mean != 100 {
		t.Fatalf("control: %+v", control)
	}
	if treated.Tests != 2 || treated.Subjects != 1 || treated.MissingSubjects != 1 || treated.MissingReasons["NOT_OBSERVED"] != 1 || *treated.Mean != 50 {
		t.Fatalf("treated: %+v", treated)
	}
	if ungrouped.GroupID != "" || ungrouped.Subjects != 1 || *ungrouped.Mean != 70 {
		t.Fatalf("ungrouped: %+v", ungrouped)
	}
	q.MetricEngineVersion = 2
	if summary, err := s.Summary(ctx, viewer, q); err != nil || len(summary.Groups) != 1 || *summary.Groups[0].Mean != 55 || summary.Version.MetricEngineVersion != 2 {
		t.Fatalf("engine 2: %+v %v", summary, err)
	}
	if _, err := application.New(postgres.NewStore(sc.pool), 3).ExportMetrics(ctx, viewer, application.Query{ExperimentID: sc.e1.id}); !errors.Is(err, application.ErrExportTooLarge) {
		t.Fatalf("export limit: %v", err)
	}

	for _, sql := range []string{
		"UPDATE misko.report_runs SET trigger = 'MANUAL'",
		"DELETE FROM misko.report_runs",
		"UPDATE misko.report_tests SET subject_code = 'X'",
		"DELETE FROM misko.report_tests",
	} {
		if _, err := sc.pool.Exec(ctx, sql); err == nil {
			t.Errorf("a report view accepted a write: %s", sql)
		}
	}
}

// tracer records the SQL and arguments the store sends, so the test can ask
// PostgreSQL for the plan of exactly those statements.
type tracer struct {
	mu    sync.Mutex
	calls []pgx.TraceQueryStartData
}

func (r *tracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, data)
	return ctx
}

func (r *tracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

type planNode struct {
	NodeType  string     `json:"Node Type"`
	Relation  string     `json:"Relation Name"`
	Index     string     `json:"Index Name"`
	IndexCond string     `json:"Index Cond"`
	Plans     []planNode `json:"Plans"`
}

func walk(n planNode, visit func(planNode)) {
	visit(n)
	for _, child := range n.Plans {
		walk(child, visit)
	}
}

// TestReportPlansUseIndexes generates 50 experiments, 2,000 subjects, 10,000
// tests, trials and recordings, 10,400 published runs (400 tests reanalyzed),
// 31,200 metric results and 20,800 events with set-based SQL through the same
// triggers, then checks the plans of the filtered report statements.
func TestReportPlansUseIndexes(t *testing.T) {
	pool := pgtest.Database(t)
	seedPlanData(t, pool)
	cfg := pool.Config()
	trace := &tracer{}
	cfg.ConnConfig.Tracer = trace
	traced, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer traced.Close()
	s := application.New(postgres.NewStore(traced), 100_000)

	id := func(sql string) string {
		var out string
		if err := pool.QueryRow(ctx, sql).Scan(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	experimentID := id("SELECT md5('e7')::uuid::text")
	scenarios := map[string]func() error{
		"metrics by experiment": func() error {
			_, err := s.Metrics(ctx, viewer, application.Query{ExperimentID: experimentID, MetricKey: "distance_cm"})
			return err
		},
		"metrics by subject": func() error {
			_, err := s.Metrics(ctx, viewer, application.Query{SubjectID: id("SELECT md5('s77')::uuid::text")})
			return err
		},
		"metrics by group": func() error {
			_, err := s.Metrics(ctx, viewer, application.Query{GroupID: id("SELECT md5('g7-1')::uuid::text")})
			return err
		},
		"metrics by video": func() error {
			_, err := s.Metrics(ctx, viewer, application.Query{VideoID: id("SELECT md5('a77-3')::uuid::text")})
			return err
		},
		"metrics by run": func() error {
			_, err := s.Metrics(ctx, viewer, application.Query{RunID: id("SELECT id::text FROM misko.analysis_runs ORDER BY id LIMIT 1 OFFSET 500")})
			return err
		},
		"events by experiment": func() error {
			_, err := s.Events(ctx, viewer, application.Query{ExperimentID: experimentID, Selection: "all"})
			return err
		},
		"summary of an experiment": func() error {
			_, err := s.Summary(ctx, viewer, application.Query{ExperimentID: experimentID, MetricKey: "distance_cm"})
			return err
		},
	}
	for name, call := range scenarios {
		trace.mu.Lock()
		trace.calls = nil
		trace.mu.Unlock()
		if err := call(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, q := range trace.calls {
			statement := strings.Fields(strings.TrimPrefix(q.SQL, "-- name: "))[0]
			// The cost-based plan on this dataset is logged as evidence. With
			// sequential scans disabled, a remaining full scan means no index
			// condition can serve the statement, which fails the test.
			indexes, full, _ := explain(t, pool, false, q)
			t.Logf("%s (%s): indexes %v, full scans %v", name, statement, indexes, full)
			if name == "metrics by experiment" && !slices.Contains(indexes, "analysis_runs_report_idx") {
				t.Errorf("%s (%s): the latest-run check does not use analysis_runs_report_idx: %v", name, statement, indexes)
			}
			if _, full, raw := explain(t, pool, true, q); len(full) > 0 {
				t.Errorf("%s (%s): no index condition for %v\n%s", name, statement, full, raw)
			}
		}
	}
}

// large lists tables that grow with every test and analysis. The planner may
// hash a small lookup table such as subjects, joined on its primary key.
var large = []string{"analysis_runs", "metric_results", "analysis_events", "tests", "test_recordings", "video_assets", "trials"}

// explain returns the indexes and the full scans of large tables in the plan of
// a recorded statement. An index scan without an index condition reads the whole
// index, so it counts as a full scan.
func explain(t *testing.T, pool *pgxpool.Pool, noSeqScan bool, q pgx.TraceQueryStartData) (indexes, full []string, raw []byte) {
	t.Helper()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, fmt.Sprintf("SET enable_seqscan = %t", !noSeqScan)); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = conn.Exec(ctx, "RESET enable_seqscan") }()
	if err := conn.QueryRow(ctx, "EXPLAIN (FORMAT JSON) "+q.SQL, q.Args...).Scan(&raw); err != nil {
		t.Fatalf("explain: %v", err)
	}
	var plans []struct {
		Plan planNode `json:"Plan"`
	}
	if err := json.Unmarshal(raw, &plans); err != nil {
		t.Fatal(err)
	}
	walk(plans[0].Plan, func(n planNode) {
		if n.Index != "" {
			indexes = append(indexes, n.Index)
		}
		indexScan := n.NodeType == "Index Scan" || n.NodeType == "Index Only Scan" || n.NodeType == "Bitmap Index Scan"
		if n.NodeType == "Seq Scan" && slices.Contains(large, n.Relation) {
			full = append(full, "Seq Scan on "+n.Relation)
		}
		// Bitmap index scans do not name their table, so any unconditioned one counts.
		if indexScan && n.IndexCond == "" && (n.Relation == "" || slices.Contains(large, n.Relation)) {
			full = append(full, n.NodeType+" using "+n.Index)
		}
	})
	return indexes, full, raw
}

func seedPlanData(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	statements := []string{
		`INSERT INTO misko.analysis_workers (id, name, model_version, token_sha256, created_by)
			VALUES (md5('worker')::uuid, 'planner', 'tracker 1', decode(md5('c') || md5('d'), 'hex'), md5('author')::uuid)`,
		`INSERT INTO misko.environments (id, name, paradigm_key) VALUES (md5('env')::uuid, 'Plan arena', 'OPEN_FIELD')`,
		`INSERT INTO misko.environment_revisions (id, environment_id, paradigm_key, number, paradigm_version, apparatus, created_by)
			VALUES (md5('rev')::uuid, md5('env')::uuid, 'OPEN_FIELD', 1, 1, '{}', md5('author')::uuid)`,
		`INSERT INTO misko.experiments (id, code, title) SELECT md5('e' || e)::uuid, 'PE' || e, 'Plan' FROM generate_series(1, 50) e`,
		`INSERT INTO misko.experiment_groups (id, experiment_id, name, role)
			SELECT md5('g' || e || '-' || g)::uuid, md5('e' || e)::uuid, 'G' || g, CASE g WHEN 1 THEN 'CONTROL' ELSE 'TREATMENT' END
			FROM generate_series(1, 50) e, generate_series(1, 2) g`,
		`INSERT INTO misko.protocols (id, experiment_id, name) SELECT md5('p' || e)::uuid, md5('e' || e)::uuid, 'Plan' FROM generate_series(1, 50) e`,
		`INSERT INTO misko.protocol_versions (id, experiment_id, protocol_id, number, step_count, created_by)
			SELECT md5('v' || e)::uuid, md5('e' || e)::uuid, md5('p' || e)::uuid, 1, 1, md5('author')::uuid FROM generate_series(1, 50) e`,
		`INSERT INTO misko.protocol_steps (protocol_version_id, position, paradigm_key, paradigm_version, environment_revision_id, trial_type, trials, inter_trial_interval_s, session)
			SELECT md5('v' || e)::uuid, 1, 'OPEN_FIELD', 1, md5('rev')::uuid, 'STANDARD', 1, 0, '{}' FROM generate_series(1, 50) e`,
		`INSERT INTO misko.subjects (id, code, species, sex) SELECT md5('s' || s)::uuid, 'PS' || s, 'MOUSE', 'MALE' FROM generate_series(1, 2000) s`,
		`INSERT INTO misko.enrollments (id, experiment_id, subject_id, enrolled_at)
			SELECT md5('n' || s)::uuid, md5('e' || ((s - 1) / 40 + 1))::uuid, md5('s' || s)::uuid, now() - interval '60 days' FROM generate_series(1, 2000) s`,
		`INSERT INTO misko.tests (id, experiment_id, enrollment_id, subject_id, group_id, protocol_version_id, step_position, paradigm_key, paradigm_version,
				environment_revision_id, planned_trials, scheduled_at, created_by)
			SELECT md5('t' || s || '-' || k)::uuid, md5('e' || ((s - 1) / 40 + 1))::uuid, md5('n' || s)::uuid, md5('s' || s)::uuid,
				md5('g' || ((s - 1) / 40 + 1) || '-' || (s % 2 + 1))::uuid, md5('v' || ((s - 1) / 40 + 1))::uuid, 1, 'OPEN_FIELD', 1, md5('rev')::uuid, 1,
				now() - make_interval(days => k), md5('author')::uuid
			FROM generate_series(1, 2000) s, generate_series(1, 5) k`,
		`INSERT INTO misko.video_assets (id, kind, bucket, object_name, content_type, size_bytes, crc32c, status, generation, verified_at, created_by)
			SELECT md5('a' || s || '-' || k)::uuid, 'ORIGINAL', 'misko-plan', 'plan/' || s || '-' || k, 'video/mp4', 100, 1, 'VERIFIED', 42, now(), md5('author')::uuid
			FROM generate_series(1, 2000) s, generate_series(1, 5) k`,
		`INSERT INTO misko.test_recordings (id, experiment_id, test_id, video_asset_id, created_by)
			SELECT md5('r' || s || '-' || k)::uuid, md5('e' || ((s - 1) / 40 + 1))::uuid, md5('t' || s || '-' || k)::uuid, md5('a' || s || '-' || k)::uuid, md5('author')::uuid
			FROM generate_series(1, 2000) s, generate_series(1, 5) k`,
		`UPDATE misko.tests SET status = 'IN_PROGRESS', started_at = scheduled_at`,
		`INSERT INTO misko.trials (test_id, number, repetition, attempt, started_at, recorded_by)
			SELECT id, 1, 1, 1, scheduled_at, md5('author')::uuid FROM misko.tests`,
		`INSERT INTO misko.analysis_runs (experiment_id, test_id, recording_id, source_asset_id, source_generation, source_crc32c, clip_start_us, paradigm_key,
				paradigm_version, metric_engine_version, result_schema_version, environment_revision_id, protocol_version_id, parameters, trigger, status,
				attempt, max_attempts, worker_id, lease_expires_at, available_at)
			SELECT md5('e' || ((s - 1) / 40 + 1))::uuid, md5('t' || s || '-' || k)::uuid, md5('r' || s || '-' || k)::uuid, md5('a' || s || '-' || k)::uuid,
				42, 1, 0, 'OPEN_FIELD', 1, 1, 1, md5('rev')::uuid, md5('v' || ((s - 1) / 40 + 1))::uuid, '{}', CASE n WHEN 1 THEN 'AUTOMATIC' ELSE 'MANUAL' END,
				'RUNNING', 1, 3, md5('worker')::uuid, now() + interval '1 hour', now() + make_interval(mins => n)
			FROM generate_series(1, 2000) s, generate_series(1, 5) k, generate_series(1, CASE WHEN s % 5 = 0 AND k = 1 THEN 2 ELSE 1 END) n`,
		`INSERT INTO misko.analysis_output_uploads (run_id, attempt, object_name, kind, content_type, size_bytes, crc32c)
			SELECT id, 1, 'runs/' || id || '/attempts/1/overlay.mp4', 'ANALYZED_VIDEO', 'video/mp4', 10, 1 FROM misko.analysis_runs`,
		`INSERT INTO misko.video_assets (id, kind, bucket, object_name, content_type, size_bytes, crc32c, status, generation, verified_at, created_by)
			SELECT md5('overlay' || id)::uuid, 'ANALYZED', 'misko-plan', 'runs/' || id || '/attempts/1/overlay.mp4', 'video/mp4', 10, 1, 'VERIFIED', 7, now(), md5('worker')::uuid
			FROM misko.analysis_runs`,
		`INSERT INTO misko.analysis_artifacts (run_id, attempt, object_name, kind, bucket, generation, size_bytes, crc32c, content_type, video_asset_id)
			SELECT id, 1, 'runs/' || id || '/attempts/1/overlay.mp4', 'ANALYZED_VIDEO', 'misko-plan', 7, 10, 1, 'video/mp4', md5('overlay' || id)::uuid FROM misko.analysis_runs`,
		`INSERT INTO misko.analysis_video_pairs (run_id, attempt, source_asset_id, source_generation, analyzed_asset_id, source_offset_us, output_offset_us, time_mapping_version)
			SELECT id, 1, source_asset_id, 42, md5('overlay' || id)::uuid, 0, 0, 'identity-v1' FROM misko.analysis_runs`,
		`INSERT INTO misko.metric_results (run_id, attempt, metric_key, unit, value)
			SELECT r.id, 1, m.key, 'cm', (hashtext(r.id::text || m.key) % 1000)::float8 FROM misko.analysis_runs r,
				(VALUES ('distance_cm'), ('immobile_time_s'), ('center_time_s')) m(key)`,
		`INSERT INTO misko.analysis_events (run_id, attempt, test_id, event_type, kind, start_us, end_us, confidence)
			SELECT r.id, 1, r.test_id, e.type, 'INTERVAL', 0, 1000, 0.9 FROM misko.analysis_runs r, (VALUES ('in_center'), ('immobile')) e(type)`,
		`UPDATE misko.analysis_runs SET status = 'SUCCEEDED', lease_expires_at = NULL, model_version = 'tracker 1', finished_at = available_at`,
	}
	start := time.Now()
	if err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		for _, sql := range statements {
			if _, err := tx.Exec(ctx, sql); err != nil {
				return fmt.Errorf("%w\n%s", err, sql)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "ANALYZE"); err != nil {
		t.Fatal(err)
	}
	var runs, metrics int
	if err := pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM misko.report_runs), (SELECT count(*) FROM misko.metric_results)").Scan(&runs, &metrics); err != nil {
		t.Fatal(err)
	}
	if runs != 10_400 || metrics != 31_200 {
		t.Fatalf("plan dataset: %d runs, %d metrics", runs, metrics)
	}
	t.Logf("seeded %d runs and %d metric results in %s", runs, metrics, time.Since(start).Round(time.Millisecond))
}
