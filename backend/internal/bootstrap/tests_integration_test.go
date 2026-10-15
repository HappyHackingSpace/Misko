//go:build integration

package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/config"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// sessionStudy is a study prepared with SQL, since the experiment, subject and
// environment contexts are ported separately: one experiment with two groups,
// two timepoints, five allocated animals, three open field arenas and a maze.
type sessionStudy struct {
	experiment, foreignExperiment string
	baseline, day7                string
	allocations                   map[string]string
	foreignAllocation             string
	boxA, boxB, boxC, maze        string
}

func sqlID(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&id); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return id
}

func sqlExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func newSessionStudy(t *testing.T, pool *pgxpool.Pool) sessionStudy {
	const user = "00000000-0000-7000-8000-000000000001"
	s := sessionStudy{allocations: map[string]string{}}
	s.experiment = sqlID(t, pool, `INSERT INTO misko.experiments (code, title) VALUES ('E-T', 'Sessions') RETURNING id`)
	s.foreignExperiment = sqlID(t, pool, `INSERT INTO misko.experiments (code, title) VALUES ('E-F', 'Other') RETURNING id`)
	healthy := sqlID(t, pool, `INSERT INTO misko.experiment_groups (experiment_id, code, name, role, target_size)
		VALUES ($1, 'G1', 'Healthy', 'HEALTHY_CONTROL', 4) RETURNING id`, s.experiment)
	other := sqlID(t, pool, `INSERT INTO misko.experiment_groups (experiment_id, code, name, role, target_size)
		VALUES ($1, 'G2', 'Healthy 2', 'HEALTHY_CONTROL', 4) RETURNING id`, s.experiment)
	foreignGroup := sqlID(t, pool, `INSERT INTO misko.experiment_groups (experiment_id, code, name, role)
		VALUES ($1, 'G1', 'Healthy', 'HEALTHY_CONTROL') RETURNING id`, s.foreignExperiment)
	s.baseline = sqlID(t, pool, `INSERT INTO misko.timepoints (experiment_id, name, study_day, position) VALUES ($1, 'Baseline', -1, 1) RETURNING id`, s.experiment)
	s.day7 = sqlID(t, pool, `INSERT INTO misko.timepoints (experiment_id, name, study_day, position) VALUES ($1, 'Day 7', 7, 2) RETURNING id`, s.experiment)
	for i, code := range []string{"M1", "M2", "M3", "M4", "M5", "F1"} {
		subject := sqlID(t, pool, `INSERT INTO misko.subjects (code, species, sex) VALUES ($1, 'MOUSE', 'MALE') RETURNING id`, code)
		experiment, group := s.experiment, healthy
		if i%2 == 1 {
			group = other
		}
		if code == "F1" {
			experiment, group = s.foreignExperiment, foreignGroup
		}
		allocation := sqlID(t, pool, `INSERT INTO misko.allocations (experiment_id, group_id, subject_id, allocated_at, created_by)
			VALUES ($1, $2, $3, now() - interval '30 days', $4) RETURNING id`, experiment, group, subject, user)
		if code == "F1" {
			s.foreignAllocation = allocation
			continue
		}
		s.allocations[code] = allocation
	}
	arena := func(name, paradigm, parameters string) string {
		env := sqlID(t, pool, `INSERT INTO misko.environments (name, paradigm_key) VALUES ($1, $2) RETURNING id`, name, paradigm)
		return sqlID(t, pool, `INSERT INTO misko.environment_revisions (environment_id, paradigm_key, paradigm_version, number, parameters, created_by)
			VALUES ($1, $2, 1, 1, $3, $4) RETURNING id`, env, paradigm, parameters, user)
	}
	box := `{"arena_width_cm": 50, "arena_height_cm": 50}`
	s.boxA, s.boxB, s.boxC = arena("Box A", "OPEN_FIELD", box), arena("Box B", "OPEN_FIELD", box), arena("Box C", "OPEN_FIELD", box)
	s.maze = arena("Maze", "EPM", `{}`)
	return s
}

