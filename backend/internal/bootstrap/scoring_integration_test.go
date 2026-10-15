//go:build integration

package bootstrap

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/jackc/pgx/v5"
)

// scoringStudy adds an observer-scored protocol (rotarod, then novel object
// recognition) with day 14 endpoints and their arenas to a study world.
type scoringStudy struct {
	*studyWorld
	version              string
	rotarod, novel       []string
	latency, recognition string
}

func newScoringStudy(t *testing.T, w *studyWorld) *scoringStudy {
	t.Helper()
	s := &scoringStudy{studyWorld: w}
	protocol := w.id("INSERT INTO misko.protocols (experiment_id, name) VALUES ($1, 'Motor and memory') RETURNING id", w.exp)
	if err := pgx.BeginFunc(context.Background(), w.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(context.Background(), `INSERT INTO misko.protocol_versions (experiment_id, protocol_id, number, step_count, created_by)
			VALUES ($1, $2, 1, 2, $3) RETURNING id`, w.exp, protocol, studyAuthor).Scan(&s.version); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `INSERT INTO misko.protocol_steps (protocol_version_id, position, paradigm_key, paradigm_version, trial_type, trials, session)
			VALUES ($1, 1, 'ROTAROD', 1, 'FIXED_SPEED', 1, '{"accelerating": 0, "start_rpm": 10, "max_trial_duration_s": 60}'),
			       ($1, 2, 'NOVEL_OBJECT', 1, 'TEST', 1, '{}')`, s.version)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	s.latency = w.id(`INSERT INTO misko.endpoints (experiment_id, timepoint_id, paradigm_key, metric_key, kind, direction)
		VALUES ($1, $2, 'ROTAROD', 'latency_to_fall_s', 'SECONDARY', 'HIGHER_IS_BETTER') RETURNING id`, w.exp, w.timepoints["Day 14"])
	s.recognition = w.id(`INSERT INTO misko.endpoints (experiment_id, timepoint_id, paradigm_key, metric_key, kind, direction)
		VALUES ($1, $2, 'NOVEL_OBJECT', 'discrimination_index', 'SECONDARY', 'HIGHER_IS_BETTER') RETURNING id`, w.exp, w.timepoints["Day 14"])
	arena := func(paradigm, name, parameters string) string {
		env := w.id("INSERT INTO misko.environments (name, paradigm_key) VALUES ($1, $2) RETURNING id", w.exp+" "+name, paradigm)
		return w.id(`INSERT INTO misko.environment_revisions (environment_id, paradigm_key, paradigm_version, number, parameters, created_by)
			VALUES ($1, $2, 1, 1, $3, $4) RETURNING id`, env, paradigm, parameters, studyAuthor)
	}
	s.rotarod = []string{arena("ROTAROD", "lane 1", "{}"), arena("ROTAROD", "lane 2", "{}")}
	s.novel = []string{arena("NOVEL_OBJECT", "box", `{"arena_width_cm": 50, "arena_height_cm": 50}`)}
	return s
}

