//go:build integration

package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/config"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
	"golang.org/x/crypto/bcrypt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestSubjectsAndExperimentsOverHTTP checks read/write RBAC for every role and
// the enrollment invariants through the composed API.
func TestSubjectsAndExperimentsOverHTTP(t *testing.T) {
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
		c.expect(role+" creates subject", c.call("POST", "/api/subjects", token, map[string]any{"code": "S-" + role, "species": "MOUSE", "sex": "FEMALE"}), status, code)
		c.expect(role+" creates experiment", c.call("POST", "/api/experiments", token, map[string]any{"code": "E-" + role, "title": "Study"}), status, code)
	}

	subject := c.create("/api/subjects", admin, map[string]any{"code": "F-001", "species": "MOUSE", "sex": "FEMALE", "strain": "C57BL/6J", "birthDate": "2026-05-01"})
	if subject["birthDate"] != "2026-05-01" || subject["notes"] != nil {
		t.Fatalf("subject representation: %v", subject)
	}
	spare := c.create("/api/subjects", admin, map[string]any{"code": "F-002", "species": "MOUSE", "sex": "FEMALE"})
	exp := c.create("/api/experiments", admin, map[string]any{"code": "EXP-1", "title": "Anxiety", "requiresControl": true})
	base := "/api/experiments/" + id(exp)
	if detail := c.expect("detail", c.call("GET", base, admin, nil), 200, "").body; detail["controlRequirementMet"] != false {
		t.Fatalf("control requirement before groups: %v", detail)
	}
	baseline := c.create(base+"/phases", admin, map[string]any{"name": "Baseline", "position": 1})
	control := c.create(base+"/groups", admin, map[string]any{"name": "Vehicle", "role": "CONTROL", "targetSize": 1})
	treated := c.create(base+"/groups", admin, map[string]any{"name": "Drug", "role": "TREATMENT", "targetSize": 1})
	if detail := c.expect("detail", c.call("GET", base, admin, nil), 200, "").body; detail["controlRequirementMet"] != true {
		t.Fatalf("control requirement with control group: %v", detail)
	}
	enrolledAt := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339)
	enrollment := c.create(base+"/enrollments", tokens["RESEARCHER"], map[string]any{"subjectId": id(subject), "groupId": id(treated), "enrolledAt": enrolledAt})
	if enrollment["currentGroupId"] != id(treated) {
		t.Fatalf("initial group: %v", enrollment)
	}

	writes := []struct {
		method, path string
		body         any
	}{
		{"PATCH", "/api/subjects/" + id(subject), map[string]any{"notes": "moved"}},
		{"DELETE", "/api/subjects/" + id(spare), nil},
		{"PATCH", base, map[string]any{"title": "Renamed"}},
		{"POST", base + "/phases", map[string]any{"name": "Post", "position": 2}},
		{"PATCH", base + "/phases/" + id(baseline), map[string]any{"name": "Renamed"}},
		{"DELETE", base + "/phases/" + id(baseline), nil},
		{"POST", base + "/groups", map[string]any{"name": "Extra", "role": "TREATMENT"}},
		{"PATCH", base + "/groups/" + id(control), map[string]any{"targetSize": 5}},
		{"DELETE", base + "/groups/" + id(control), nil},
		{"POST", base + "/enrollments", map[string]any{"subjectId": id(spare), "enrolledAt": enrolledAt}},
		{"POST", base + "/enrollments/" + id(enrollment) + "/assignments", map[string]any{"groupId": id(control), "effectiveFrom": enrolledAt}},
	}
	for _, role := range []string{"TECHNICIAN", "VIEWER"} {
		for _, w := range writes {
			c.expect(role+" "+w.method+" "+w.path, c.call(w.method, w.path, tokens[role], w.body), 403, "auth.forbidden")
		}
	}
	reads := []string{
		"/api/subjects", "/api/subjects/" + id(subject), "/api/subjects/" + id(subject) + "/enrollments",
		"/api/experiments", base, base + "/phases", base + "/groups", base + "/enrollments", base + "/enrollments/" + id(enrollment),
	}
	for role, token := range tokens {
		for _, path := range reads {
			c.expect(role+" GET "+path, c.call("GET", path, token, nil), 200, "")
		}
	}
	for _, path := range reads {
		c.expect("anonymous GET "+path, c.call("GET", path, "", nil), 401, "auth.unauthenticated")
	}
	if phases := c.call("GET", base+"/phases", admin, nil).body["data"].([]any); len(phases) != 1 {
		t.Fatalf("denied writes changed phases: %v", phases)
	}

	c.expect("duplicate enrollment", c.call("POST", base+"/enrollments", admin, map[string]any{"subjectId": id(subject), "enrolledAt": enrolledAt}), 409, "enrollment.duplicate")
	second := c.create("/api/experiments", admin, map[string]any{"code": "EXP-2", "title": "Follow-up"})
	foreign := c.create("/api/experiments/"+id(second)+"/groups", admin, map[string]any{"name": "Vehicle", "role": "CONTROL"})
	c.expect("foreign group", c.call("POST", base+"/enrollments", admin, map[string]any{"subjectId": id(spare), "groupId": id(foreign), "enrolledAt": enrolledAt}), 400, "assignment.groupNotInExperiment")
	c.expect("foreign phase", c.call("PATCH", "/api/experiments/"+id(second)+"/phases/"+id(baseline), admin, map[string]any{"name": "X"}), 404, "phase.notFound")

	sizes := func() map[string][2]any {
		out := map[string][2]any{}
		for _, g := range c.call("GET", base+"/groups", admin, nil).body["data"].([]any) {
			group := g.(map[string]any)
			out[group["name"].(string)] = [2]any{group["targetSize"], group["activeSubjects"]}
		}
		return out
	}
	if got := sizes(); got["Vehicle"] != [2]any{float64(1), float64(0)} || got["Drug"] != [2]any{float64(1), float64(1)} {
		t.Fatalf("target versus actual: %v", got)
	}
	crossover := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	c.create(base+"/enrollments/"+id(enrollment)+"/assignments", admin, map[string]any{"groupId": id(control), "effectiveFrom": crossover})
	if got := sizes(); got["Vehicle"] != [2]any{float64(1), float64(1)} || got["Drug"] != [2]any{float64(1), float64(0)} {
		t.Fatalf("after crossover: %v", got)
	}
	detail := c.expect("enrollment detail", c.call("GET", base+"/enrollments/"+id(enrollment), admin, nil), 200, "").body
	if history := detail["assignments"].([]any); len(history) != 2 || detail["currentGroupId"] != id(control) || history[0].(map[string]any)["validTo"] == nil {
		t.Fatalf("assignment history: %v", detail)
	}
	c.expect("same group", c.call("POST", base+"/enrollments/"+id(enrollment)+"/assignments", admin, map[string]any{"groupId": id(control), "effectiveFrom": time.Now().UTC().Format(time.RFC3339)}), 409, "assignment.alreadyInGroup")
	c.create("/api/experiments/"+id(second)+"/enrollments", admin, map[string]any{"subjectId": id(subject), "enrolledAt": enrolledAt})
	if list := c.call("GET", "/api/subjects/"+id(subject)+"/enrollments", tokens["VIEWER"], nil).body["data"].([]any); len(list) != 2 {
		t.Fatalf("subject enrollments: %v", list)
	}
	c.expect("group in use", c.call("DELETE", base+"/groups/"+id(control), admin, nil), 409, "group.inUse")
	c.expect("enrolled subject", c.call("DELETE", "/api/subjects/"+id(subject), admin, nil), 409, "subject.inUse")
	c.expect("unknown sort", c.call("GET", "/api/subjects?sort=notes", admin, nil), 400, "common.invalidQuery")
	c.expect("code ignoring case", c.call("POST", "/api/subjects", admin, map[string]any{"code": "f-001", "species": "RAT", "sex": "MALE"}), 409, "subject.codeTaken")
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
