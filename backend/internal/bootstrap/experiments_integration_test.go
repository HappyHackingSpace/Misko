//go:build integration

package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HappyHackingSpace/Misko/backend/internal/platform/config"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// TestExperimentsOverHTTP walks a study from design to completion through the
// composed API: read/write RBAC for every role, the design rules, the start
// check, allocation and randomisation, and what freezes when.
func TestExperimentsOverHTTP(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.Database(t)
	setup, err := Setup(ctx, pool, config.Setup{AdminEmail: "admin@lab.io", LabName: "Lab", LabTimezone: "UTC", BcryptCost: bcrypt.MinCost})
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
	writers := map[string]bool{"SUPERADMIN": true, "LAB_MANAGER": true, "RESEARCHER": true}
	for role, token := range tokens {
		status, code := 403, "auth.forbidden"
		if writers[role] {
			status, code = 201, ""
		}
		c.expect(role+" creates experiment", c.call("POST", "/api/experiments", token, map[string]any{"code": "E-" + role, "title": "Study"}), status, code)
	}
	c.expect("removed field", c.call("POST", "/api/experiments", admin, map[string]any{"code": "X", "title": "T", "requiresControl": true}), 400, "common.invalidJSON")

	// Catalog rows, animals and the protocol belong to other contexts.
	sql := sqlRunner{t, pool}
	model := sql.id(`INSERT INTO misko.disease_models (name, induction_type) VALUES ('MPTP Parkinson', 'CHEMICAL') RETURNING id`)
	mptp := sql.id(`INSERT INTO misko.substances (name, kind) VALUES ('MPTP', 'INDUCER') RETURNING id`)
	saline := sql.id(`INSERT INTO misko.substances (name, kind) VALUES ('Saline', 'VEHICLE') RETURNING id`)
	compound := sql.id(`INSERT INTO misko.substances (name, kind) VALUES ('Compound X', 'COMPOUND') RETURNING id`)
	animals := make([]string, 9)
	for i := range animals {
		animals[i] = sql.id(`INSERT INTO misko.subjects (code, species, sex) VALUES ($1, 'MOUSE', 'MALE') RETURNING id`, fmt.Sprintf("M%d", i+1))
	}

	exp := c.create("/api/experiments", admin, map[string]any{"code": "EXP-1", "title": "Compound X in MPTP mice", "hypothesis": "X rescues motor deficits",
		"randomisation": "Block randomisation", "blinded": true})
	if exp["status"] != "DRAFT" || exp["dayZero"] != nil || exp["blinded"] != true {
		t.Fatalf("new experiment: %v", exp)
	}
	base := "/api/experiments/" + id(exp)
	if list := c.call("GET", "/api/experiments?status=DRAFT&search=exp-1", tokens["VIEWER"], nil).body; list["total"] != float64(1) {
		t.Fatalf("filtered list: %v", list)
	}
	c.expect("unknown status", c.call("GET", "/api/experiments?status=PAUSED", admin, nil), 400, "common.invalidQuery")

	// Groups: role and disease model.
	c.expect("healthy with model", c.call("POST", base+"/groups", admin, map[string]any{"code": "G0", "name": "Wrong", "role": "HEALTHY_CONTROL", "diseaseModelId": model}), 400, "group.diseaseModelNotAllowed")
	c.expect("treatment without model", c.call("POST", base+"/groups", admin, map[string]any{"code": "G0", "name": "Wrong", "role": "TREATMENT"}), 400, "group.diseaseModelRequired")
	c.expect("unknown model", c.call("POST", base+"/groups", admin, map[string]any{"code": "G0", "name": "Wrong", "role": "SHAM", "diseaseModelId": "00000000-0000-7000-8000-00000000abcd"}), 400, "group.unknownDiseaseModel")
	healthy := c.create(base+"/groups", admin, map[string]any{"code": "G1", "name": "Healthy", "role": "HEALTHY_CONTROL", "targetSize": 2})
	disease := c.create(base+"/groups", admin, map[string]any{"code": "G2", "name": "MPTP + vehicle", "role": "DISEASE_CONTROL", "diseaseModelId": model, "targetSize": 2})
	treated := c.create(base+"/groups", tokens["RESEARCHER"], map[string]any{"code": "G3", "name": "MPTP + X", "role": "TREATMENT", "diseaseModelId": model})
	c.expect("group code", c.call("POST", base+"/groups", admin, map[string]any{"code": "g1", "name": "Other", "role": "HEALTHY_CONTROL"}), 409, "group.codeTaken")
	if g := c.expect("target", c.call("PATCH", base+"/groups/"+id(treated), admin, map[string]any{"targetSize": 2}), 200, "").body; g["targetSize"] != float64(2) {
		t.Fatalf("target size: %v", g)
	}

	// Timepoints, protocol and endpoints.
	baseline := c.create(base+"/timepoints", admin, map[string]any{"name": "Baseline", "studyDay": -1, "position": 1})
	day14 := c.create(base+"/timepoints", admin, map[string]any{"name": "Day 14", "studyDay": 14, "position": 2})
	c.expect("timepoint position", c.call("POST", base+"/timepoints", admin, map[string]any{"name": "Other", "studyDay": 3, "position": 2}), 409, "timepoint.conflict")
	primary := map[string]any{"timepointId": id(day14), "baselineTimepointId": id(baseline), "paradigmKey": "OPEN_FIELD", "metricKey": "distance_cm",
		"kind": "PRIMARY", "direction": "HIGHER_IS_BETTER"}
	c.expect("metric before protocol", c.call("POST", base+"/endpoints", admin, primary), 400, "endpoint.metricUnavailable")
	version := sql.protocol(id(exp))
	options := c.expect("metric options", c.call("GET", base+"/metric-options", tokens["VIEWER"], nil), 200, "").body["data"].([]any)
	if len(options) != 1 || options[0].(map[string]any)["paradigmKey"] != "OPEN_FIELD" {
		t.Fatalf("metric options: %v", options)
	}
	endpoint := c.create(base+"/endpoints", admin, primary)
	c.expect("second primary", c.call("POST", base+"/endpoints", admin, map[string]any{"timepointId": id(day14), "paradigmKey": "OPEN_FIELD",
		"metricKey": "center_time_s", "kind": "PRIMARY", "direction": "HIGHER_IS_BETTER"}), 409, "endpoint.primaryTaken")
	c.expect("unknown metric", c.call("POST", base+"/endpoints", admin, map[string]any{"timepointId": id(day14), "paradigmKey": "OPEN_FIELD",
		"metricKey": "freezing_s", "kind": "SECONDARY", "direction": "LOWER_IS_BETTER"}), 400, "endpoint.metricUnavailable")
	c.expect("same baseline", c.call("PATCH", base+"/endpoints/"+id(endpoint), admin, map[string]any{"baselineTimepointId": id(day14)}), 400, "endpoint.baselineSameTimepoint")
	c.expect("timepoint in use", c.call("DELETE", base+"/timepoints/"+id(baseline), admin, nil), 409, "timepoint.inUse")

	// The start check lists every unmet rule.
	readiness := c.expect("readiness", c.call("GET", base+"/readiness", tokens["VIEWER"], nil), 200, "").body
	if readiness["ready"] != false || !hasRule(readiness["unmet"], "readiness.inductionItemMissing", id(treated)) || !hasRule(readiness["unmet"], "readiness.vehicleItemMissing", id(disease)) {
		t.Fatalf("readiness: %v", readiness)
	}
	notReady := c.expect("start too early", c.call("POST", base+"/start", admin, map[string]any{"dayZero": "2026-10-20"}), 422, "experiment.notReady").body
	if len(notReady["unmet"].([]any)) != len(readiness["unmet"].([]any)) {
		t.Fatalf("start and readiness disagree: %v %v", notReady, readiness)
	}

	// Regimen items.
	item := func(group map[string]any, purpose, substance, frequency string, start, end int) map[string]any {
		return map[string]any{"groupId": id(group), "purpose": purpose, "substanceId": substance, "amount": "20", "unit": "mg/kg",
			"route": "INTRAPERITONEAL", "frequency": frequency, "startDay": start, "endDay": end}
	}
	c.create(base+"/regimen-items", admin, item(disease, "INDUCTION", mptp, "ONCE", 0, 0))
	c.create(base+"/regimen-items", admin, item(disease, "VEHICLE", saline, "DAILY", 1, 14))
	c.create(base+"/regimen-items", admin, item(treated, "INDUCTION", mptp, "ONCE", 0, 0))
	treatment := c.create(base+"/regimen-items", admin, item(treated, "TREATMENT", compound, "DAILY", 1, 14))
	if treatment["amount"] != "20" || treatment["intervalDays"] != nil {
		t.Fatalf("regimen item: %v", treatment)
	}
	c.expect("second treatment", c.call("POST", base+"/regimen-items", admin, item(treated, "TREATMENT", compound, "DAILY", 1, 14)), 409, "regimen.treatmentTaken")
	c.expect("once over days", c.call("POST", base+"/regimen-items", admin, item(healthy, "VEHICLE", saline, "ONCE", 1, 3)), 400, "regimen.invalidDays")
	c.expect("unknown substance", c.call("POST", base+"/regimen-items", admin, item(healthy, "VEHICLE", id(healthy), "DAILY", 1, 3)), 400, "regimen.unknownSubstance")
	if changed := c.expect("dose", c.call("PATCH", base+"/regimen-items/"+id(treatment), admin, map[string]any{"amount": "12.5"}), 200, "").body; changed["amount"] != "12.5" {
		t.Fatalf("regimen patch: %v", changed)
	}
	if readiness := c.call("GET", base+"/readiness", admin, nil).body; readiness["ready"] != true {
		t.Fatalf("ready design: %v", readiness)
	}

	// Allocation in the draft: move, remove, randomise up to the targets.
	allocation := c.create(base+"/allocations", tokens["RESEARCHER"], map[string]any{"subjectId": animals[0], "groupId": id(healthy), "cage": "C1"})
	c.expect("allocated twice", c.call("POST", base+"/allocations", admin, map[string]any{"subjectId": animals[0], "groupId": id(disease)}), 409, "allocation.subjectAllocated")
	c.expect("unknown subject", c.call("POST", base+"/allocations", admin, map[string]any{"subjectId": id(healthy), "groupId": id(disease)}), 400, "allocation.unknownSubject")
	other := c.create("/api/experiments", admin, map[string]any{"code": "EXP-2", "title": "Other"})
	foreign := c.create("/api/experiments/"+id(other)+"/groups", admin, map[string]any{"code": "G1", "name": "Healthy", "role": "HEALTHY_CONTROL"})
	c.expect("foreign group", c.call("POST", base+"/allocations", admin, map[string]any{"subjectId": animals[1], "groupId": id(foreign)}), 400, "group.notInExperiment")
	if moved := c.expect("move", c.call("PATCH", base+"/allocations/"+id(allocation), admin, map[string]any{"groupId": id(disease)}), 200, "").body; moved["groupId"] != id(disease) {
		t.Fatalf("move: %v", moved)
	}
	spare := c.create(base+"/allocations", admin, map[string]any{"subjectId": animals[8], "groupId": id(healthy)})
	c.expect("remove", c.call("DELETE", base+"/allocations/"+id(spare), admin, nil), 204, "")
	c.expect("too many", c.call("POST", base+"/allocations/randomise", admin, map[string]any{"subjectIds": animals[1:7]}), 409, "allocation.notEnoughPlaces")
	randomised := c.expect("randomise", c.call("POST", base+"/allocations/randomise", admin, map[string]any{"subjectIds": animals[1:6], "cage": "C2"}), 201, "").body["data"].([]any)
	if len(randomised) != 5 {
		t.Fatalf("randomised: %v", randomised)
	}
	list := c.expect("allocations", c.call("GET", base+"/allocations", tokens["VIEWER"], nil), 200, "").body
	for _, g := range list["groups"].([]any) {
		if group := g.(map[string]any); group["allocated"] != float64(2) || group["excluded"] != float64(0) {
			t.Fatalf("group fill: %v", list["groups"])
		}
	}
	if data := list["data"].([]any); len(data) != 6 || data[0].(map[string]any)["subjectCode"] == nil {
		t.Fatalf("allocation list: %v", data)
	}
	c.expect("group in use", c.call("DELETE", base+"/groups/"+id(healthy), admin, nil), 409, "group.inUse")

	// Start: day 0 is set and the design freezes.
	c.expect("bad day zero", c.call("POST", base+"/start", admin, map[string]any{"dayZero": "20/10/2026"}), 400, "experiment.invalidDayZero")
	started := c.expect("start", c.call("POST", base+"/start", tokens["LAB_MANAGER"], map[string]any{"dayZero": "2026-10-20"}), 200, "").body
	if started["status"] != "RUNNING" || started["dayZero"] != "2026-10-20" || started["startedAt"] == nil {
		t.Fatalf("started: %v", started)
	}
	c.expect("start twice", c.call("POST", base+"/start", admin, map[string]any{"dayZero": "2026-10-20"}), 409, "experiment.notDraft")
	frozen := []struct {
		method, path string
		body         any
	}{
		{"POST", base + "/groups", map[string]any{"code": "G9", "name": "Late", "role": "HEALTHY_CONTROL"}},
		{"PATCH", base + "/groups/" + id(healthy), map[string]any{"targetSize": 3}},
		{"PATCH", base + "/regimen-items/" + id(treatment), map[string]any{"amount": "1"}},
		{"DELETE", base + "/timepoints/" + id(baseline), nil},
		{"POST", base + "/endpoints", map[string]any{"timepointId": id(day14), "paradigmKey": "OPEN_FIELD", "metricKey": "center_time_s", "kind": "SECONDARY", "direction": "HIGHER_IS_BETTER"}},
		{"PATCH", base, map[string]any{"blinded": false}},
	}
	for _, f := range frozen {
		c.expect("frozen "+f.method+" "+f.path, c.call(f.method, f.path, admin, f.body), 409, "experiment.designFrozen")
	}
	c.expect("rename running", c.call("PATCH", base, admin, map[string]any{"title": "Renamed"}), 200, "")
	c.expect("move running", c.call("PATCH", base+"/allocations/"+id(allocation), admin, map[string]any{"groupId": id(healthy)}), 409, "allocation.frozen")
	c.expect("remove running", c.call("DELETE", base+"/allocations/"+id(allocation), admin, nil), 409, "allocation.frozen")
	if caged := c.expect("cage", c.call("PATCH", base+"/allocations/"+id(allocation), admin, map[string]any{"cage": "C9"}), 200, "").body; caged["cage"] != "C9" {
		t.Fatalf("cage: %v", caged)
	}
	c.expect("bad reason", c.call("POST", base+"/allocations/"+id(allocation)+"/exclusion", admin, map[string]any{"reason": "LOST"}), 400, "allocation.invalidExclusionReason")
	excluded := c.expect("exclude", c.call("POST", base+"/allocations/"+id(allocation)+"/exclusion", admin, map[string]any{"reason": "DIED", "note": "Found dead"}), 200, "").body
	if excluded["excluded"] != true || excluded["exclusionReason"] != "DIED" {
		t.Fatalf("excluded: %v", excluded)
	}
	c.expect("exclude twice", c.call("POST", base+"/allocations/"+id(allocation)+"/exclusion", admin, map[string]any{"reason": "OTHER"}), 409, "allocation.alreadyExcluded")
	c.create(base+"/allocations", admin, map[string]any{"subjectId": animals[6], "groupId": id(disease)})
	if excludedOnly := c.call("GET", base+"/allocations?excluded=true", admin, nil).body["data"].([]any); len(excludedOnly) != 1 {
		t.Fatalf("excluded filter: %v", excludedOnly)
	}

	// RBAC on every route.
	writes := []struct {
		method, path string
		body         any
	}{
		{"PATCH", base, map[string]any{"title": "x"}},
		{"POST", base + "/start", map[string]any{"dayZero": "2026-10-20"}},
		{"POST", base + "/complete", nil},
		{"POST", base + "/groups", map[string]any{"code": "G9", "name": "x", "role": "HEALTHY_CONTROL"}},
		{"PATCH", base + "/groups/" + id(healthy), map[string]any{"name": "x"}},
		{"DELETE", base + "/groups/" + id(healthy), nil},
		{"POST", base + "/regimen-items", item(healthy, "VEHICLE", saline, "DAILY", 1, 3)},
		{"PATCH", base + "/regimen-items/" + id(treatment), map[string]any{"amount": "1"}},
		{"DELETE", base + "/regimen-items/" + id(treatment), nil},
		{"POST", base + "/timepoints", map[string]any{"name": "x", "studyDay": 1, "position": 9}},
		{"PATCH", base + "/timepoints/" + id(day14), map[string]any{"name": "x"}},
		{"DELETE", base + "/timepoints/" + id(day14), nil},
		{"POST", base + "/endpoints", primary},
		{"PATCH", base + "/endpoints/" + id(endpoint), map[string]any{"kind": "SECONDARY"}},
		{"DELETE", base + "/endpoints/" + id(endpoint), nil},
		{"POST", base + "/allocations", map[string]any{"subjectId": animals[7], "groupId": id(healthy)}},
		{"POST", base + "/allocations/randomise", map[string]any{"subjectIds": animals[7:8]}},
		{"PATCH", base + "/allocations/" + id(allocation), map[string]any{"cage": "x"}},
		{"DELETE", base + "/allocations/" + id(allocation), nil},
		{"POST", base + "/allocations/" + id(allocation) + "/exclusion", map[string]any{"reason": "DIED"}},
	}
	for _, role := range []string{"TECHNICIAN", "VIEWER"} {
		for _, w := range writes {
			c.expect(role+" "+w.method+" "+w.path, c.call(w.method, w.path, tokens[role], w.body), 403, "auth.forbidden")
		}
	}
	reads := []string{"/api/experiments", base, base + "/readiness", base + "/metric-options", base + "/groups", base + "/regimen-items",
		base + "/timepoints", base + "/endpoints", base + "/allocations"}
	for role, token := range tokens {
		for _, path := range reads {
			c.expect(role+" GET "+path, c.call("GET", path, token, nil), 200, "")
		}
	}
	for _, path := range reads {
		c.expect("anonymous GET "+path, c.call("GET", path, "", nil), 401, "auth.unauthenticated")
	}
	for _, path := range []string{"/api/experiments/not-a-uuid", "/api/experiments/00000000-0000-7000-8000-00000000abcd/groups"} {
		c.expect("missing "+path, c.call("GET", path, admin, nil), 404, "experiment.notFound")
	}
	c.expect("foreign group patch", c.call("PATCH", "/api/experiments/"+id(other)+"/groups/"+id(healthy), admin, map[string]any{"name": "X"}), 404, "group.notFound")

	// Completion waits for every test, then the experiment is read-only.
	test := sql.id(`INSERT INTO misko.tests (experiment_id, timepoint_id, protocol_version_id, step_position, paradigm_key, paradigm_version,
		planned_trials, scheduled_at, created_by) VALUES ($1, $2, $3, 1, 'OPEN_FIELD', 1, 1, now(), $4) RETURNING id`, id(exp), id(day14), version, sql.user())
	c.expect("complete with open test", c.call("POST", base+"/complete", admin, nil), 409, "experiment.testsOpen")
	sql.exec(`UPDATE misko.tests SET status = 'CANCELLED', cancelled_at = now(), cancel_reason = 'Camera failure' WHERE id = $1`, test)
	completed := c.expect("complete", c.call("POST", base+"/complete", admin, nil), 200, "").body
	if completed["status"] != "COMPLETED" || completed["completedAt"] == nil {
		t.Fatalf("completed: %v", completed)
	}
	c.expect("complete twice", c.call("POST", base+"/complete", admin, nil), 409, "experiment.notRunning")
	c.expect("rename completed", c.call("PATCH", base, admin, map[string]any{"title": "Late"}), 409, "experiment.completed")
	c.expect("allocate completed", c.call("POST", base+"/allocations", admin, map[string]any{"subjectId": animals[7], "groupId": id(healthy)}), 409, "allocation.frozen")
	c.expect("cage completed", c.call("PATCH", base+"/allocations/"+id(allocation), admin, map[string]any{"cage": "C1"}), 409, "allocation.frozen")
	c.expect("complete draft", c.call("POST", "/api/experiments/"+id(other)+"/complete", admin, nil), 409, "experiment.notRunning")
}

