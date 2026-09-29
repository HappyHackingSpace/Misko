//go:build integration

package bootstrap

import (
	"context"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/config"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
	"golang.org/x/crypto/bcrypt"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestTestsTrialsAndCommentsOverHTTP runs the test lifecycle, trial repeats and
// comment ownership for every role through the composed API.
func TestTestsTrialsAndCommentsOverHTTP(t *testing.T) {
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
	enrolledAt := time.Now().UTC().Add(-48 * time.Hour)
	stamp := func(d time.Duration) string { return time.Now().UTC().Add(d).Format(time.RFC3339Nano) }

	subject := c.create("/api/subjects", admin, map[string]any{"code": "M-1", "species": "MOUSE", "sex": "MALE"})
	exp := c.create("/api/experiments", admin, map[string]any{"code": "EXP-T", "title": "Anxiety"})
	base := "/api/experiments/" + id(exp)
	phase := c.create(base+"/phases", admin, map[string]any{"name": "Baseline", "position": 1})
	group := c.create(base+"/groups", admin, map[string]any{"name": "Vehicle", "role": "CONTROL"})
	enrollment := c.create(base+"/enrollments", admin, map[string]any{"subjectId": id(subject), "groupId": id(group), "enrolledAt": enrolledAt.Format(time.RFC3339)})
	arena := c.create("/api/environments", admin, map[string]any{"name": "Arena", "paradigmKey": "OPEN_FIELD",
		"revision": map[string]any{"paradigmVersion": 1, "apparatus": map[string]any{"arena_width_cm": 50, "arena_height_cm": 50, "center_fraction": 0.5}}})
	revision := arena["revision"].(map[string]any)
	protocol := c.create(base+"/protocols", admin, map[string]any{"name": "Open field", "version": map[string]any{"steps": []any{map[string]any{
		"position": 1, "paradigmKey": "OPEN_FIELD", "paradigmVersion": 1, "environmentRevisionId": id(revision), "trialType": "STANDARD", "trials": 2,
	}}}})
	version := protocol["version"].(map[string]any)
	plan := map[string]any{"enrollmentId": id(enrollment), "phaseId": id(phase), "protocolVersionId": id(version), "stepPosition": 1, "scheduledAt": stamp(-time.Hour)}

	testWriters := map[string]bool{"SUPERADMIN": true, "LAB_MANAGER": true, "RESEARCHER": true}
	runners := map[string]bool{"SUPERADMIN": true, "LAB_MANAGER": true, "RESEARCHER": true, "TECHNICIAN": true}
	planned := map[string]map[string]any{}
	for role, token := range tokens {
		if testWriters[role] {
			planned[role] = c.create(base+"/tests", token, plan)
			continue
		}
		c.expect(role+" creates test", c.call("POST", base+"/tests", token, plan), 403, "auth.forbidden")
	}
	test := planned["RESEARCHER"]
	if test["status"] != "PLANNED" || test["subjectId"] != id(subject) || test["groupId"] != id(group) || test["paradigmKey"] != "OPEN_FIELD" ||
		test["environmentRevisionId"] != id(revision) || test["plannedTrials"] != float64(2) || test["startedAt"] != nil {
		t.Fatalf("planned test: %v", test)
	}
	path := "/api/tests/" + id(test)
	for role, token := range tokens {
		if !runners[role] {
			c.expect(role+" starts test", c.call("POST", path+"/start", token, map[string]any{}), 403, "auth.forbidden")
			c.expect(role+" records trial", c.call("POST", path+"/trials", token, map[string]any{"repetition": 1, "startedAt": stamp(0)}), 403, "auth.forbidden")
		}
		if !testWriters[role] {
			c.expect(role+" cancels test", c.call("POST", path+"/cancel", token, map[string]any{"reason": "no"}), 403, "auth.forbidden")
		}
	}

	started := c.expect("technician starts", c.call("POST", path+"/start", tokens["TECHNICIAN"], map[string]any{"startedAt": stamp(-30 * time.Minute)}), 200, "").body
	if started["status"] != "IN_PROGRESS" || started["startedAt"] == nil {
		t.Fatalf("started: %v", started)
	}
	c.expect("start twice", c.call("POST", path+"/start", tokens["TECHNICIAN"], map[string]any{}), 409, "test.invalidTransition")
	c.expect("complete without trials", c.call("POST", path+"/complete", tokens["TECHNICIAN"], map[string]any{}), 409, "test.noTrials")
	first := c.create(path+"/trials", tokens["TECHNICIAN"], map[string]any{"repetition": 1, "startedAt": stamp(-25 * time.Minute), "endedAt": stamp(-20 * time.Minute)})
	repeat := c.create(path+"/trials", tokens["TECHNICIAN"], map[string]any{"repetition": 1, "startedAt": stamp(-15 * time.Minute), "notes": "subject jumped out"})
	if first["attempt"] != float64(1) || repeat["attempt"] != float64(2) || repeat["number"] != float64(2) {
		t.Fatalf("repeat must add an attempt: %v %v", first, repeat)
	}
	c.expect("repetition beyond plan", c.call("POST", path+"/trials", tokens["TECHNICIAN"], map[string]any{"repetition": 3, "startedAt": stamp(-time.Minute)}), 400, "trial.invalidRepetition")
	c.expect("trial before the test", c.call("POST", path+"/trials", tokens["TECHNICIAN"], map[string]any{"repetition": 2, "startedAt": stamp(-time.Hour)}), 400, "trial.beforeTest")
	completed := c.expect("technician completes", c.call("POST", path+"/complete", tokens["TECHNICIAN"], map[string]any{}), 200, "").body
	if completed["status"] != "COMPLETED" || completed["completedAt"] == nil {
		t.Fatalf("completed: %v", completed)
	}
	c.expect("trial after completion", c.call("POST", path+"/trials", tokens["TECHNICIAN"], map[string]any{"repetition": 2, "startedAt": stamp(0)}), 409, "test.notInProgress")
	c.expect("cancel a completed test", c.call("POST", path+"/cancel", tokens["RESEARCHER"], map[string]any{"reason": "mistake"}), 409, "test.invalidTransition")
	cancelled := c.expect("researcher cancels", c.call("POST", "/api/tests/"+id(planned["LAB_MANAGER"])+"/cancel", tokens["RESEARCHER"], map[string]any{"reason": "duplicate"}), 200, "").body
	if cancelled["status"] != "CANCELLED" || cancelled["cancelReason"] != "duplicate" {
		t.Fatalf("cancelled: %v", cancelled)
	}

	for role, token := range tokens {
		detail := c.expect(role+" reads test", c.call("GET", path, token, nil), 200, "").body
		if trials := detail["trials"].([]any); len(trials) != 2 || detail["status"] != "COMPLETED" {
			t.Fatalf("%s detail: %v", role, detail)
		}
		for _, read := range []string{path + "/trials", path + "/comments", base + "/tests"} {
			c.expect(role+" GET "+read, c.call("GET", read, token, nil), 200, "")
		}
	}
	for _, read := range []string{path, path + "/trials", path + "/comments", base + "/tests"} {
		c.expect("anonymous GET "+read, c.call("GET", read, "", nil), 401, "auth.unauthenticated")
	}
	if page := c.call("GET", base+"/tests?status=COMPLETED", tokens["VIEWER"], nil).body; page["total"] != float64(1) {
		t.Fatalf("status filter: %v", page)
	}
	c.expect("unknown status", c.call("GET", base+"/tests?status=DONE", tokens["VIEWER"], nil), 400, "common.invalidQuery")
	// The worklist across experiments sees the same tests and takes the same filters.
	all := c.expect("viewer lists all tests", c.call("GET", "/api/tests", tokens["VIEWER"], nil), 200, "").body
	scoped := c.call("GET", base+"/tests", tokens["VIEWER"], nil).body
	if all["total"] != scoped["total"] {
		t.Fatalf("all tests: %v, experiment tests: %v", all["total"], scoped["total"])
	}
	if page := c.call("GET", "/api/tests?status=COMPLETED&experimentId="+id(exp), tokens["VIEWER"], nil).body; page["total"] != float64(1) {
		t.Fatalf("all tests status filter: %v", page)
	}
	c.expect("anonymous GET /api/tests", c.call("GET", "/api/tests", "", nil), 401, "auth.unauthenticated")
	c.expect("all tests unknown status", c.call("GET", "/api/tests?status=DONE", tokens["VIEWER"], nil), 400, "common.invalidQuery")
	foreign := c.create("/api/experiments", admin, map[string]any{"code": "EXP-F", "title": "Other"})
	c.expect("enrollment of another experiment", c.call("POST", "/api/experiments/"+id(foreign)+"/tests", admin, plan), 404, "enrollment.notFound")

	// Comments: every role may comment, only the author edits, the author or a lab manager deletes.
	comment := c.create(path+"/comments", tokens["VIEWER"], map[string]any{"body": "Calm during the first trial"})
	if comment["authorName"] != "VIEWER" || comment["edited"] != false {
		t.Fatalf("viewer comment: %v", comment)
	}
	commentPath := path + "/comments/" + id(comment)
	c.expect("technician edits viewer comment", c.call("PATCH", commentPath, tokens["TECHNICIAN"], map[string]any{"body": "changed"}), 403, "comment.cannotEdit")
	c.expect("admin edits viewer comment", c.call("PATCH", commentPath, admin, map[string]any{"body": "changed"}), 403, "comment.cannotEdit")
	edited := c.expect("viewer edits", c.call("PATCH", commentPath, tokens["VIEWER"], map[string]any{"body": "Calm during both trials"}), 200, "").body
	if edited["body"] != "Calm during both trials" || edited["edited"] != true {
		t.Fatalf("edited: %v", edited)
	}
	other := "/api/tests/" + id(planned["SUPERADMIN"])
	c.expect("comment through another test", c.call("PATCH", other+"/comments/"+id(comment), tokens["VIEWER"], map[string]any{"body": "x"}), 404, "comment.notFound")
	c.expect("researcher deletes viewer comment", c.call("DELETE", commentPath, tokens["RESEARCHER"], nil), 403, "comment.cannotDelete")
	c.expect("anonymous comment", c.call("POST", path+"/comments", "", map[string]any{"body": "hi"}), 401, "auth.unauthenticated")
	c.expect("empty comment", c.call("POST", path+"/comments", tokens["TECHNICIAN"], map[string]any{"body": " "}), 400, "comment.invalid")
	if code := c.call("DELETE", commentPath, tokens["LAB_MANAGER"], nil).status; code != 204 {
		t.Fatalf("lab manager delete: %d", code)
	}
	if list := c.call("GET", path+"/comments", tokens["VIEWER"], nil).body["data"].([]any); len(list) != 0 {
		t.Fatalf("after delete: %v", list)
	}
	// The viewer used 3 of 20 comment changes this minute: one create and two edits.
	for i := range 17 {
		c.expect("comment within the limit", c.call("POST", other+"/comments", tokens["VIEWER"], map[string]any{"body": "note " + string(rune('a'+i))}), 201, "")
	}
	c.expect("comment over the limit", c.call("POST", other+"/comments", tokens["VIEWER"], map[string]any{"body": "one more"}), 429, "comment.rateLimited")
	c.expect("another user is not limited", c.call("POST", other+"/comments", tokens["TECHNICIAN"], map[string]any{"body": "fine"}), 201, "")
}