// test plans a session of one protocol step with one animal per arena,
// starts it and, when clipStartUs is not negative, attaches a verified video
// whose clip runs from clipStartUs for 60 s.
func (s *scoringStudy) test(step int, paradigm string, arenas []string, clipStartUs int64, allocations ...string) (string, []string) {
	s.t.Helper()
	test := s.id(`INSERT INTO misko.tests (experiment_id, timepoint_id, protocol_version_id, step_position, paradigm_key, paradigm_version, planned_trials, scheduled_at, created_by)
		VALUES ($1, $2, $3, $4, $5, 1, 1, now() - interval '1 hour', $6) RETURNING id`, s.exp, s.timepoints["Day 14"], s.version, step, paradigm, studyAuthor)
	var subjects []string
	for i, allocation := range allocations {
		subjects = append(subjects, s.id(`INSERT INTO misko.test_subjects (experiment_id, test_id, allocation_id, environment_revision_id, paradigm_key, paradigm_version, created_by)
			VALUES ($1, $2, $3, $4, $5, 1, $6) RETURNING id`, s.exp, test, allocation, arenas[i], paradigm, studyAuthor))
	}
	s.exec("UPDATE misko.tests SET status = 'IN_PROGRESS', started_at = now() - interval '30 minutes' WHERE id = $1", test)
	if clipStartUs >= 0 {
		s.objects++
		asset := s.id(`INSERT INTO misko.video_assets (kind, bucket, object_name, content_type, size_bytes, crc32c, status, generation, verified_at, created_by)
			VALUES ('ORIGINAL', 'misko-test-videos', $1, 'video/mp4', 4096, 1, 'VERIFIED', 42, now(), $2) RETURNING id`,
			fmt.Sprintf("videos/%s/%d.mp4", s.exp, s.objects), studyAuthor)
		s.exec(`INSERT INTO misko.test_recordings (experiment_id, test_id, video_asset_id, clip_start_us, clip_end_us, created_by)
			VALUES ($1, $2, $3, $4, $5, $6)`, s.exp, test, asset, clipStartUs, clipStartUs+60_000_000, studyAuthor)
	}
	return test, subjects
}

func metricValue(t *testing.T, metrics []any, key string) any {
	t.Helper()
	for _, m := range metrics {
		if m.(map[string]any)["key"] == key {
			return m.(map[string]any)["value"]
		}
	}
	t.Fatalf("metric %s missing from %v", key, metrics)
	return nil
}