func hasRule(unmet any, rule, groupID string) bool {
	for _, p := range unmet.([]any) {
		if problem := p.(map[string]any); problem["rule"] == rule && problem["groupId"] == groupID {
			return true
		}
	}
	return false
}

// sqlRunner writes rows that belong to other contexts directly.
type sqlRunner struct {
	t    *testing.T
	pool *pgxpool.Pool
}

func (s sqlRunner) id(query string, args ...any) string {
	s.t.Helper()
	var id string
	if err := s.pool.QueryRow(context.Background(), query, args...).Scan(&id); err != nil {
		s.t.Fatalf("%s: %v", query, err)
	}
	return id
}

func (s sqlRunner) exec(query string, args ...any) {
	s.t.Helper()
	if _, err := s.pool.Exec(context.Background(), query, args...); err != nil {
		s.t.Fatalf("%s: %v", query, err)
	}
}

func (s sqlRunner) user() string {
	return s.id(`SELECT id FROM misko.users ORDER BY created_at LIMIT 1`)
}

// protocol stores a one-step OPEN_FIELD protocol and returns its version id.
func (s sqlRunner) protocol(experimentID string) string {
	s.t.Helper()
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		s.t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var protocol, version string
	if err := tx.QueryRow(ctx, `INSERT INTO misko.protocols (experiment_id, name) VALUES ($1, 'Battery') RETURNING id`, experimentID).Scan(&protocol); err != nil {
		s.t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO misko.protocol_versions (experiment_id, protocol_id, number, step_count, created_by)
		VALUES ($1, $2, 1, 1, $3) RETURNING id`, experimentID, protocol, s.user()).Scan(&version); err != nil {
		s.t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO misko.protocol_steps (protocol_version_id, position, paradigm_key, paradigm_version, trial_type, trials, session)
		VALUES ($1, 1, 'OPEN_FIELD', 1, 'STANDARD', 1, '{}')`, version); err != nil {
		s.t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		s.t.Fatal(err)
	}
	return version
}

func id(record map[string]any) string { return record["id"].(string) }

type client struct {
	t    *testing.T
	url  string
	http *http.Client
}

type response struct {
	status int
	body   map[string]any
}

func (c client) call(method, path, token string, body any) response {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, c.url+path, reader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	var decoded map[string]any
	_ = json.NewDecoder(res.Body).Decode(&decoded)
	return response{res.StatusCode, decoded}
}

func (c client) expect(label string, r response, status int, code string) response {
	c.t.Helper()
	if r.status != status || (code != "" && r.body["code"] != code) {
		c.t.Fatalf("%s: status=%d body=%v want %d %s", label, r.status, r.body, status, code)
	}
	return r
}

func (c client) create(path, token string, body any) map[string]any {
	c.t.Helper()
	r := c.expect("POST "+path, c.call("POST", path, token, body), 201, "")
	if user, ok := r.body["user"].(map[string]any); ok {
		return user
	}
	return r.body
}

func (c client) login(email, password string) string {
	c.t.Helper()
	r := c.expect("login "+email, c.call("POST", "/api/auth/login", "", map[string]string{"email": email, "password": password}), 200, "")
	return r.body["token"].(string)
}
