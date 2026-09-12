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

// TestInterventionsOverHTTP checks the role-by-operation matrix for plans,
// weights, conditions and administrations, and the records' behavior end to end.
func TestInterventionsOverHTTP(t *testing.T) {
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
	roles := []string{"SUPERADMIN", "LAB_MANAGER", "RESEARCHER", "TECHNICIAN", "VIEWER"}
	tokens := map[string]string{"SUPERADMIN": admin}
	for _, role := range roles[1:] {
		email := strings.ToLower(role) + "@lab.io"
		c.create("/api/users", admin, map[string]any{"email": email, "name": role, "role": role, "password": "Password-123"})
		tokens[role] = c.login(email, "Password-123")
	}

	subject := c.create("/api/subjects", admin, map[string]any{"code": "M-1", "species": "MOUSE", "sex": "MALE"})
	exp := c.create("/api/experiments", admin, map[string]any{"code": "EXP-1", "title": "Stroke study"})
	base := "/api/experiments/" + id(exp)
	phase := c.create(base+"/phases", admin, map[string]any{"name": "Treatment", "position": 1})
	treated := c.create(base+"/groups", admin, map[string]any{"name": "Drug", "role": "TREATMENT"})
	enrolledAt := time.Now().UTC().Add(-72 * time.Hour).Format(time.RFC3339)
	enrollment := c.create(base+"/enrollments", admin, map[string]any{"subjectId": id(subject), "groupId": id(treated), "enrolledAt": enrolledAt})
	substance := c.create("/api/substances", admin, map[string]any{"name": "Minocycline"})
	model := c.create("/api/disease-models", admin, map[string]any{"name": "MCAO stroke"})
	plan := c.create(base+"/intervention-plans", admin, map[string]any{
		"groupId": id(treated), "phaseId": id(phase), "substanceId": id(substance), "amount": "10", "unit": "mg/kg", "route": "INTRAPERITONEAL", "schedule": "daily for 7 days",
	})
	if total := c.call("GET", "/api/administrations", admin, nil).body["total"]; total != float64(0) {
		t.Fatalf("creating a plan recorded administrations: %v", total)
	}
	now := time.Now().UTC()
	at := func(ago time.Duration) string { return now.Add(-ago).Format(time.RFC3339) }

	permitted := map[string]map[string]bool{
		"study:write":  {"SUPERADMIN": true, "LAB_MANAGER": true, "RESEARCHER": true},
		"weight:write": {"SUPERADMIN": true, "LAB_MANAGER": true, "RESEARCHER": true, "TECHNICIAN": true},
		"test:run":     {"SUPERADMIN": true, "LAB_MANAGER": true, "RESEARCHER": true, "TECHNICIAN": true},
	}
	for i, role := range roles {
		token := tokens[role]
		expect := func(permission, label string, r response) {
			t.Helper()
			if permitted[permission][role] {
				c.expect(role+" "+label, r, 201, "")
			} else {
				c.expect(role+" "+label, r, 403, "auth.forbidden")
			}
		}
		expect("study:write", "creates disease model", c.call("POST", "/api/disease-models", token, map[string]any{"name": "Model " + role}))
		expect("study:write", "creates substance", c.call("POST", "/api/substances", token, map[string]any{"name": "Substance " + role}))
		expect("study:write", "creates plan", c.call("POST", base+"/intervention-plans", token, map[string]any{
			"groupId": id(treated), "substanceId": id(substance), "amount": "1", "unit": "mg", "route": "ORAL", "schedule": "once",
		}))
		weight := c.call("POST", "/api/subjects/"+id(subject)+"/weights", token, map[string]any{"grams": "25", "measuredAt": at(time.Duration(i+1) * time.Minute)})
		expect("weight:write", "records weight", weight)
		expect("test:run", "records condition", c.call("POST", "/api/subjects/"+id(subject)+"/conditions", token, map[string]any{
			"diseaseModelId": id(model), "status": "INDUCED", "observedAt": at(48 * time.Hour),
		}))
		expect("test:run", "records administration", c.call("POST", base+"/enrollments/"+id(enrollment)+"/administrations", token, map[string]any{
			"substanceId": id(substance), "amount": "0.2", "unit": "mL", "route": "ORAL", "administeredAt": at(time.Hour),
		}))
		c.expect(role+" updates plan", c.call("PATCH", base+"/intervention-plans/"+id(plan), token, map[string]any{"schedule": "daily"}), map[bool]int{true: 200, false: 403}[permitted["study:write"][role]], "")
		for _, path := range []string{
			"/api/disease-models", "/api/substances", base + "/intervention-plans", "/api/subjects/" + id(subject) + "/weights",
			"/api/subjects/" + id(subject) + "/conditions", "/api/conditions", "/api/subjects/" + id(subject) + "/administrations", "/api/administrations",
		} {
			c.expect(role+" GET "+path, c.call("GET", path, token, nil), 200, "")
			if role == "VIEWER" {
				c.expect("anonymous GET "+path, c.call("GET", path, "", nil), 401, "auth.unauthenticated")
			}
		}
	}

	weight := c.create("/api/subjects/"+id(subject)+"/weights", tokens["TECHNICIAN"], map[string]any{"grams": "25", "measuredAt": at(30 * time.Minute)})
	given := c.create(base+"/enrollments/"+id(enrollment)+"/administrations", tokens["TECHNICIAN"], map[string]any{
		"substanceId": id(substance), "planId": id(plan), "weightMeasurementId": id(weight), "amount": "10", "unit": "mg/kg", "route": "INTRAPERITONEAL", "administeredAt": at(10 * time.Minute),
	})
	abs, _ := given["absoluteDose"].(map[string]any)
	if abs["amount"] != "0.25" || abs["unit"] != "mg" || given["bodyWeightGrams"] != "25" || given["substanceName"] != "Minocycline" {
		t.Fatalf("per-kg administration: %v", given)
	}
	c.expect("per-kg without weight", c.call("POST", base+"/enrollments/"+id(enrollment)+"/administrations", admin, map[string]any{
		"substanceId": id(substance), "amount": "10", "unit": "mg/kg", "route": "INTRAPERITONEAL", "administeredAt": at(time.Minute),
	}), 400, "administration.weightRequired")
	c.expect("invalid amount", c.call("POST", base+"/enrollments/"+id(enrollment)+"/administrations", admin, map[string]any{
		"substanceId": id(substance), "amount": "1e3", "unit": "mg", "route": "ORAL", "administeredAt": at(time.Minute),
	}), 400, "dose.invalidAmount")
	c.expect("future administration", c.call("POST", base+"/enrollments/"+id(enrollment)+"/administrations", admin, map[string]any{
		"substanceId": id(substance), "amount": "1", "unit": "mg", "route": "ORAL", "administeredAt": now.Add(time.Hour).Format(time.RFC3339),
	}), 400, "common.futureTime")

	c.create("/api/subjects/"+id(subject)+"/conditions", admin, map[string]any{"diseaseModelId": id(model), "status": "CONFIRMED", "observedAt": at(24 * time.Hour)})
	if page := c.call("GET", "/api/conditions?diseaseModelId="+id(model)+"&status=INDUCED&current=true", tokens["VIEWER"], nil).body; page["total"] != float64(0) {
		t.Fatalf("superseded induction reported as current: %v", page)
	}
	if page := c.call("GET", "/api/conditions?diseaseModelId="+id(model)+"&status=CONFIRMED&current=true", tokens["VIEWER"], nil).body; page["total"] != float64(1) {
		t.Fatalf("current confirmation: %v", page)
	}
	if page := c.call("GET", "/api/subjects/"+id(subject)+"/administrations?substanceId="+id(substance), tokens["VIEWER"], nil).body; page["total"] != float64(5) {
		t.Fatalf("subject administration history: %v", page["total"])
	}
	c.expect("bad range", c.call("GET", "/api/administrations?from=yesterday", admin, nil), 400, "common.invalidQuery")
	c.expect("phase used by plan", c.call("DELETE", base+"/phases/"+id(phase), admin, nil), 409, "phase.inUse")
	c.expect("group used by plan", c.call("DELETE", base+"/groups/"+id(treated), admin, nil), 409, "group.inUse")
}
