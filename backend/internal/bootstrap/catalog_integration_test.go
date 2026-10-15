//go:build integration

package bootstrap

import (
	"context"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/config"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// wp1Roles lists every role in a fixed order.
var wp1Roles = []string{"SUPERADMIN", "LAB_MANAGER", "RESEARCHER", "TECHNICIAN", "VIEWER"}

// newWP1Client composes the API on a fresh database and signs in one user
// per role.
func newWP1Client(t *testing.T) (client, map[string]string, *pgxpool.Pool) {
	t.Helper()
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
	t.Cleanup(server.Close)
	c := client{t: t, url: server.URL, http: server.Client()}
	admin := c.login("admin@lab.io", setup.AdminPassword)
	tokens := map[string]string{"SUPERADMIN": admin}
	for _, role := range wp1Roles[1:] {
		email := strings.ToLower(role) + "@lab.io"
		c.create("/api/users", admin, map[string]any{"email": email, "name": role, "role": role, "password": "Password-123"})
		tokens[role] = c.login(email, "Password-123")
	}
	return c, tokens, pool
}

// TestCatalogOverHTTP checks the role matrix, validation codes, filters and
// the in-use protection of substances and disease models.
func TestCatalogOverHTTP(t *testing.T) {
	ctx := context.Background()
	c, tokens, pool := newWP1Client(t)
	admin := tokens["SUPERADMIN"]
	studyWriters := map[string]bool{"SUPERADMIN": true, "LAB_MANAGER": true, "RESEARCHER": true}

	for _, role := range wp1Roles {
		status, code := 403, "auth.forbidden"
		if studyWriters[role] {
			status, code = 201, ""
		}
		c.expect(role+" creates substance", c.call("POST", "/api/catalog/substances", tokens[role], map[string]any{"name": "Substance " + role, "kind": "COMPOUND"}), status, code)
		c.expect(role+" creates disease model", c.call("POST", "/api/catalog/disease-models", tokens[role], map[string]any{"name": "Model " + role, "inductionType": "OTHER"}), status, code)
	}

	saline := c.create("/api/catalog/substances", tokens["RESEARCHER"], map[string]any{"name": " Saline 0.9% ", "kind": "VEHICLE", "description": "sterile"})
	if saline["name"] != "Saline 0.9%" || saline["kind"] != "VEHICLE" || saline["description"] != "sterile" {
		t.Fatalf("substance representation: %v", saline)
	}
	mptp := c.create("/api/catalog/substances", admin, map[string]any{"name": "MPTP", "kind": "INDUCER"})
	if v, ok := mptp["description"]; !ok || v != nil {
		t.Fatalf("missing description is not null: %v", mptp)
	}
	model := c.create("/api/catalog/disease-models", admin, map[string]any{
		"name": "MPTP parkinsonism", "inductionType": "CHEMICAL", "confirmationCriterion": "pole descent time > 150% of baseline",
	})
	if model["inductionType"] != "CHEMICAL" || model["confirmationCriterion"] != "pole descent time > 150% of baseline" || model["description"] != nil {
		t.Fatalf("disease model representation: %v", model)
	}
	stroke := c.create("/api/catalog/disease-models", admin, map[string]any{"name": "MCAO stroke", "inductionType": "SURGICAL"})

	for _, tc := range []struct {
		label, method, path string
		body                any
		status              int
		code                string
	}{
		{"missing kind", "POST", "/api/catalog/substances", map[string]any{"name": "X"}, 400, "substance.invalidKind"},
		{"unknown kind", "POST", "/api/catalog/substances", map[string]any{"name": "X", "kind": "SOLVENT"}, 400, "substance.invalidKind"},
		{"empty name", "POST", "/api/catalog/substances", map[string]any{"name": " ", "kind": "COMPOUND"}, 400, "common.invalidName"},
		{"long description", "POST", "/api/catalog/substances", map[string]any{"name": "X", "kind": "COMPOUND", "description": strings.Repeat("d", 5001)}, 400, "common.invalidDescription"},
		{"unknown field", "POST", "/api/catalog/substances", map[string]any{"name": "X", "kind": "COMPOUND", "dose": 1}, 400, "common.invalidJSON"},
		{"name taken", "POST", "/api/catalog/substances", map[string]any{"name": "mptp", "kind": "COMPOUND"}, 409, "catalog.nameTaken"},
		{"genetic model", "POST", "/api/catalog/disease-models", map[string]any{"name": "Tg", "inductionType": "GENETIC"}, 400, "diseaseModel.invalidInductionType"},
		{"long criterion", "POST", "/api/catalog/disease-models", map[string]any{"name": "Y", "inductionType": "DIET", "confirmationCriterion": strings.Repeat("c", 2001)}, 400, "diseaseModel.invalidConfirmationCriterion"},
		{"model name taken", "POST", "/api/catalog/disease-models", map[string]any{"name": "mcao STROKE", "inductionType": "SURGICAL"}, 409, "catalog.nameTaken"},
		{"empty patch", "PATCH", "/api/catalog/substances/" + id(mptp), map[string]any{}, 400, "common.noChanges"},
		{"rename to taken", "PATCH", "/api/catalog/disease-models/" + id(model), map[string]any{"name": "MCAO Stroke"}, 409, "catalog.nameTaken"},
		{"unknown substance", "GET", "/api/catalog/substances/00000000-0000-0000-0000-000000000000", nil, 404, "substance.notFound"},
		{"malformed substance", "PATCH", "/api/catalog/substances/x", map[string]any{"name": "Y"}, 404, "substance.notFound"},
		{"unknown disease model", "DELETE", "/api/catalog/disease-models/00000000-0000-0000-0000-000000000000", nil, 404, "diseaseModel.notFound"},
		{"bad kind filter", "GET", "/api/catalog/substances?kind=vehicle", nil, 400, "common.invalidQuery"},
		{"bad induction filter", "GET", "/api/catalog/disease-models?inductionType=GENETIC", nil, 400, "common.invalidQuery"},
	} {
		c.expect(tc.label, c.call(tc.method, tc.path, admin, tc.body), tc.status, tc.code)
	}

	writes := []struct {
		method, path string
		body         any
	}{
		{"PATCH", "/api/catalog/substances/" + id(mptp), map[string]any{"description": "toxin"}},
		{"DELETE", "/api/catalog/substances/" + id(mptp), nil},
		{"PATCH", "/api/catalog/disease-models/" + id(model), map[string]any{"description": "toxin model"}},
		{"DELETE", "/api/catalog/disease-models/" + id(model), nil},
	}
	reads := []string{
		"/api/catalog/substances", "/api/catalog/substances/" + id(mptp),
		"/api/catalog/disease-models", "/api/catalog/disease-models/" + id(model),
	}
	for _, role := range []string{"TECHNICIAN", "VIEWER"} {
		for _, w := range writes {
			c.expect(role+" "+w.method+" "+w.path, c.call(w.method, w.path, tokens[role], w.body), 403, "auth.forbidden")
		}
	}
	for _, role := range wp1Roles {
		for _, path := range reads {
			c.expect(role+" GET "+path, c.call("GET", path, tokens[role], nil), 200, "")
		}
	}
	for _, path := range reads {
		c.expect("anonymous GET "+path, c.call("GET", path, "", nil), 401, "auth.unauthenticated")
	}

	listed := func(path string) []string {
		var out []string
		for _, item := range c.expect("GET "+path, c.call("GET", path, tokens["VIEWER"], nil), 200, "").body["data"].([]any) {
			out = append(out, item.(map[string]any)["name"].(string))
		}
		return out
	}
	if got := listed("/api/catalog/substances?kind=VEHICLE"); len(got) != 1 || got[0] != "Saline 0.9%" {
		t.Fatalf("kind filter: %v", got)
	}
	if got := listed("/api/catalog/substances?search=%25"); len(got) != 1 || got[0] != "Saline 0.9%" {
		t.Fatalf("escaped search: %v", got)
	}
	if got := listed("/api/catalog/disease-models?inductionType=SURGICAL&search=mcao"); len(got) != 1 || got[0] != "MCAO stroke" {
		t.Fatalf("disease model filters: %v", got)
	}
	if got := listed("/api/catalog/substances"); len(got) != 5 || got[0] != "MPTP" {
		t.Fatalf("substances by name: %v", got)
	}

	patched := c.expect("patch substance", c.call("PATCH", "/api/catalog/substances/"+id(saline), tokens["LAB_MANAGER"], map[string]any{"kind": "REFERENCE_DRUG", "description": ""}), 200, "").body
	if patched["kind"] != "REFERENCE_DRUG" || patched["description"] != nil || patched["name"] != "Saline 0.9%" {
		t.Fatalf("patched substance: %v", patched)
	}
	patchedModel := c.expect("patch disease model", c.call("PATCH", "/api/catalog/disease-models/"+id(model), admin, map[string]any{"confirmationCriterion": "", "inductionType": "OTHER"}), 200, "").body
	if patchedModel["confirmationCriterion"] != nil || patchedModel["inductionType"] != "OTHER" {
		t.Fatalf("patched disease model: %v", patchedModel)
	}

	if _, err := pool.Exec(ctx, `
		WITH e AS (INSERT INTO misko.experiments (code, title) VALUES ('E1', 'Study') RETURNING id),
		     g AS (INSERT INTO misko.experiment_groups (experiment_id, code, name, role, disease_model_id)
		           SELECT e.id, 'G1', 'Stroke', 'DISEASE_CONTROL', $2 FROM e RETURNING id, experiment_id)
		INSERT INTO misko.regimen_items (experiment_id, group_id, purpose, substance_id, dose_micro, unit, route, frequency, start_day, end_day)
		SELECT g.experiment_id, g.id, 'VEHICLE', $1, 10000000, 'mL/kg', 'ORAL', 'DAILY', 0, 14 FROM g`, id(saline), id(stroke)); err != nil {
		t.Fatal(err)
	}
	c.expect("delete used substance", c.call("DELETE", "/api/catalog/substances/"+id(saline), admin, nil), 409, "substance.inUse")
	c.expect("delete used disease model", c.call("DELETE", "/api/catalog/disease-models/"+id(stroke), admin, nil), 409, "diseaseModel.inUse")
	c.expect("delete free substance", c.call("DELETE", "/api/catalog/substances/"+id(mptp), tokens["RESEARCHER"], nil), 204, "")
	c.expect("deleted substance", c.call("GET", "/api/catalog/substances/"+id(mptp), admin, nil), 404, "substance.notFound")
	c.expect("delete free disease model", c.call("DELETE", "/api/catalog/disease-models/"+id(model), admin, nil), 204, "")
	c.expect("deleted disease model", c.call("GET", "/api/catalog/disease-models/"+id(model), admin, nil), 404, "diseaseModel.notFound")
}
