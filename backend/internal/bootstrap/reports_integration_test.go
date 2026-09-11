//go:build integration

package bootstrap

import (
	"context"
	"encoding/csv"
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

func TestReportsOverHTTP(t *testing.T) {
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
	c.create("/api/users", admin, map[string]any{"email": "viewer@lab.io", "name": "Viewer", "role": "VIEWER", "password": "Password-123"})
	viewer := c.login("viewer@lab.io", "Password-123")
	exp := c.create("/api/experiments", admin, map[string]any{"code": "EXP-R", "title": "Reports"})

	page := c.expect("viewer reads metrics", c.call("GET", "/api/reports/metrics?experimentId="+id(exp)+"&metricKey=distance_cm", viewer, nil), 200, "").body
	if data, ok := page["data"].([]any); !ok || len(data) != 0 || page["total"] != float64(0) || page["pageSize"] != float64(20) {
		t.Fatalf("empty report: %v", page)
	}
	c.expect("events", c.call("GET", "/api/reports/events?selection=all&sort=eventType", viewer, nil), 200, "")
	c.expect("anonymous", c.call("GET", "/api/reports/metrics", "", nil), 401, "auth.unauthenticated")
	c.expect("malformed id", c.call("GET", "/api/reports/metrics?subjectId=S1", viewer, nil), 400, "report.invalidQuery")
	c.expect("summary without metric", c.call("GET", "/api/reports/metric-summary?experimentId="+id(exp), viewer, nil), 400, "report.invalidSummary")
	summary := c.expect("summary", c.call("GET", "/api/reports/metric-summary?experimentId="+id(exp)+"&metricKey=distance_cm", viewer, nil), 200, "").body
	if groups, ok := summary["groups"].([]any); !ok || len(groups) != 0 || summary["version"] != nil {
		t.Fatalf("empty summary: %v", summary)
	}

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/reports/events/export?experimentId="+id(exp), nil)
	req.Header.Set("Authorization", "Bearer "+viewer)
	res, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	records, err := csv.NewReader(res.Body).ReadAll()
	if res.StatusCode != 200 || err != nil || len(records) != 1 || records[0][0] != "runId" || res.Header.Get("Content-Type") != "text/csv; charset=utf-8" {
		t.Fatalf("csv export: %d %v %v", res.StatusCode, records, err)
	}
}