// TestTestSessionsOverHTTP plans sessions with several animals, enforces one
// test per animal, timepoint and step, runs trials per animal and checks every
// role through the composed API. Protocol steps no longer name an arena.
func TestTestSessionsOverHTTP(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.Database(t)
	setup, err := Setup(ctx, pool, config.Setup{AdminEmail: "admin@lab.io", LabName: "Lab", LabTimezone: "Europe/Istanbul", BcryptCost: bcrypt.MinCost})
	if err != nil {
		t.Fatal(err)
	}
	api, err := NewAPI(pool, config.Config{ProbeTimeout: time.Second},
		config.Auth{JWTSecret: []byte(strings.Repeat("s", 32)), JWTIssuer: "misko", JWTAudience: "misko-api", TokenTTL: time.Hour, BcryptCost: bcrypt.MinCost},
		slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.Handler)
	defer server.Close()
	c := client{t: t, url: server.URL, http: server.Client()}

	admin := c.login("admin@lab.io", setup.AdminPassword)
	tokens := map[string]string{"SUPERADMIN": admin}
	for _, role := range []string{"LAB_MANAGER", "RESEARCHER", "TECHNICIAN", "VIEWER"} {
		email := strings.ToLower(role) + "@lab.io"
		c.create("/api/users", admin, map[string]any{"email": email, "name": role, "role": role, "password": "Password-123"})
		tokens[role] = c.login(email, "Password-123")
	}
	researcher, technician, viewer := tokens["RESEARCHER"], tokens["TECHNICIAN"], tokens["VIEWER"]
	s := newSessionStudy(t, pool)
	base := "/api/experiments/" + s.experiment

	// Protocols: steps name a paradigm and session values, never an arena.
	steps := []any{
		map[string]any{"position": 1, "paradigmKey": "OPEN_FIELD", "paradigmVersion": 1, "trialType": "STANDARD", "trials": 2},
		map[string]any{"position": 2, "paradigmKey": "ROTAROD", "paradigmVersion": 1, "trialType": "ACCELERATING", "trials": 3,
			"interTrialIntervalS": 900, "session": map[string]any{"start_rpm": 4, "end_rpm": 40}},
	}
	battery := map[string]any{"name": "Battery", "version": map[string]any{"steps": steps}}
	c.expect("technician creates protocol", c.call("POST", base+"/protocols", technician, battery), 403, "auth.forbidden")
	created := c.create(base+"/protocols", researcher, battery)
	version := created["version"].(map[string]any)
	rotarod := version["steps"].([]any)[1].(map[string]any)
	session := rotarod["session"].(map[string]any)
	if _, ok := rotarod["environmentRevisionId"]; ok || session["start_rpm"] != float64(4) || session["max_trial_duration_s"] == nil {
		t.Fatalf("resolved step: %v", rotarod)
	}
	invalid := func(step map[string]any) map[string]any {
		return map[string]any{"name": "Bad", "version": map[string]any{"steps": []any{step}}}
	}
	c.expect("session out of order", c.call("POST", base+"/protocols", researcher, invalid(map[string]any{"position": 1, "paradigmKey": "ROTAROD", "paradigmVersion": 1,
		"trialType": "ACCELERATING", "trials": 1, "session": map[string]any{"start_rpm": 40, "end_rpm": 4}})), 400, "protocol.invalidSession")
	c.expect("apparatus value in a session", c.call("POST", base+"/protocols", researcher, invalid(map[string]any{"position": 1, "paradigmKey": "OPEN_FIELD",
		"paradigmVersion": 1, "trialType": "STANDARD", "trials": 1, "session": map[string]any{"arena_width_cm": 40}})), 400, "protocol.invalidSession")
	c.expect("arena on a step", c.call("POST", base+"/protocols", researcher, invalid(map[string]any{"position": 1, "paradigmKey": "OPEN_FIELD",
		"paradigmVersion": 1, "trialType": "STANDARD", "trials": 1, "environmentRevisionId": s.boxA})), 400, "common.invalidJSON")
	c.expect("unknown trial type", c.call("POST", base+"/protocols", researcher, invalid(map[string]any{"position": 1, "paradigmKey": "OPEN_FIELD",
		"paradigmVersion": 1, "trialType": "PROBE", "trials": 1})), 400, "protocol.trialTypeNotSupported")
	c.expect("unknown experiment", c.call("POST", "/api/experiments/"+s.allocations["M1"]+"/protocols", researcher, battery), 404, "experiment.notFound")
	foreignProtocol := c.create("/api/experiments/"+s.foreignExperiment+"/protocols", researcher, battery)
	foreignVersion := id(foreignProtocol["version"].(map[string]any))

	plan := func(timepoint string, subjects ...[2]string) map[string]any {
		body := map[string]any{"timepointId": timepoint, "protocolVersionId": id(version), "stepPosition": 1}
		list := []any{}
		for _, subject := range subjects {
			list = append(list, map[string]any{"allocationId": subject[0], "environmentRevisionId": subject[1]})
		}
		body["subjects"] = list
		return body
	}
	m := s.allocations
	pair := plan(s.day7, [2]string{m["M1"], s.boxA}, [2]string{m["M2"], s.boxB})
	c.expect("plan in a draft", c.call("POST", base+"/tests", researcher, pair), 409, "test.experimentNotRunning")

	// Start the study: day 0 is 1 September; the lab is in Istanbul (UTC+3).
	sqlExec(t, pool, `UPDATE misko.experiments SET status = 'RUNNING', day_zero = '2026-09-01', started_at = now() WHERE id = $1`, s.experiment)
	sqlExec(t, pool, `UPDATE misko.allocations SET excluded_at = now(), exclusion_reason = 'DIED' WHERE id = $1`, m["M5"])

	for _, role := range []string{"TECHNICIAN", "VIEWER"} {
		c.expect(role+" plans", c.call("POST", base+"/tests", tokens[role], pair), 403, "auth.forbidden")
	}
	test := c.create(base+"/tests", researcher, pair)
	subjects := test["subjects"].([]any)
	if test["status"] != "PLANNED" || test["scheduledAt"] != "2026-09-07T21:00:00Z" || test["timepointName"] != "Day 7" || test["studyDay"] != float64(7) ||
		test["paradigmKey"] != "OPEN_FIELD" || test["plannedTrials"] != float64(2) || test["protocolName"] != "Battery" || len(subjects) != 2 {
		t.Fatalf("planned test: %v", test)
	}
	first := subjects[0].(map[string]any)
	if first["subjectCode"] != "M1" || first["environmentName"] != "Box A" || first["allocationId"] != m["M1"] || first["groupId"] != nil {
		t.Fatalf("first animal: %v", first)
	}
	path := "/api/tests/" + id(test)

	for label, tc := range map[string]struct {
		body   map[string]any
		status int
		code   string
	}{
		"animal already in the slot": {plan(s.day7, [2]string{m["M1"], s.boxC}), 409, "test.animalAlreadyScheduled"},
		"excluded animal":            {plan(s.day7, [2]string{m["M5"], s.boxC}), 409, "test.animalExcluded"},
		"arena of another paradigm":  {plan(s.day7, [2]string{m["M3"], s.maze}), 400, "test.arenaParadigmMismatch"},
		"unknown arena":              {plan(s.day7, [2]string{m["M3"], m["M3"]}), 404, "environment.revisionNotFound"},
		"same arena twice":           {plan(s.day7, [2]string{m["M3"], s.boxC}, [2]string{m["M4"], s.boxC}), 409, "test.duplicateArena"},
		"same animal twice":          {plan(s.day7, [2]string{m["M3"], s.boxC}, [2]string{m["M3"], s.boxA}), 409, "test.duplicateAnimal"},
		"animal of another study":    {plan(s.day7, [2]string{s.foreignAllocation, s.boxC}), 404, "allocation.notFound"},
		"no animals":                 {plan(s.day7), 400, "test.noSubjects"},
		"timepoint of another study": {plan(s.experiment, [2]string{m["M3"], s.boxC}), 404, "timepoint.notFound"},
		"step that does not exist":   {func() map[string]any { b := plan(s.day7, [2]string{m["M3"], s.boxC}); b["stepPosition"] = 3; return b }(), 404, "protocol.stepNotFound"},
		"protocol of another study": {func() map[string]any {
			b := plan(s.day7, [2]string{m["M3"], s.boxC})
			b["protocolVersionId"] = foreignVersion
			return b
		}(), 404, "protocol.versionNotFound"},
	} {
		c.expect(label, c.call("POST", base+"/tests", researcher, tc.body), tc.status, tc.code)
	}
	// Another timepoint is another slot.
	scheduled := plan(s.baseline, [2]string{m["M1"], s.boxA})
	scheduled["scheduledAt"] = "2026-08-31T08:30:00Z"
	baselineTest := c.create(base+"/tests", researcher, scheduled)
	if baselineTest["scheduledAt"] != "2026-08-31T08:30:00Z" {
		t.Fatalf("explicit schedule: %v", baselineTest)
	}

	// Animals join and leave while the test is planned.
	c.expect("arena already used", c.call("POST", path+"/subjects", researcher, map[string]any{"allocationId": m["M3"], "environmentRevisionId": s.boxA}), 409, "test.duplicateArena")
	c.expect("technician adds", c.call("POST", path+"/subjects", technician, map[string]any{"allocationId": m["M3"], "environmentRevisionId": s.boxC}), 403, "auth.forbidden")
	grown := c.create(path+"/subjects", researcher, map[string]any{"allocationId": m["M3"], "environmentRevisionId": s.boxC})
	if len(grown["subjects"].([]any)) != 3 {
		t.Fatalf("after add: %v", grown)
	}
	third := grown["subjects"].([]any)[2].(map[string]any)
	if third["subjectCode"] != "M3" {
		t.Fatalf("third animal: %v", third)
	}
	c.expect("unknown animal", c.call("DELETE", path+"/subjects/"+id(test), researcher, nil), 404, "test.subjectNotFound")
	shrunk := c.expect("remove M3", c.call("DELETE", path+"/subjects/"+id(third), researcher, nil), 200, "").body
	if len(shrunk["subjects"].([]any)) != 2 {
		t.Fatalf("after remove: %v", shrunk)
	}
	lonely := c.create(base+"/tests", researcher, plan(s.day7, [2]string{m["M4"], s.boxC}))
	c.expect("remove the last animal", c.call("DELETE", "/api/tests/"+id(lonely)+"/subjects/"+id(lonely["subjects"].([]any)[0].(map[string]any)), researcher, nil),
		409, "test.lastSubject")
	// A cancelled test frees its animals for the slot.
	c.expect("cancel lonely", c.call("POST", "/api/tests/"+id(lonely)+"/cancel", researcher, map[string]any{"reason": "camera broken"}), 200, "")
	replanned := c.create(base+"/tests", researcher, plan(s.day7, [2]string{m["M4"], s.boxC}))

	// Two planners racing for the same animal in the same slot: one wins.
	sqlExec(t, pool, `INSERT INTO misko.subjects (code, species, sex) VALUES ('M6', 'MOUSE', 'MALE')`)
	m6 := sqlID(t, pool, `INSERT INTO misko.allocations (experiment_id, group_id, subject_id, allocated_at, created_by)
		SELECT $1, a.group_id, s.id, now(), a.created_by FROM misko.allocations a, misko.subjects s WHERE a.id = $2 AND s.code = 'M6' RETURNING id`, s.experiment, m["M1"])
	statuses := make([]int, 4)
	var wg sync.WaitGroup
	for i := range statuses {
		wg.Add(1)
		go func() {
			defer wg.Done()
			arena := []string{s.boxA, s.boxB, s.boxC, s.boxA}[i]
			raw, _ := json.Marshal(plan(s.baseline, [2]string{m6, arena}))
			req, _ := http.NewRequest("POST", server.URL+base+"/tests", bytes.NewReader(raw))
			req.Header.Set("Authorization", "Bearer "+researcher)
			if res, err := server.Client().Do(req); err == nil {
				statuses[i] = res.StatusCode
				res.Body.Close()
			}
		}()
	}
	wg.Wait()
	created201 := 0
	for _, status := range statuses {
		switch status {
		case 201:
			created201++
		case 409:
		default:
			t.Fatalf("concurrent plans: %v", statuses)
		}
	}
	if created201 != 1 {
		t.Fatalf("concurrent plans of one animal: %v", statuses)
	}

	// Running the session: trials belong to one animal each.
	for role, token := range tokens {
		if role == "VIEWER" {
			c.expect(role+" starts", c.call("POST", path+"/start", token, map[string]any{}), 403, "auth.forbidden")
		}
	}
	stamp := func(d time.Duration) string { return time.Now().UTC().Add(d).Format(time.RFC3339Nano) }
	started := c.expect("technician starts", c.call("POST", path+"/start", technician, map[string]any{"startedAt": stamp(-30 * time.Minute)}), 200, "").body
	if started["status"] != "IN_PROGRESS" {
		t.Fatalf("started: %v", started)
	}
	c.expect("add after start", c.call("POST", path+"/subjects", researcher, map[string]any{"allocationId": m["M3"], "environmentRevisionId": s.boxC}), 409, "test.notPlanned")
	animals := started["subjects"].([]any)
	s1, s2 := id(animals[0].(map[string]any)), id(animals[1].(map[string]any))
	c.expect("remove after start", c.call("DELETE", path+"/subjects/"+s2, researcher, nil), 409, "test.notPlanned")
	trials := func(subject string) string { return path + "/subjects/" + subject + "/trials" }
	one := c.create(trials(s1), technician, map[string]any{"repetition": 1, "startedAt": stamp(-25 * time.Minute), "endedAt": stamp(-20 * time.Minute)})
	again := c.create(trials(s1), technician, map[string]any{"repetition": 1, "startedAt": stamp(-15 * time.Minute), "notes": "jumped out"})
	if one["number"] != float64(1) || again["number"] != float64(2) || again["attempt"] != float64(2) || again["testSubjectId"] != s1 {
		t.Fatalf("trials of M1: %v %v", one, again)
	}
	c.expect("viewer records trial", c.call("POST", trials(s2), viewer, map[string]any{"repetition": 1, "startedAt": stamp(-time.Minute)}), 403, "auth.forbidden")
	c.expect("trial of another test's animal", c.call("POST", "/api/tests/"+id(replanned)+"/subjects/"+s1+"/trials", technician,
		map[string]any{"repetition": 1, "startedAt": stamp(-time.Minute)}), 404, "test.subjectNotFound")
	c.expect("trial in a planned test", c.call("POST", "/api/tests/"+id(replanned)+"/subjects/"+id(replanned["subjects"].([]any)[0].(map[string]any))+"/trials",
		technician, map[string]any{"repetition": 1, "startedAt": stamp(-time.Minute)}), 409, "test.notInProgress")
	c.expect("repetition beyond plan", c.call("POST", trials(s2), technician, map[string]any{"repetition": 3, "startedAt": stamp(-time.Minute)}), 400, "trial.invalidRepetition")
	c.expect("complete with M2 untested", c.call("POST", path+"/complete", technician, map[string]any{}), 409, "test.noTrials")
	other := c.create(trials(s2), technician, map[string]any{"repetition": 1, "startedAt": stamp(-10 * time.Minute)})
	if other["number"] != float64(1) || other["attempt"] != float64(1) {
		t.Fatalf("first trial of M2: %v", other)
	}
	if list := c.call("GET", trials(s1), viewer, nil).body["data"].([]any); len(list) != 2 {
		t.Fatalf("trials of M1: %v", list)
	}
	completed := c.expect("technician completes", c.call("POST", path+"/complete", technician, map[string]any{}), 200, "").body
	if completed["status"] != "COMPLETED" {
		t.Fatalf("completed: %v", completed)
	}
	c.expect("trial after completion", c.call("POST", trials(s2), technician, map[string]any{"repetition": 2, "startedAt": stamp(0)}), 409, "test.notInProgress")
	c.expect("cancel a completed test", c.call("POST", path+"/cancel", researcher, map[string]any{"reason": "mistake"}), 409, "test.invalidTransition")

	detail := c.expect("viewer reads", c.call("GET", path, viewer, nil), 200, "").body
	byAnimal := map[string]int{}
	for _, raw := range detail["subjects"].([]any) {
		subject := raw.(map[string]any)
		byAnimal[subject["subjectCode"].(string)] = len(subject["trials"].([]any))
	}
	if byAnimal["M1"] != 2 || byAnimal["M2"] != 1 || len(byAnimal) != 2 {
		t.Fatalf("detail trials per animal: %v", detail)
	}

	// An animal excluded after planning blocks the start until it is removed.
	late := c.create(base+"/tests", researcher, plan(s.day7, [2]string{m["M3"], s.boxA}, [2]string{m6, s.boxB}))
	sqlExec(t, pool, `UPDATE misko.allocations SET excluded_at = now(), exclusion_reason = 'HUMANE_ENDPOINT' WHERE id = $1`, m["M3"])
	c.expect("start with an excluded animal", c.call("POST", "/api/tests/"+id(late)+"/start", technician, map[string]any{}), 409, "test.subjectExcluded")
	c.expect("remove the excluded animal", c.call("DELETE", "/api/tests/"+id(late)+"/subjects/"+id(late["subjects"].([]any)[0].(map[string]any)), researcher, nil), 200, "")
	c.expect("start without it", c.call("POST", "/api/tests/"+id(late)+"/start", technician, map[string]any{}), 200, "")

	// Worklist filters.
	count := func(query string) float64 {
		t.Helper()
		return c.expect("GET "+query, c.call("GET", query, viewer, nil), 200, "").body["total"].(float64)
	}
	for query, want := range map[string]float64{
		"/api/tests": 6,
		"/api/tests?experimentId=" + s.experiment:  6,
		"/api/tests?status=COMPLETED":              1,
		"/api/tests?status=CANCELLED":              1,
		"/api/tests?timepointId=" + s.baseline:     2,
		"/api/tests?from=2026-09-08&to=2026-09-08": 4,
		"/api/tests?subjectId=" + sqlID(t, pool, `SELECT subject_id FROM misko.allocations WHERE id = $1`, m["M1"]): 2,
		base + "/tests?timepointId=" + s.day7 + "&status=PLANNED":                                                   1,
		"/api/tests?paradigmKey=ROTAROD":                                                                            0,
		"/api/tests?experimentId=" + s.foreignExperiment:                                                            0,
	} {
		if got := count(query); got != want {
			t.Errorf("%s: total %v want %v", query, got, want)
		}
	}
	// Day 7 tests are planned for 8 September in Istanbul, which is still the
	// 7th in UTC; date filters use the laboratory's calendar.
	if got := count("/api/tests?timepointId=" + s.day7 + "&status=PLANNED&from=2026-09-08&to=2026-09-08"); got != 1 {
		t.Errorf("planned day 7 tests on the 8th: %v", got)
	}
	defaults := sqlID(t, pool, `SELECT count(*)::text FROM misko.tests WHERE scheduled_at = '2026-09-07T21:00:00Z'`)
	if got := count("/api/tests?from=2026-09-07&to=2026-09-07"); defaults == "0" || got != 0 {
		t.Errorf("worklist on the 7th in Istanbul: %v (stored at the default: %s)", got, defaults)
	}
	if got := count("/api/tests?from=2026-09-08"); got != 4 {
		t.Errorf("worklist from the 8th: %v", got)
	}
	newest := c.call("GET", "/api/tests?pageSize=1", viewer, nil).body["data"].([]any)[0].(map[string]any)
	oldest := c.call("GET", base+"/tests?pageSize=1", viewer, nil).body["data"].([]any)[0].(map[string]any)
	if newest["scheduledAt"] != "2026-09-07T21:00:00Z" || oldest["scheduledAt"] != "2026-08-30T21:00:00Z" || len(oldest["subjects"].([]any)) != 1 {
		t.Fatalf("ordering: newest %v oldest %v", newest, oldest)
	}
	for _, query := range []string{"/api/tests?status=DONE", "/api/tests?from=08-09-2026", "/api/tests?from=2026-09-09&to=2026-09-08"} {
		c.expect("bad query "+query, c.call("GET", query, viewer, nil), 400, "common.invalidQuery")
	}
	c.expect("anonymous worklist", c.call("GET", "/api/tests", "", nil), 401, "auth.unauthenticated")

	// Comments: every role may comment, only the author edits.
	comment := c.create(path+"/comments", viewer, map[string]any{"body": "Calm during the first trial"})
	c.expect("technician edits viewer comment", c.call("PATCH", path+"/comments/"+id(comment), technician, map[string]any{"body": "x"}), 403, "comment.cannotEdit")
	c.expect("lab manager deletes", c.call("DELETE", path+"/comments/"+id(comment), tokens["LAB_MANAGER"], nil), 204, "")

	// A completed experiment takes no new tests or protocols.
	sqlExec(t, pool, `UPDATE misko.experiments SET status = 'COMPLETED', completed_at = now() WHERE id = $1`, s.experiment)
	c.expect("plan in a completed study", c.call("POST", base+"/tests", researcher, plan(s.day7, [2]string{m["M2"], s.boxC})), 409, "test.experimentNotRunning")
	c.expect("protocol in a completed study", c.call("POST", base+"/protocols", researcher, battery), 409, "protocol.experimentClosed")
}
