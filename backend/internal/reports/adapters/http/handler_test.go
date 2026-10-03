package reportshttp_test

import (
	"context"
	"encoding/csv"
	"encoding/json"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	reportshttp "github.com/HappyHackingSpace/Misko/backend/internal/reports/adapters/http"
	"github.com/HappyHackingSpace/Misko/backend/internal/reports/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/reports/domain"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

type store struct {
	metrics []domain.MetricRow
	events  []domain.EventRow
}

func (s store) MetricRows(context.Context, application.Filter) ([]domain.MetricRow, error) {
	return s.metrics, nil
}
func (s store) CountMetricRows(context.Context, application.Filter) (int, error) {
	return len(s.metrics), nil
}
func (s store) EventRows(context.Context, application.Filter) ([]domain.EventRow, error) {
	return s.events, nil
}
func (s store) CountEventRows(context.Context, application.Filter) (int, error) {
	return len(s.events), nil
}

func server(t *testing.T, s store) *httptest.Server {
	mux := http.NewServeMux()
	authenticate := func(r *http.Request) (access.Actor, error) {
		if r.Header.Get("Authorization") == "" {
			return access.Actor{}, access.ErrUnauthenticated
		}
		return access.Actor{UserID: "01a00000-0000-7000-8000-000000000001", Role: access.Viewer}, nil
	}
	reportshttp.Register(mux, application.New(s, 100), authenticate, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, srv *httptest.Server, path string, auth bool) (*http.Response, []byte) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	if auth {
		req.Header.Set("Authorization", "Bearer x")
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	return res, body
}

func provenance(test, subject, group string, engine int) domain.Provenance {
	return domain.Provenance{
		RunID: "run-" + test, Latest: true, Trigger: "AUTOMATIC", MetricEngineVersion: engine, ResultSchemaVersion: 1, TestID: test, SubjectID: subject,
		SubjectCode: "M-1", ExperimentCode: "EXP", GroupID: "g1", GroupName: group, GroupRole: "TREATMENT", ParadigmKey: "OPEN_FIELD", ParadigmVersion: 1,
		ScheduledAt: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC), RunFinishedAt: time.Date(2026, 9, 1, 11, 0, 0, 0, time.UTC),
	}
}

func TestMetricExportsKeepMissingValuesEmptyAndNeutralizeFormulas(t *testing.T) {
	value := 12.5
	srv := server(t, store{metrics: []domain.MetricRow{
		{Provenance: provenance("t1", "s1", "=HYPERLINK(\"x\")", 1), MetricKey: "distance_cm", Unit: "cm", Value: &value},
		{Provenance: provenance("t2", "s1", "+cmd", 1), MetricKey: "distance_cm", Unit: "cm", MissingReason: "NOT_OBSERVED"},
	}})

	res, body := get(t, srv, "/api/reports/metrics/export?experimentId=01a00000-0000-7000-8000-000000000009", true)
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "text/csv; charset=utf-8" || !strings.Contains(res.Header.Get("Content-Disposition"), `filename="misko-metrics.csv"`) {
		t.Fatalf("csv response: %d %v", res.StatusCode, res.Header)
	}
	records, err := csv.NewReader(strings.NewReader(string(body))).ReadAll()
	if err != nil || len(records) != 3 {
		t.Fatalf("csv: %v %q", err, body)
	}
	col := map[string]int{}
	for i, name := range records[0] {
		col[name] = i
	}
	first, second := records[1], records[2]
	if first[col["groupName"]] != `'=HYPERLINK("x")` || second[col["groupName"]] != "'+cmd" || first[col["subjectCode"]] != "M-1" {
		t.Fatalf("formula cells: %q %q", first[col["groupName"]], second[col["groupName"]])
	}
	if first[col["value"]] != "12.5" || second[col["value"]] != "" || second[col["missingReason"]] != "NOT_OBSERVED" || first[col["scheduledAt"]] != "2026-09-01T10:00:00Z" {
		t.Fatalf("values: %q %q", first, second)
	}

	res, body = get(t, srv, "/api/reports/metrics/export?format=json", true)
	var decoded struct {
		Data []map[string]any `json:"data"`
	}
	if res.StatusCode != 200 || json.Unmarshal(body, &decoded) != nil || len(decoded.Data) != 2 || decoded.Data[1]["value"] != nil ||
		decoded.Data[1]["missingReason"] != "NOT_OBSERVED" || decoded.Data[0]["subjectCode"] != "M-1" || decoded.Data[0]["phaseId"] != nil {
		t.Fatalf("json export: %d %s", res.StatusCode, body)
	}
	if res, body := get(t, srv, "/api/reports/metrics/export?format=xlsx", true); res.StatusCode != 400 || !strings.Contains(string(body), "report.invalidQuery") {
		t.Fatalf("unknown format: %d %s", res.StatusCode, body)
	}
	if res, _ := get(t, srv, "/api/reports/metrics/export", false); res.StatusCode != 401 {
		t.Fatalf("anonymous export: %d", res.StatusCode)
	}
}

func TestEventExportCarriesTrialProvenance(t *testing.T) {
	srv := server(t, store{events: []domain.EventRow{
		{Provenance: provenance("t1", "s1", "g", 1), EventID: "e1", EventType: "in_center", Kind: "INTERVAL", StartUs: 1, EndUs: 5, Confidence: 0.5, TrialID: "tr1", TrialNumber: 2, TrialRepetition: 1, TrialAttempt: 2},
		{Provenance: provenance("t1", "s1", "g", 1), EventID: "e2", EventType: "center_entry", Kind: "POINT", StartUs: 3, EndUs: 3, Confidence: 1},
	}})
	_, body := get(t, srv, "/api/reports/events/export", true)
	records, err := csv.NewReader(strings.NewReader(string(body))).ReadAll()
	n := len(records[0])
	if err != nil || len(records) != 3 || strings.Join(records[1][n-4:], ",") != "tr1,2,1,2" || strings.Join(records[2][n-4:], ",") != ",,," {
		t.Fatalf("events csv: %v %q", err, records)
	}
	res, body := get(t, srv, "/api/reports/events?pageSize=1", true)
	var page struct {
		Data     []map[string]any `json:"data"`
		Total    int              `json:"total"`
		PageSize int              `json:"pageSize"`
	}
	if res.StatusCode != 200 || json.Unmarshal(body, &page) != nil || page.Total != 2 || page.PageSize != 1 || page.Data[0]["trialNumber"] != float64(2) || page.Data[1]["trialId"] != nil {
		t.Fatalf("events page: %s", body)
	}
}

func TestSummaryResponses(t *testing.T) {
	a, b := 10.0, 20.0
	srv := server(t, store{metrics: []domain.MetricRow{
		{Provenance: provenance("t1", "s1", "g", 1), MetricKey: "distance_cm", Unit: "cm", Value: &a},
		{Provenance: provenance("t2", "s2", "g", 1), MetricKey: "distance_cm", Unit: "cm", Value: &b},
	}})
	path := "/api/reports/metric-summary?experimentId=01a00000-0000-7000-8000-000000000009&metricKey=distance_cm"
	res, body := get(t, srv, path, true)
	var summary struct {
		Version map[string]any   `json:"version"`
		Groups  []map[string]any `json:"groups"`
	}
	if res.StatusCode != 200 || json.Unmarshal(body, &summary) != nil || summary.Version["unit"] != "cm" || summary.Groups[0]["subjects"] != float64(2) || summary.Groups[0]["mean"] != float64(15) {
		t.Fatalf("summary: %d %s", res.StatusCode, body)
	}
	if res, body := get(t, srv, "/api/reports/metric-summary?metricKey=distance_cm", true); res.StatusCode != 400 || !strings.Contains(string(body), "report.invalidSummary") {
		t.Fatalf("summary without experiment: %d %s", res.StatusCode, body)
	}

	srv = server(t, store{metrics: []domain.MetricRow{
		{Provenance: provenance("t1", "s1", "g", 1), MetricKey: "distance_cm", Unit: "cm", Value: &a},
		{Provenance: provenance("t1", "s1", "g", 2), MetricKey: "distance_cm", Unit: "cm", Value: &b},
	}})
	res, body = get(t, srv, path, true)
	if res.StatusCode != 409 || !strings.Contains(string(body), "report.incompatibleVersions") || !strings.Contains(string(body), "OPEN_FIELD v1 engine 2 (cm)") {
		t.Fatalf("incompatible: %d %s", res.StatusCode, body)
	}
}

func TestExcelCSVIsUTF16WithTabsAndKeepsTurkishAndNegatives(t *testing.T) {
	value := -3.5
	srv := server(t, store{metrics: []domain.MetricRow{
		{Provenance: provenance("t1", "s1", "Kontrol Grubu ŞğüİıÖç", 1), MetricKey: "distance_cm", Unit: "cm", Value: &value},
	}})
	_, body := get(t, srv, "/api/reports/metrics/export?experimentId=01a00000-0000-7000-8000-000000000009&excel=1", true)
	if len(body) < 4 || body[0] != 0xFF || body[1] != 0xFE || len(body)%2 != 0 {
		t.Fatalf("not UTF-16 LE with BOM: % x", body[:4])
	}
	units := make([]uint16, 0, len(body)/2-1)
	for i := 2; i < len(body); i += 2 {
		units = append(units, uint16(body[i])|uint16(body[i+1])<<8)
	}
	r := csv.NewReader(strings.NewReader(string(utf16.Decode(units))))
	r.Comma = '\t'
	records, err := r.ReadAll()
	if err != nil || len(records) != 2 {
		t.Fatalf("excel csv: %v", err)
	}
	row := "|" + strings.Join(records[1], "|") + "|"
	if !strings.Contains(row, "|Kontrol Grubu ŞğüİıÖç|") || !strings.Contains(row, "|-3.5|") {
		t.Fatalf("row: %q", row)
	}
}
