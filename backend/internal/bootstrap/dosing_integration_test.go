//go:build integration

package bootstrap

import (
	"context"
	"fmt"
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

// TestDosingOverHTTP checks the role-by-operation matrix for administrations,
// the due-dose list and induction outcomes, and their behaviour end to end.
// Experiments, groups, regimen items, allocations, substances and weights are
// inserted with SQL because their own contexts are not needed for dosing.
func TestDosingOverHTTP(t *testing.T) {
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

	sqlID := func(sql string, args ...any) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return id
	}
	const recorder = "01a00000-0000-7000-8000-000000000001"
	now := time.Now().UTC()
	saline := sqlID(`INSERT INTO misko.substances (name, kind) VALUES ('Saline', 'VEHICLE') RETURNING id`)
	mptp := sqlID(`INSERT INTO misko.substances (name, kind) VALUES ('MPTP', 'INDUCER') RETURNING id`)
	model := sqlID(`INSERT INTO misko.disease_models (name, induction_type) VALUES ('MPTP Parkinson', 'CHEMICAL') RETURNING id`)
	exp := sqlID(`INSERT INTO misko.experiments (code, title) VALUES ('E1', 'MPTP study') RETURNING id`)
	draft := sqlID(`INSERT INTO misko.experiments (code, title) VALUES ('E2', 'Draft study') RETURNING id`)
	healthy := sqlID(`INSERT INTO misko.experiment_groups (experiment_id, code, name, role) VALUES ($1, 'G1', 'Healthy', 'HEALTHY_CONTROL') RETURNING id`, exp)
	disease := sqlID(`INSERT INTO misko.experiment_groups (experiment_id, code, name, role, disease_model_id) VALUES ($1, 'G2', 'MPTP', 'DISEASE_CONTROL', $2) RETURNING id`, exp, model)
	draftGroup := sqlID(`INSERT INTO misko.experiment_groups (experiment_id, code, name, role) VALUES ($1, 'G1', 'Healthy', 'HEALTHY_CONTROL') RETURNING id`, draft)
	item := func(group, purpose, substance, unit, frequency string, start, end int) string {
		return sqlID(`INSERT INTO misko.regimen_items (experiment_id, group_id, purpose, substance_id, dose_micro, unit, route, frequency, start_day, end_day)
			VALUES ($1, $2, $3, $4, 10000000, $5, 'INTRAPERITONEAL', $6, $7, $8) RETURNING id`, exp, group, purpose, substance, unit, frequency, start, end)
	}
	healthyVehicle := item(healthy, "VEHICLE", saline, "mL/kg", "DAILY", 1, 5)
	induction := item(disease, "INDUCTION", mptp, "mg/kg", "ONCE", 0, 0)
	sickVehicle := item(disease, "VEHICLE", saline, "mL/kg", "DAILY", 1, 5)
	n := 0
	animal := func(experiment, group string) (subject, allocation string) {
		n++
		subject = sqlID(`INSERT INTO misko.subjects (code, species, sex) VALUES ($1, 'MOUSE', 'MALE') RETURNING id`, fmt.Sprintf("M%02d", n))
		allocation = sqlID(`INSERT INTO misko.allocations (experiment_id, group_id, subject_id, allocated_at, created_by) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			experiment, group, subject, now.Add(-5*24*time.Hour), recorder)
		return subject, allocation
	}
	weigh := func(subject string) string {
		return sqlID(`INSERT INTO misko.weight_measurements (subject_id, body_milligrams, measured_at, recorded_by) VALUES ($1, 25000, $2, $3) RETURNING id`,
			subject, now.Add(-2*time.Hour), recorder)
	}
	m1, a1 := animal(exp, healthy)
	m2, a2 := animal(exp, disease)
	_, a3 := animal(exp, healthy)
	_, draftAllocation := animal(draft, draftGroup)
	sick := map[string]string{}
	for _, role := range roles {
		_, sick[role] = animal(exp, disease)
	}
	weigh(m1)
	weigh(m2)
	// Day 0 was yesterday in the laboratory's time zone (UTC), so today is study day 1.
	if _, err := pool.Exec(ctx, `UPDATE misko.experiments SET status = 'RUNNING', day_zero = (now() AT TIME ZONE 'UTC')::date - 1, started_at = now() - interval '1 day' WHERE id = $1`, exp); err != nil {
		t.Fatal(err)
	}
	base, draftBase := "/api/experiments/"+exp, "/api/experiments/"+draft
	at := func(ago time.Duration) string { return now.Add(-ago).Format(time.RFC3339) }
	unplanned := func(ago time.Duration) map[string]any {
		return map[string]any{"purpose": "VEHICLE", "substanceId": saline, "amount": "0.2", "unit": "mL", "route": "ORAL", "administeredAt": at(ago)}
	}

	runs := map[string]bool{"SUPERADMIN": true, "LAB_MANAGER": true, "RESEARCHER": true, "TECHNICIAN": true}
	for i, role := range roles {
		token := tokens[role]
		want, code := 201, ""
		if !runs[role] {
			want, code = 403, "auth.forbidden"
		}
		c.expect(role+" records administration", c.call("POST", base+"/allocations/"+a2+"/administrations", token, unplanned(time.Duration(i+1)*time.Minute)), want, code)
		c.expect(role+" records induction outcome", c.call("POST", base+"/allocations/"+sick[role]+"/induction-outcome", token, map[string]any{
			"status": "CONFIRMED", "assessedAt": at(time.Minute),
		}), want, code)
		for _, path := range []string{
			base + "/administrations", "/api/subjects/" + m2 + "/administrations", base + "/dosing", base + "/induction-outcomes",
		} {
			c.expect(role+" GET "+path, c.call("GET", path, token, nil), 200, "")
			if role == "VIEWER" {
				c.expect("anonymous GET "+path, c.call("GET", path, "", nil), 401, "auth.unauthenticated")
			}
		}
	}

	tech := tokens["TECHNICIAN"]
	given := c.create(base+"/allocations/"+a1+"/administrations", tech, map[string]any{"regimenItemId": healthyVehicle, "administeredAt": at(time.Hour)})
	abs, _ := given["absoluteDose"].(map[string]any)
	dose, _ := given["dose"].(map[string]any)
	if given["regimenItemId"] != healthyVehicle || given["purpose"] != "VEHICLE" || given["substanceName"] != "Saline" || given["bodyWeightGrams"] != "25" ||
		dose["amount"] != "10" || dose["unit"] != "mL/kg" || abs["amount"] != "0.25" || abs["unit"] != "mL" || given["route"] != "INTRAPERITONEAL" {
		t.Fatalf("planned per-kg dose: %v", given)
	}

	post := func(allocationPath string, body map[string]any) response {
		return c.call("POST", allocationPath+"/administrations", tech, body)
	}
	c.expect("dose in a draft", post(draftBase+"/allocations/"+draftAllocation, unplanned(time.Minute)), 409, "experiment.notRunning")
	c.expect("unknown allocation", post(base+"/allocations/"+draftAllocation, unplanned(time.Minute)), 404, "allocation.notFound")
	c.expect("malformed allocation", post(base+"/allocations/nope", unplanned(time.Minute)), 404, "allocation.notFound")
	c.expect("regimen item of another group", post(base+"/allocations/"+a1, map[string]any{"regimenItemId": induction, "administeredAt": at(time.Minute)}), 400, "administration.regimenItemOtherGroup")
	c.expect("unknown regimen item", post(base+"/allocations/"+a1, map[string]any{"regimenItemId": a1, "administeredAt": at(time.Minute)}), 404, "regimenItem.notFound")
	c.expect("per-kg dose without weight", post(base+"/allocations/"+a3, map[string]any{"regimenItemId": healthyVehicle, "administeredAt": at(time.Minute)}), 400, "administration.weightRequired")
	c.expect("weight of another animal", post(base+"/allocations/"+a3, map[string]any{
		"regimenItemId": healthyVehicle, "weightMeasurementId": weigh(m1), "administeredAt": at(time.Minute),
	}), 400, "administration.weightOtherSubject")
	c.expect("other substance than planned", post(base+"/allocations/"+a2, map[string]any{"regimenItemId": induction, "substanceId": saline, "administeredAt": at(time.Minute)}), 400, "administration.substanceMismatch")
	c.expect("unplanned without route", post(base+"/allocations/"+a2, map[string]any{"purpose": "VEHICLE", "substanceId": saline, "amount": "1", "unit": "mL", "administeredAt": at(time.Minute)}), 400, "administration.unplannedIncomplete")
	c.expect("unknown substance", post(base+"/allocations/"+a2, map[string]any{"purpose": "VEHICLE", "substanceId": a2, "amount": "1", "unit": "mL", "route": "ORAL", "administeredAt": at(time.Minute)}), 404, "substance.notFound")
	bad := unplanned(time.Minute)
	bad["amount"] = "1e3"
	c.expect("invalid amount", post(base+"/allocations/"+a2, bad), 400, "dose.invalidAmount")
	c.expect("future dose", post(base+"/allocations/"+a2, unplanned(-time.Hour)), 400, "common.futureTime")
	c.expect("before allocation", post(base+"/allocations/"+a2, unplanned(30*24*time.Hour)), 400, "dosing.beforeAllocation")

	today := now.Format(time.DateOnly)
	schedule := c.expect("dosing today", c.call("GET", base+"/dosing?date="+today, tokens["VIEWER"], nil), 200, "").body
	animals, _ := schedule["animals"].([]any)
	if schedule["studyDay"] != float64(1) || schedule["date"] != today || len(animals) != 3+len(roles) {
		t.Fatalf("day 1: %v", schedule)
	}
	first, _ := animals[0].(map[string]any)
	doses, _ := first["doses"].([]any)
	vehicle, _ := doses[0].(map[string]any)
	if first["allocationId"] != a1 || first["groupCode"] != "G1" || vehicle["regimenItemId"] != healthyVehicle || vehicle["due"] != float64(1) ||
		vehicle["given"] != float64(1) || vehicle["complete"] != true {
		t.Fatalf("M01 on day 1: %v", first)
	}
	byAllocation := map[string]map[string]any{}
	for _, raw := range animals {
		a := raw.(map[string]any)
		byAllocation[a["allocationId"].(string)] = a
	}
	m2Dose := byAllocation[a2]["doses"].([]any)[0].(map[string]any)
	if m2Dose["regimenItemId"] != sickVehicle || m2Dose["given"] != float64(0) || m2Dose["complete"] != false || m2Dose["absoluteDose"].(map[string]any)["amount"] != "0.25" {
		t.Fatalf("M02 on day 1 (unplanned doses do not count): %v", m2Dose)
	}
	if noWeight := byAllocation[a3]; noWeight["weight"] != nil || noWeight["doses"].([]any)[0].(map[string]any)["absoluteDose"] != nil {
		t.Fatalf("M03 has no weight: %v", noWeight)
	}
	if day0 := c.call("GET", base+"/dosing?date="+now.Add(-24*time.Hour).Format(time.DateOnly), admin, nil).body; day0["studyDay"] != float64(0) ||
		len(day0["animals"].([]any)) != 1+len(roles) {
		t.Fatalf("day 0, only the disease group is induced: %v", day0)
	}
	if implicit := c.call("GET", base+"/dosing", admin, nil).body; implicit["date"] != today {
		t.Fatalf("date defaults to the laboratory's today: %v", implicit)
	}
	c.expect("bad date", c.call("GET", base+"/dosing?date=31.12.2026", admin, nil), 400, "common.invalidDate")
	c.expect("dosing of a draft", c.call("GET", draftBase+"/dosing", admin, nil), 409, "experiment.notStarted")
	c.expect("dosing of an unknown experiment", c.call("GET", "/api/experiments/"+a1+"/dosing", admin, nil), 404, "experiment.notFound")

	if page := c.call("GET", base+"/administrations?allocationId="+a1, tokens["VIEWER"], nil).body; page["total"] != float64(1) {
		t.Fatalf("per animal in the experiment: %v", page)
	}
	if page := c.call("GET", base+"/administrations?purpose=VEHICLE&pageSize=2", tokens["VIEWER"], nil).body; page["total"] != float64(len(runs)+1) || len(page["data"].([]any)) != 2 {
		t.Fatalf("experiment administrations: %v", page)
	}
	if page := c.call("GET", "/api/subjects/"+m1+"/administrations", tokens["VIEWER"], nil).body; page["total"] != float64(1) {
		t.Fatalf("subject administrations: %v", page)
	}
	c.expect("bad range", c.call("GET", base+"/administrations?from=yesterday", admin, nil), 400, "common.invalidQuery")
	c.expect("bad purpose", c.call("GET", base+"/administrations?purpose=x", admin, nil), 400, "common.invalidQuery")

	outcome := func(allocation string, body map[string]any) response {
		return c.call("POST", base+"/allocations/"+allocation+"/induction-outcome", tech, body)
	}
	c.expect("outcome of a healthy control", outcome(a1, map[string]any{"status": "CONFIRMED", "assessedAt": at(time.Minute)}), 409, "inductionOutcome.noDisease")
	c.expect("invalid outcome", outcome(a2, map[string]any{"status": "INDUCED", "assessedAt": at(time.Minute)}), 400, "inductionOutcome.invalidStatus")
	recorded := c.create(base+"/allocations/"+a2+"/induction-outcome", tech, map[string]any{"status": "NOT_CONFIRMED", "note": "no deficit", "assessedAt": at(time.Minute)})
	if recorded["allocationId"] != a2 || recorded["status"] != "NOT_CONFIRMED" || recorded["note"] != "no deficit" {
		t.Fatalf("outcome: %v", recorded)
	}
	c.expect("second outcome", outcome(a2, map[string]any{"status": "CONFIRMED", "assessedAt": at(time.Minute)}), 409, "inductionOutcome.exists")
	list := c.call("GET", base+"/induction-outcomes", tokens["VIEWER"], nil).body["data"].([]any)
	if len(list) != len(runs)+1 || list[0].(map[string]any)["subjectId"] != m2 || list[0].(map[string]any)["groupCode"] != "G2" {
		t.Fatalf("outcomes: %v", list)
	}
	if confirmed := c.call("GET", base+"/induction-outcomes?status=CONFIRMED", admin, nil).body["data"].([]any); len(confirmed) != len(runs) {
		t.Fatalf("confirmed outcomes: %v", confirmed)
	}
	c.expect("outcome in a draft", c.call("POST", draftBase+"/allocations/"+draftAllocation+"/induction-outcome", tech, map[string]any{
		"status": "CONFIRMED", "assessedAt": at(time.Minute),
	}), 409, "experiment.notRunning")
}
