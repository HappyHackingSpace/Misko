package paradigmshttp

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/paradigms/application"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

var update = flag.Bool("update", false, "write golden manifests for newly published paradigm versions")

func newMux() *http.ServeMux {
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
	return mux
}

func get(mux *http.ServeMux, method, path, who string) (int, []byte) {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", who)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return rr.Code, rr.Body.Bytes()
}

func TestCatalogIsReadOnlyAndVersioned(t *testing.T) {
	mux := newMux()
	call := func(method, path, who string) (int, map[string]any) {
		status, raw := get(mux, method, path, who)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		return status, body
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
		{"GET", "/api/paradigms/MWM", "", 401, "auth.unauthenticated"},
		{"POST", "/api/paradigms", "viewer", 405, ""},
		{"PATCH", "/api/paradigms/OPEN_FIELD", "viewer", 405, ""},
		{"PUT", "/api/paradigms/EPM", "viewer", 405, ""},
		{"DELETE", "/api/paradigms/OPEN_FIELD/versions/1", "viewer", 405, ""},
	} {
		if status, body := call(tc.method, tc.path, tc.who); status != tc.status || (tc.code != "" && body["code"] != tc.code) {
			t.Errorf("%s %s as %q: %d %v", tc.method, tc.path, tc.who, status, body)
		}
	}

	status, list := call("GET", "/api/paradigms", "viewer")
	data, _ := list["data"].([]any)
	want := []string{"BARNES_MAZE", "EPM", "LIGHT_DARK", "MWM", "NOVEL_OBJECT", "OPEN_FIELD", "POLE", "ROTAROD", "THREE_CHAMBER", "TREADMILL", "Y_MAZE"}
	if status != 200 || len(data) != len(want) {
		t.Fatalf("list: %d %v", status, list)
	}
	for i, item := range data {
		summary := item.(map[string]any)
		// No worker capability exists yet, so catalog presence must not claim automated analysis.
		if summary["key"] != want[i] || summary["latestVersion"] != float64(1) || summary["automatedAnalysis"] != false {
			t.Fatalf("summary %d: %v", i, summary)
		}
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

	_, epm := call("GET", "/api/paradigms/EPM/versions/1", "viewer")
	zone := epm["zones"].([]any)[0].(map[string]any)
	input := epm["inputEvents"].([]any)[0].(map[string]any)
	if zone["key"] != "center" || zone["calibrated"] != true || zone["required"] != true ||
		input["type"] != "risk_assessment" || input["kind"] != "POINT" || input["labels"] == nil {
		t.Fatalf("EPM contract: %v %v", zone, input)
	}
	_, rotarod := call("GET", "/api/paradigms/ROTAROD", "viewer")
	session := rotarod["sessionParameters"].([]any)[0].(map[string]any)
	zones, _ := rotarod["zones"].([]any)
	if session["key"] != "accelerating" || session["valueType"] != "integer" || zones == nil || len(zones) != 0 {
		t.Fatalf("ROTAROD contract: %v %v", session, rotarod["zones"])
	}
	_, novel := call("GET", "/api/paradigms/NOVEL_OBJECT", "viewer")
	exploration := novel["inputEvents"].([]any)[0].(map[string]any)
	if labels := exploration["labels"].([]any); len(labels) != 2 || labels[0] != "novel" {
		t.Fatalf("NOVEL_OBJECT labels: %v", exploration)
	}
}

// TestPublishedManifestsNeverChange pins every published manifest to a golden
// file. A changed contract must be published as a new version instead; run the
// test with -update only to add goldens for new versions.
func TestPublishedManifestsNeverChange(t *testing.T) {
	mux := newMux()
	_, raw := get(mux, "GET", "/api/paradigms", "viewer")
	var list struct {
		Data []struct {
			Key      string `json:"key"`
			Versions []int  `json:"versions"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatal(err)
	}
	var published []string
	for _, p := range list.Data {
		for _, v := range p.Versions {
			name := fmt.Sprintf("%s_v%d.json", p.Key, v)
			published = append(published, name)
			status, body := get(mux, "GET", fmt.Sprintf("/api/paradigms/%s/versions/%d", p.Key, v), "viewer")
			if status != 200 {
				t.Fatalf("%s: %d", name, status)
			}
			var got bytes.Buffer
			if err := json.Indent(&got, bytes.TrimSpace(body), "", "  "); err != nil {
				t.Fatal(err)
			}
			got.WriteByte('\n')
			path := filepath.Join("testdata", "manifests", name)
			want, err := os.ReadFile(path)
			if os.IsNotExist(err) && *update {
				if err := os.WriteFile(path, got.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
				continue
			}
			if err != nil {
				t.Fatalf("%s has no golden manifest: %v", name, err)
			}
			if !bytes.Equal(got.Bytes(), want) {
				t.Errorf("published manifest %s changed; publish a new version instead", name)
			}
		}
	}
	goldens, err := filepath.Glob(filepath.Join("testdata", "manifests", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range goldens {
		if !slices.Contains(published, filepath.Base(g)) {
			t.Errorf("golden %s has no published version; published versions cannot be removed", g)
		}
	}
	if len(goldens) < len(published) && !*update {
		t.Errorf("%d goldens for %d published versions", len(goldens), len(published))
	}
}
