package paradigmshttp

import (
	"encoding/json"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/paradigms/application"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCatalogIsReadOnlyAndVersioned(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, application.New(), func(r *http.Request) (access.Actor, error) {
		switch r.Header.Get("Authorization") {
		case "viewer":
			return access.Actor{UserID: "u", Role: access.Viewer}, nil
		case "unknown-role":
			return access.Actor{UserID: "u", Role: "ROOT"}, nil
		}
		return access.Actor{}, access.ErrUnauthenticated
	}, slog.New(slog.NewJSONHandler(io.Discard, nil)))

	call := func(method, path, who string) (int, map[string]any) {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", who)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		var body map[string]any
		_ = json.Unmarshal(rr.Body.Bytes(), &body)
		return rr.Code, body
	}
	for _, tc := range []struct {
		method, path, who string
		status            int
		code              string
	}{
		{"GET", "/api/paradigms", "", 401, "auth.unauthenticated"},
		{"GET", "/api/paradigms", "unknown-role", 403, "auth.forbidden"},
		{"GET", "/api/paradigms/MAZE", "viewer", 404, "paradigm.notFound"},
		{"GET", "/api/paradigms/open_field", "viewer", 404, "paradigm.notFound"},
		{"GET", "/api/paradigms/OPEN_FIELD/versions/2", "viewer", 404, "paradigm.versionNotFound"},
		{"GET", "/api/paradigms/OPEN_FIELD/versions/one", "viewer", 404, "paradigm.versionNotFound"},
		{"POST", "/api/paradigms", "viewer", 405, ""},
		{"PATCH", "/api/paradigms/OPEN_FIELD", "viewer", 405, ""},
		{"DELETE", "/api/paradigms/OPEN_FIELD/versions/1", "viewer", 405, ""},
	} {
		if status, body := call(tc.method, tc.path, tc.who); status != tc.status || (tc.code != "" && body["code"] != tc.code) {
			t.Errorf("%s %s as %q: %d %v", tc.method, tc.path, tc.who, status, body)
		}
	}

	status, list := call("GET", "/api/paradigms", "viewer")
	data, _ := list["data"].([]any)
	if status != 200 || len(data) != 1 {
		t.Fatalf("list: %d %v", status, list)
	}
	summary := data[0].(map[string]any)
	if summary["key"] != "OPEN_FIELD" || summary["latestVersion"] != float64(1) || summary["automatedAnalysis"] != false {
		t.Fatalf("summary: %v", summary)
	}
	for _, path := range []string{"/api/paradigms/OPEN_FIELD", "/api/paradigms/OPEN_FIELD/versions/1"} {
		status, manifest := call("GET", path, "viewer")
		metrics, _ := manifest["metrics"].([]any)
		if status != 200 || manifest["version"] != float64(1) || manifest["metricEngineVersion"] != float64(1) ||
			manifest["resultSchemaVersion"] != float64(1) || len(metrics) != 9 || manifest["automatedAnalysis"] != false {
			t.Fatalf("%s: %d %v", path, status, manifest)
		}
		first := metrics[0].(map[string]any)
		if first["key"] != "duration_s" || first["missingWhen"] == "" || first["tolerance"] == nil || first["valueType"] != "number" {
			t.Fatalf("metric contract: %v", first)
		}
	}
}
