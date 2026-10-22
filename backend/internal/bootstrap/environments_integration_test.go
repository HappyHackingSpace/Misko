//go:build integration

package bootstrap

import "testing"

// TestEnvironmentsOverHTTP checks that revisions carry apparatus and analysis
// values as `parameters`, validated against the paradigm catalog, and the
// apparatus:write role matrix.
func TestEnvironmentsOverHTTP(t *testing.T) {
	c, tokens, _ := newWP1Client(t)
	admin := tokens["SUPERADMIN"]
	apparatusWriters := map[string]bool{"SUPERADMIN": true, "LAB_MANAGER": true, "RESEARCHER": true}
	arena := func(width float64) map[string]any {
		return map[string]any{"tank_diameter_cm": width, "platform_diameter_cm": 10, "platform_x_cm": 30, "platform_y_cm": 30}
	}

	for _, role := range wp1Roles {
		status, code := 403, "auth.forbidden"
		if apparatusWriters[role] {
			status, code = 201, ""
		}
		c.expect(role+" creates environment", c.call("POST", "/api/environments", tokens[role], map[string]any{
			"name": "Arena " + role, "paradigmKey": "MWM", "revision": map[string]any{"paradigmVersion": 1, "parameters": arena(120)},
		}), status, code)
	}
	created := c.create("/api/environments", admin, map[string]any{
		"name": "Box 1", "paradigmKey": "MWM",
		"revision": map[string]any{"paradigmVersion": 1, "parameters": map[string]any{"tank_diameter_cm": 120, "platform_diameter_cm": 10, "platform_x_cm": 30, "platform_y_cm": 30, "max_sample_gap_s": 1}},
	})
	env, revision := created["environment"].(map[string]any), created["revision"].(map[string]any)
	parameters, ok := revision["parameters"].(map[string]any)
	if !ok || parameters["max_sample_gap_s"] != float64(1) || parameters["tank_diameter_cm"] != float64(120) || len(parameters) != 5 {
		t.Fatalf("revision parameters: %v", revision)
	}
	if _, old := revision["apparatus"]; old {
		t.Fatalf("revision still has the apparatus field: %v", revision)
	}
	base := "/api/environments/" + id(env)

	for _, tc := range []struct {
		label string
		body  map[string]any
		code  string
	}{
		{"old field name", map[string]any{"paradigmVersion": 1, "apparatus": arena(120)}, "common.invalidJSON"},
		{"missing measurement", map[string]any{"paradigmVersion": 1, "parameters": map[string]any{"tank_diameter_cm": 120}}, "environment.invalidParameters"},
		{"out of range", map[string]any{"paradigmVersion": 1, "parameters": arena(50)}, "environment.invalidParameters"},
		{"parameter of another paradigm", map[string]any{"paradigmVersion": 1, "parameters": map[string]any{"tank_diameter_cm": 120, "platform_diameter_cm": 10, "platform_x_cm": 30, "platform_y_cm": 30, "pole_length_cm": 50}}, "environment.invalidParameters"},
		{"unknown version", map[string]any{"paradigmVersion": 9, "parameters": arena(120)}, "environment.unknownParadigmVersion"},
	} {
		c.expect(tc.label, c.call("POST", base+"/revisions", admin, tc.body), 400, tc.code)
	}
	c.expect("session value in treadmill parameters", c.call("POST", "/api/environments", admin, map[string]any{
		"name": "Belt", "paradigmKey": "TREADMILL", "revision": map[string]any{"paradigmVersion": 1, "parameters": map[string]any{"lane_length_cm": 40, "max_trial_duration_s": 60}},
	}), 400, "environment.invalidParameters")

	second := c.create(base+"/revisions", tokens["RESEARCHER"], map[string]any{"paradigmVersion": 1, "parameters": arena(130)})
	if second["number"] != float64(2) || second["parameters"].(map[string]any)["tank_diameter_cm"] != float64(130) {
		t.Fatalf("second revision: %v", second)
	}
	c.expect("technician revises", c.call("POST", base+"/revisions", tokens["TECHNICIAN"], map[string]any{"paradigmVersion": 1, "parameters": arena(130)}), 403, "auth.forbidden")
	first := c.expect("revision 1", c.call("GET", base+"/revisions/1", tokens["VIEWER"], nil), 200, "").body
	if first["parameters"].(map[string]any)["max_sample_gap_s"] != float64(1) {
		t.Fatalf("revision 1 changed: %v", first)
	}
	list := c.expect("revisions", c.call("GET", base+"/revisions", tokens["VIEWER"], nil), 200, "").body["data"].([]any)
	if len(list) != 2 || list[1].(map[string]any)["parameters"] == nil {
		t.Fatalf("revisions: %v", list)
	}
	if got := c.expect("environment", c.call("GET", base, tokens["VIEWER"], nil), 200, "").body; got["latestRevision"] != float64(2) {
		t.Fatalf("latest revision: %v", got)
	}
}