// Rotarod and novel object recognition are scored on their videos, enter the
// review queue as SCORED runs and, once accepted, the experiment report.
func TestScoringOverHTTP(t *testing.T) {
	ctx := context.Background()
	a := newStudyAPI(t, nil)
	s := newScoringStudy(t, newStudyWorld(t, a.pool, "EXP-S"))
	m1, m2, m3, m4 := s.allocate("G1", "S-1"), s.allocate("G2", "S-2"), s.allocate("G1", "S-3"), s.allocate("G1", "S-4")
	s.start()

	// The rotarod clip starts 2 s into the video; event times are from the clip start.
	rotarodTest, lanes := s.test(1, "ROTAROD", s.rotarod, 2_000_000, m1, m2)
	scoring := "/api/tests/" + rotarodTest + "/subjects/" + lanes[0] + "/scoring"
	a.expect("anonymous", "GET", scoring, "", nil, 401, "auth.unauthenticated")
	view := a.expect("read", "GET", scoring, "VIEWER", nil, 200, "")
	events := view["inputEvents"].([]any)
	if view["sheet"] != nil || len(view["events"].([]any)) != 0 || len(events) != 1 || events[0].(map[string]any)["type"] != "fall" ||
		events[0].(map[string]any)["single"] != true || view["video"].(map[string]any)["clipStartUs"] != float64(2_000_000) {
		t.Fatalf("empty scoring: %v", view)
	}
	if tr := a.expect("read in Turkish", "GET", scoring+"?lang=tr", "VIEWER", nil, 200, "")["inputEvents"].([]any)[0].(map[string]any); tr["label"] != "Düşme" {
		t.Fatalf("turkish label: %v", tr)
	}
	a.expect("other test", "GET", "/api/tests/"+s.exp+"/subjects/"+lanes[0]+"/scoring", "VIEWER", nil, 404, "scoring.testSubjectNotFound")
	a.expect("malformed id", "GET", "/api/tests/"+rotarodTest+"/subjects/x/scoring", "VIEWER", nil, 404, "scoring.testSubjectNotFound")

	add := scoring + "/events"
	fall := map[string]any{"type": "fall", "startUs": 30_000_000}
	a.expect("viewer scores", "POST", add, "VIEWER", fall, 403, "auth.forbidden")
	a.expect("unknown type", "POST", add, "TECHNICIAN", map[string]any{"type": "jump", "startUs": 1}, 400, "scoring.unknownEventType")
	a.expect("label on a fall", "POST", add, "TECHNICIAN", map[string]any{"type": "fall", "label": "novel", "startUs": 1}, 400, "scoring.invalidLabel")
	a.expect("point with a length", "POST", add, "TECHNICIAN", map[string]any{"type": "fall", "startUs": 1, "endUs": 2}, 400, "scoring.invalidEventTime")
	a.expect("after the clip", "POST", add, "TECHNICIAN", map[string]any{"type": "fall", "startUs": 60_000_001}, 400, "scoring.eventOutsideClip")
	a.expect("unknown field", "POST", add, "TECHNICIAN", map[string]any{"type": "fall", "startUs": 1, "at": 1}, 400, "common.invalidJSON")
	event := a.expect("score the fall", "POST", add, "TECHNICIAN", fall, 201, "")
	if event["kind"] != "POINT" || event["endUs"] != float64(30_000_000) || event["label"] != nil || event["scoredBy"] == nil {
		t.Fatalf("event: %v", event)
	}
	a.expect("second fall", "POST", add, "TECHNICIAN", fall, 409, "scoring.singleEvent")
	eventPath := add + "/" + event["id"].(string)
	a.expect("change an unknown event", "PATCH", add+"/"+s.exp, "TECHNICIAN", fall, 404, "scoring.eventNotFound")
	moved := a.expect("move the fall", "PATCH", eventPath, "TECHNICIAN", map[string]any{"type": "fall", "startUs": 25_000_000}, 200, "")
	if moved["startUs"] != float64(25_000_000) || moved["id"] != event["id"] {
		t.Fatalf("moved: %v", moved)
	}
	draft := a.expect("draft", "GET", scoring, "VIEWER", nil, 200, "")
	if sheet := draft["sheet"].(map[string]any); sheet["status"] != "DRAFT" || sheet["version"] != float64(1) || len(draft["events"].([]any)) != 1 {
		t.Fatalf("draft: %v", draft)
	}

	complete := scoring + "/complete"
	a.expect("viewer completes", "POST", complete, "VIEWER", map[string]any{}, 403, "auth.forbidden")
	a.expect("unknown scored type", "POST", complete, "TECHNICIAN", map[string]any{"scoredEventTypes": []string{"jump"}}, 400, "scoring.unknownScoredType")
	a.expect("fall not scored", "POST", complete, "TECHNICIAN", map[string]any{"scoredEventTypes": []string{}}, 400, "scoring.unscoredEvent")
	a.expect("length beyond the clip", "POST", complete, "TECHNICIAN", map[string]any{"durationUs": 61_000_000}, 400, "scoring.invalidDuration")
	a.expect("fall after the length", "POST", complete, "TECHNICIAN", map[string]any{"durationUs": 20_000_000}, 400, "scoring.eventAfterDuration")
	done := a.expect("complete", "POST", complete, "TECHNICIAN", map[string]any{}, 200, "")
	sheet := done["sheet"].(map[string]any)
	run := done["run"].(map[string]any)
	if sheet["status"] != "COMPLETED" || sheet["durationUs"] != float64(60_000_000) || sheet["runId"] != run["id"] || sheet["recordingId"] == nil ||
		metricValue(t, run["metrics"].([]any), "latency_to_fall_s") != float64(25) || metricValue(t, run["metrics"].([]any), "fall_detected") != float64(1) {
		t.Fatalf("completion: %v", done)
	}
	firstRun := run["id"].(string)
	a.expect("score after completion", "POST", add, "TECHNICIAN", fall, 409, "scoring.notDraft")
	a.expect("remove after completion", "DELETE", eventPath, "TECHNICIAN", nil, 409, "scoring.notDraft")
	a.expect("complete twice", "POST", complete, "TECHNICIAN", map[string]any{}, 409, "scoring.notDraft")

	// The run is a SCORED analysis run: no worker, the scorer recorded.
	runs := a.expect("runs", "GET", "/api/tests/"+rotarodTest+"/analysis-runs", "VIEWER", nil, 200, "")["data"].([]any)
	if r := runs[0].(map[string]any); len(runs) != 1 || r["method"] != "SCORED" || r["status"] != "SUCCEEDED" || r["workerId"] != nil || r["scoredBy"] == nil ||
		r["clipStartUs"] != float64(2_000_000) || r["parameters"].(map[string]any)["max_trial_duration_s"] != float64(60) {
		t.Fatalf("scored run: %v", runs)
	}
	detail := a.expect("run detail", "GET", "/api/analysis-runs/"+firstRun, "VIEWER", nil, 200, "")["result"].(map[string]any)
	if len(detail["metrics"].([]any)) != 3 || detail["pair"] != nil || len(detail["events"].([]any)) != 1 {
		t.Fatalf("run detail: %v", detail)
	}

	// The second animal stayed on the rod: completed without events, censored at the cut-off.
	other := a.expect("complete without a fall", "POST", "/api/tests/"+rotarodTest+"/subjects/"+lanes[1]+"/scoring/complete", "TECHNICIAN", map[string]any{}, 200, "")
	if metrics := other["run"].(map[string]any)["metrics"].([]any); metricValue(t, metrics, "latency_to_fall_s") != float64(60) || metricValue(t, metrics, "fall_detected") != float64(0) {
		t.Fatalf("censored: %v", other)
	}

	// No worker ever takes an observer-scored paradigm.
	register := map[string]any{"name": "tracker", "modelVersion": "1", "capabilities": []any{map[string]any{"paradigmKey": "ROTAROD", "paradigmVersion": 1}}}
	a.expect("rotarod worker", "POST", "/api/analysis/workers", "SUPERADMIN", register, 400, "worker.unknownCapability")
	s.exec(`INSERT INTO misko.analysis_workers (name, model_version, token_sha256, created_by) VALUES ('rogue', '1', decode(repeat('ab', 32), 'hex'), $1)`, studyAuthor)
	s.exec(`INSERT INTO misko.worker_capabilities (worker_id, paradigm_key, paradigm_version) SELECT id, 'ROTAROD', 1 FROM misko.analysis_workers WHERE name = 'rogue'`)
	if err := a.api.AnalysisTick(ctx); err != nil {
		t.Fatal(err)
	}
	a.expect("reanalyse a scored animal", "POST", "/api/analysis/test-subjects/"+lanes[0]+"/runs", "TECHNICIAN", map[string]any{}, 409, "analysis.notReady")
	if runs := a.expect("runs after a tick", "GET", "/api/tests/"+rotarodTest+"/analysis-runs", "VIEWER", nil, 200, "")["data"].([]any); len(runs) != 2 {
		t.Fatalf("a worker run was queued: %v", runs)
	}

	// Reopening starts version 2 with the events; completing it adds a run
	// and leaves the reviewed choice to the reviewer.
	a.expect("reopen as viewer", "POST", scoring+"/reopen", "VIEWER", map[string]any{}, 403, "auth.forbidden")
	reopened := a.expect("reopen", "POST", scoring+"/reopen", "TECHNICIAN", map[string]any{}, 201, "")
	if reopened["sheet"].(map[string]any)["version"] != float64(2) || reopened["sheet"].(map[string]any)["status"] != "DRAFT" || len(reopened["events"].([]any)) != 1 {
		t.Fatalf("reopened: %v", reopened)
	}
	a.expect("reopen a draft", "POST", scoring+"/reopen", "TECHNICIAN", map[string]any{}, 409, "scoring.notCompleted")
	copied := reopened["events"].([]any)[0].(map[string]any)["id"].(string)
	a.expect("remove the fall", "DELETE", add+"/"+copied, "TECHNICIAN", nil, 204, "")
	a.expect("remove it again", "DELETE", add+"/"+copied, "TECHNICIAN", nil, 404, "scoring.eventNotFound")
	a.expect("score it again", "POST", add, "TECHNICIAN", map[string]any{"type": "fall", "startUs": 20_000_000}, 201, "")
	second := a.expect("complete version 2", "POST", complete, "TECHNICIAN", map[string]any{}, 200, "")["run"].(map[string]any)["id"].(string)

	// Both animals wait in the review queue as SCORED results.
	results := a.expect("review queue", "GET", "/api/analysis/results?experimentId="+s.exp, "VIEWER", nil, 200, "")
	if results["total"] != float64(2) {
		t.Fatalf("review queue: %v", results)
	}
	item := a.expect("result", "GET", "/api/analysis/results/"+lanes[0], "VIEWER", nil, 200, "")
	if item["status"] != "PENDING" || item["run"].(map[string]any)["id"] != firstRun || item["run"].(map[string]any)["method"] != "SCORED" ||
		item["latestRunId"] != second || item["newerRunAvailable"] != true {
		t.Fatalf("review item: %v", item)
	}
	a.expect("choose version 2", "POST", "/api/analysis/results/"+lanes[0]+"/run", "TECHNICIAN", map[string]any{"runId": second}, 200, "")

	// Novel object recognition with a clip starting at 0.
	norTest, boxes := s.test(2, "NOVEL_OBJECT", s.novel, 0, m3)
	nor := "/api/tests/" + norTest + "/subjects/" + boxes[0] + "/scoring"
	if e := a.expect("nor events", "GET", nor, "VIEWER", nil, 200, "")["inputEvents"].([]any)[0].(map[string]any); e["kind"] != "INTERVAL" || len(e["labels"].([]any)) != 2 {
		t.Fatalf("nor input events: %v", e)
	}
	a.expect("exploration without an object", "POST", nor+"/events", "TECHNICIAN", map[string]any{"type": "exploration", "startUs": 0, "endUs": 1}, 400, "scoring.invalidLabel")
	a.expect("exploration without an end", "POST", nor+"/events", "TECHNICIAN", map[string]any{"type": "exploration", "label": "novel", "startUs": 0}, 400, "scoring.invalidEventTime")
	for _, e := range []map[string]any{
		{"type": "exploration", "label": "novel", "startUs": 0, "endUs": 3_000_000},
		{"type": "exploration", "label": "novel", "startUs": 2_000_000, "endUs": 4_000_000},
		{"type": "exploration", "label": "familiar", "startUs": 5_000_000, "endUs": 6_000_000},
	} {
		a.expect("explore", "POST", nor+"/events", "TECHNICIAN", e, 201, "")
	}
	norDone := a.expect("complete nor", "POST", nor+"/complete", "TECHNICIAN", map[string]any{}, 200, "")["run"].(map[string]any)["metrics"].([]any)
	if v := metricValue(t, norDone, "discrimination_index").(float64); math.Abs(v-0.6) > 1e-9 || metricValue(t, norDone, "novel_object_time_s") != float64(4) {
		t.Fatalf("nor metrics: %v", norDone)
	}
	a.expect("bulk accept", "POST", "/api/analysis/results/bulk-accept", "RESEARCHER", map[string]any{"testSubjectIds": []string{lanes[0], lanes[1], boxes[0]}}, 200, "")

	// The accepted scored results are the report's numbers.
	report := a.expect("report", "GET", "/api/experiments/"+s.exp+"/report", "VIEWER", nil, 200, "")
	groups := map[string]string{}
	for _, g := range report["groups"].([]any) {
		groups[g.(map[string]any)["id"].(string)] = g.(map[string]any)["code"].(string)
	}
	means := map[string]any{}
	for _, sec := range report["sections"].([]any) {
		section := sec.(map[string]any)
		for _, g := range section["groups"].([]any) {
			stat := g.(map[string]any)
			if stat["n"] != float64(0) {
				means[section["endpoint"].(map[string]any)["id"].(string)+" "+groups[stat["groupId"].(string)]] = stat["mean"]
			}
		}
	}
	if means[s.latency+" G1"] != float64(20) || means[s.latency+" G2"] != float64(60) || math.Abs(means[s.recognition+" G1"].(float64)-0.6) > 1e-9 {
		t.Fatalf("report means: %v", means)
	}

	// Animals that cannot be scored.
	openField, tracked := s.session("Day 14", m4)
	a.expect("tracked paradigm", "GET", "/api/tests/"+openField+"/subjects/"+tracked[0]+"/scoring", "VIEWER", nil, 409, "scoring.notObserverScored")
	noVideo, waiting := s.test(1, "ROTAROD", s.rotarod, -1, m4)
	a.expect("no video", "POST", "/api/tests/"+noVideo+"/subjects/"+waiting[0]+"/scoring/events", "TECHNICIAN", fall, 409, "scoring.noVideo")
	if v := a.expect("read without a video", "GET", "/api/tests/"+noVideo+"/subjects/"+waiting[0]+"/scoring", "VIEWER", nil, 200, ""); v["video"] != nil {
		t.Fatalf("video: %v", v)
	}
	s.exec("UPDATE misko.allocations SET excluded_at = now(), exclusion_reason = 'HUMANE_ENDPOINT' WHERE id = $1", m3)
	a.expect("excluded animal", "POST", nor+"/reopen", "TECHNICIAN", map[string]any{}, 409, "scoring.notScorable")

	// The database keeps the rules when the API is bypassed.
	violation := func(label, sql string, args ...any) {
		t.Helper()
		if _, err := a.pool.Exec(ctx, sql, args...); err == nil {
			t.Fatalf("%s: accepted", label)
		}
	}
	violation("event on a completed sheet", `INSERT INTO misko.scored_observations (sheet_id, test_subject_id, event_type, kind, start_us, end_us, scored_by)
		SELECT id, test_subject_id, 'fall', 'POINT', 1, 1, created_by FROM misko.scoring_sheets WHERE test_subject_id = $1 AND version = 1`, lanes[0])
	violation("reopen a completed sheet", "UPDATE misko.scoring_sheets SET status = 'DRAFT' WHERE test_subject_id = $1 AND version = 1", lanes[0])
	violation("change a run's method", "UPDATE misko.analysis_runs SET method = 'AUTOMATED' WHERE id = $1", firstRun)
	violation("scored run without its sheet", `INSERT INTO misko.analysis_runs (experiment_id, test_id, test_subject_id, recording_id, source_asset_id, source_generation,
			source_crc32c, clip_start_us, paradigm_key, paradigm_version, metric_engine_version, result_schema_version, parameters, trigger, status, attempt,
			max_attempts, available_at, method, scored_by)
		SELECT experiment_id, test_id, test_subject_id, recording_id, source_asset_id, source_generation, source_crc32c, clip_start_us, paradigm_key,
			paradigm_version, 1, 1, '{}', 'MANUAL', 'RUNNING', 1, 1, now(), 'SCORED', scored_by FROM misko.analysis_runs WHERE id = $1`, firstRun)
	violation("scored run with a worker", `INSERT INTO misko.analysis_runs (experiment_id, test_id, test_subject_id, recording_id, source_asset_id, source_generation,
			source_crc32c, clip_start_us, paradigm_key, paradigm_version, metric_engine_version, result_schema_version, parameters, trigger, status, attempt,
			max_attempts, available_at, method, scored_by)
		SELECT experiment_id, test_id, test_subject_id, recording_id, source_asset_id, source_generation, source_crc32c, clip_start_us, paradigm_key,
			paradigm_version, 1, 1, '{}', 'MANUAL', 'QUEUED', 0, 1, now(), 'SCORED', scored_by FROM misko.analysis_runs WHERE id = $1`, firstRun)
}
