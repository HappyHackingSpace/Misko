//go:build integration

package bootstrap

import (
	"encoding/csv"
	"io"
	"math"
	"testing"
)

// A study whose animals were analysed and reviewed: the report counts only
// accepted results of animals that are not excluded, shows what is still
// pending, and derives the comparisons from the group roles.
func TestReportsOverHTTP(t *testing.T) {
	a := newStudyAPI(t, nil)
	w := newStudyWorld(t, a.pool, "EXP-R")
	animals := map[string]string{}
	for _, a := range []struct{ group, code string }{
		{"G1", "M-11"}, {"G1", "M-12"}, {"G2", "M-21"}, {"G2", "M-22"}, {"G2", "M-23"}, {"G3", "M-31"},
		{"G4", "M-41"}, {"G4", "M-42"}, {"G5", "M-51"}, {"G5", "M-52"}, {"G5", "M-53"},
	} {
		animals[a.code] = w.allocate(a.group, a.code)
	}
	w.start()
	subjects := map[string]string{}
	for _, session := range [][]string{{"M-11", "M-12", "M-21"}, {"M-22", "M-23", "M-31"}, {"M-41", "M-42", "M-51"}, {"M-52", "M-53"}} {
		var allocations []string
		for _, code := range session {
			allocations = append(allocations, animals[code])
		}
		_, ids := w.session("Day 14", allocations...)
		for i, code := range session {
			subjects[code] = ids[i]
		}
	}
	_, baseline := w.session("Baseline", animals["M-11"], animals["M-21"])
	w.publish(baseline[0], distance(90), "ACCEPTED")
	w.publish(baseline[1], distance(100), "ACCEPTED")
	for code, value := range map[string]float64{"M-11": 100, "M-12": 120, "M-21": 40, "M-22": 60, "M-31": 90, "M-41": 60, "M-51": 80, "M-52": 1000} {
		w.publish(subjects[code], distance(value), "ACCEPTED")
	}
	w.publish(subjects["M-23"], distance(999), "PENDING")
	w.publish(subjects["M-42"], distance(5), "REJECTED")
	w.publish(subjects["M-53"], nil, "ACCEPTED")
	w.exec("UPDATE misko.allocations SET excluded_at = now(), exclusion_reason = 'HUMANE_ENDPOINT' WHERE id = $1", animals["M-52"])

	path := "/api/experiments/" + w.exp + "/report"
	a.expect("anonymous", "GET", path, "", nil, 401, "auth.unauthenticated")
	a.expect("unknown experiment", "GET", "/api/experiments/"+w.endpoint+"/report", "VIEWER", nil, 404, "report.experimentNotFound")
	a.expect("malformed endpoint", "GET", path+"?endpointId=E1", "VIEWER", nil, 400, "report.invalidQuery")
	a.expect("endpoint of another experiment", "GET", path+"?endpointId="+w.exp, "VIEWER", nil, 404, "report.endpointNotFound")
	report := a.expect("report", "GET", path, "VIEWER", nil, 200, "")
	if report["experiment"].(map[string]any)["code"] != "EXP-R" || len(report["groups"].([]any)) != 5 || len(report["timepoints"].([]any)) != 2 {
		t.Fatalf("report: %v", report)
	}
	codes := map[string]string{}
	for _, g := range report["groups"].([]any) {
		group := g.(map[string]any)
		codes[group["id"].(string)] = group["code"].(string)
		if group["code"] == "G5" && (group["excluded"] != float64(1) || group["allocated"] != float64(3) || group["treatment"].(map[string]any)["doseMicro"] != float64(3_000_000)) {
			t.Fatalf("group G5: %v", group)
		}
	}
	section := report["sections"].([]any)[0].(map[string]any)
	if section["endpoint"].(map[string]any)["id"] != w.endpoint || section["comparable"] != true || len(section["versions"].([]any)) != 1 {
		t.Fatalf("section: %v", section)
	}
	stats := map[string]map[string]any{}
	for _, g := range section["groups"].([]any) {
		stats[codes[g.(map[string]any)["groupId"].(string)]] = g.(map[string]any)
	}
	near := func(label string, got any, want float64) {
		t.Helper()
		if v, ok := got.(float64); !ok || math.Abs(v-want) > 1e-9 {
			t.Errorf("%s: %v want %v", label, got, want)
		}
	}
	near("healthy mean", stats["G1"]["mean"], 110)
	near("healthy sem", stats["G1"]["sem"], 10)
	near("healthy change", stats["G1"]["change"].(map[string]any)["mean"], 10)
	near("disease mean", stats["G2"]["mean"], 50)
	near("disease change", stats["G2"]["change"].(map[string]any)["mean"], -60)
	near("disease pending", stats["G2"]["pending"], 1)
	near("low dose rejected", stats["G4"]["rejected"], 1)
	near("high dose n", stats["G5"]["n"], 1)
	near("high dose mean without the excluded animal", stats["G5"]["mean"], 80)
	near("high dose missing", stats["G5"]["missing"], 1)
	near("high dose excluded", stats["G5"]["excluded"], 1)
	rescue := map[string]any{}
	for _, c := range section["comparisons"].([]any) {
		comparison := c.(map[string]any)
		rescue[comparison["kind"].(string)+" "+codes[comparison["groupId"].(string)]] = comparison["rescuePercent"]
		if comparison["kind"] == "MODEL_VALIDITY" {
			near("model validity", comparison["difference"], -60)
		}
	}
	near("assay sensitivity", rescue["ASSAY_SENSITIVITY G3"], 40.0/60*100)
	near("efficacy low dose", rescue["EFFICACY G4"], 10.0/60*100)
	near("efficacy high dose", rescue["EFFICACY G5"], 30.0/60*100)
	dose := section["doseResponse"].([]any)
	if len(dose) != 1 || len(dose[0].(map[string]any)["points"].([]any)) != 2 {
		t.Fatalf("dose response: %v", dose)
	}
	if course := section["timeCourse"].([]any); len(course) != 2 {
		t.Fatalf("time course: %v", course)
	}

	read := func(query string) [][]string {
		t.Helper()
		res := a.raw("GET", path+"/export"+query, a.tokens["VIEWER"], nil)
		defer res.Body.Close()
		if res.StatusCode != 200 || res.Header.Get("Content-Type") != "text/csv; charset=utf-8" {
			body, _ := io.ReadAll(res.Body)
			t.Fatalf("export %s: %d %s", query, res.StatusCode, body)
		}
		records, err := csv.NewReader(res.Body).ReadAll()
		if err != nil {
			t.Fatal(err)
		}
		return records
	}
	if animals := read(""); len(animals) != 1+11+2 || animals[0][0] != "experimentCode" {
		t.Fatalf("animal export: %d rows %v", len(animals), animals[0])
	}
	if groups := read("?level=groups&endpointId=" + w.endpoint); len(groups) != 1+2*5 || groups[0][len(groups[0])-1] != "comparable" {
		t.Fatalf("group export: %d rows", len(groups))
	}
	a.expect("unknown level", "GET", path+"/export?level=cages", "VIEWER", nil, 400, "report.invalidQuery")
}
