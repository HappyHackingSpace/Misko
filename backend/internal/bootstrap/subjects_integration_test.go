//go:build integration

package bootstrap

import (
	"context"
	"testing"
	"time"
)

// TestSubjectsWeightsAndAllocationOverHTTP checks subject RBAC, the read-only
// allocation summary, the unallocated filter and body weights end to end.
func TestSubjectsWeightsAndAllocationOverHTTP(t *testing.T) {
	ctx := context.Background()
	c, tokens, pool := newWP1Client(t)
	admin := tokens["SUPERADMIN"]
	subjectWriters := map[string]bool{"SUPERADMIN": true, "LAB_MANAGER": true, "RESEARCHER": true}
	weightWriters := map[string]bool{"SUPERADMIN": true, "LAB_MANAGER": true, "RESEARCHER": true, "TECHNICIAN": true}

	for _, role := range wp1Roles {
		status, code := 403, "auth.forbidden"
		if subjectWriters[role] {
			status, code = 201, ""
		}
		c.expect(role+" creates subject", c.call("POST", "/api/subjects", tokens[role], map[string]any{"code": "S-" + role, "species": "MOUSE", "sex": "FEMALE"}), status, code)
	}
	free := c.create("/api/subjects", admin, map[string]any{"code": "F-001", "species": "MOUSE", "sex": "FEMALE"})
	if v, ok := free["allocation"]; !ok || v != nil {
		t.Fatalf("unallocated subject must carry allocation null: %v", free)
	}
	allocated := c.create("/api/subjects", admin, map[string]any{"code": "F-002", "species": "RAT", "sex": "MALE"})
	c.expect("allocation is read-only", c.call("PATCH", "/api/subjects/"+id(allocated), admin, map[string]any{"allocation": nil}), 400, "common.invalidJSON")

	var experimentID, groupID string
	if err := pool.QueryRow(ctx, `
		WITH e AS (INSERT INTO misko.experiments (code, title) VALUES ('EXP-1', 'Study') RETURNING id),
		     g AS (INSERT INTO misko.experiment_groups (experiment_id, code, name, role) SELECT e.id, 'G1', 'Healthy', 'HEALTHY_CONTROL' FROM e RETURNING id, experiment_id)
		INSERT INTO misko.allocations (experiment_id, group_id, subject_id, allocated_at, created_by)
		SELECT g.experiment_id, g.id, $1, now() - interval '1 hour', $2 FROM g
		RETURNING experiment_id, group_id`, id(allocated), "01900000-0000-7000-8000-000000000001").Scan(&experimentID, &groupID); err != nil {
		t.Fatal(err)
	}
	summary := c.expect("allocated subject", c.call("GET", "/api/subjects/"+id(allocated), tokens["VIEWER"], nil), 200, "").body["allocation"].(map[string]any)
	want := map[string]any{"experimentId": experimentID, "experimentCode": "EXP-1", "groupId": groupID, "groupCode": "G1", "groupRole": "HEALTHY_CONTROL", "excluded": false}
	for key, value := range want {
		if summary[key] != value {
			t.Fatalf("allocation %s=%v want %v (%v)", key, summary[key], value, summary)
		}
	}
	if updated := c.expect("patch allocated subject", c.call("PATCH", "/api/subjects/"+id(allocated), admin, map[string]any{"sex": "UNKNOWN"}), 200, "").body; updated["allocation"] == nil || updated["sex"] != "UNKNOWN" {
		t.Fatalf("patch response lost the allocation: %v", updated)
	}

	codes := func(query string) []string {
		var out []string
		for _, item := range c.expect("list "+query, c.call("GET", "/api/subjects?sort=code&"+query, admin, nil), 200, "").body["data"].([]any) {
			out = append(out, item.(map[string]any)["code"].(string))
		}
		return out
	}
	if got := codes("search=F-"); len(got) != 2 {
		t.Fatalf("all F subjects: %v", got)
	}
	if got := codes("search=F-&unallocated=true"); len(got) != 1 || got[0] != "F-001" {
		t.Fatalf("unallocated filter: %v", got)
	}
	if got := codes("search=F-&unallocated=false"); len(got) != 2 {
		t.Fatalf("unallocated=false must not filter: %v", got)
	}
	if total := c.call("GET", "/api/subjects?unallocated=true", admin, nil).body["total"]; total != float64(len(wp1Roles)-2+1) {
		t.Fatalf("unallocated total: %v", total)
	}
	c.expect("bad unallocated value", c.call("GET", "/api/subjects?unallocated=yes", admin, nil), 400, "common.invalidQuery")

	weights := "/api/subjects/" + id(free) + "/weights"
	now := time.Now().UTC()
	for _, role := range wp1Roles {
		status, code := 403, "auth.forbidden"
		if weightWriters[role] {
			status, code = 201, ""
		}
		c.expect(role+" records weight", c.call("POST", weights, tokens[role], map[string]any{"grams": "25", "measuredAt": now.Add(-48 * time.Hour).Format(time.RFC3339)}), status, code)
		c.expect(role+" reads weights", c.call("GET", weights, tokens[role], nil), 200, "")
	}
	c.expect("anonymous weights", c.call("GET", weights, "", nil), 401, "auth.unauthenticated")
	latest := c.create(weights, tokens["TECHNICIAN"], map[string]any{"grams": "26.125", "measuredAt": now.Add(-time.Hour).Format(time.RFC3339Nano)})
	if latest["grams"] != "26.125" || latest["subjectId"] != id(free) || latest["recordedBy"] == nil {
		t.Fatalf("weight representation: %v", latest)
	}
	for _, tc := range []struct {
		label string
		body  map[string]any
		code  string
	}{
		{"zero grams", map[string]any{"grams": "0", "measuredAt": now.Format(time.RFC3339)}, "weight.invalid"},
		{"too many decimals", map[string]any{"grams": "25.1234", "measuredAt": now.Format(time.RFC3339)}, "weight.invalid"},
		{"numeric grams", map[string]any{"grams": 25, "measuredAt": now.Format(time.RFC3339)}, "common.invalidJSON"},
		{"missing time", map[string]any{"grams": "25"}, "common.invalidTime"},
		{"future time", map[string]any{"grams": "25", "measuredAt": now.Add(time.Hour).Format(time.RFC3339)}, "common.futureTime"},
	} {
		c.expect(tc.label, c.call("POST", weights, admin, tc.body), 400, tc.code)
	}
	c.expect("weight of unknown subject", c.call("POST", "/api/subjects/00000000-0000-0000-0000-000000000000/weights", admin, map[string]any{"grams": "25", "measuredAt": now.Format(time.RFC3339)}), 404, "subject.notFound")
	c.expect("weights of unknown subject", c.call("GET", "/api/subjects/00000000-0000-0000-0000-000000000000/weights", admin, nil), 404, "subject.notFound")
	list := c.expect("weights", c.call("GET", weights, tokens["VIEWER"], nil), 200, "").body["data"].([]any)
	if len(list) != len(weightWriters)+1 || list[0].(map[string]any)["id"] != id(latest) {
		t.Fatalf("weights newest first: %v", list)
	}

	c.expect("weighed subject", c.call("DELETE", "/api/subjects/"+id(free), admin, nil), 409, "subject.inUse")
	c.expect("allocated subject", c.call("DELETE", "/api/subjects/"+id(allocated), admin, nil), 409, "subject.inUse")
	spare := c.create("/api/subjects", admin, map[string]any{"code": "F-003", "species": "MOUSE", "sex": "MALE"})
	c.expect("technician deletes subject", c.call("DELETE", "/api/subjects/"+id(spare), tokens["TECHNICIAN"], nil), 403, "auth.forbidden")
	c.expect("delete free subject", c.call("DELETE", "/api/subjects/"+id(spare), tokens["RESEARCHER"], nil), 204, "")
	c.expect("code ignoring case", c.call("POST", "/api/subjects", admin, map[string]any{"code": "f-001", "species": "RAT", "sex": "MALE"}), 409, "subject.codeTaken")
}
