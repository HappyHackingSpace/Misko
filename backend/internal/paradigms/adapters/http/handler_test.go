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
	Register(mux, application.New(nil), func(r *http.Request) (access.Actor, error) {
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
		{"GET", "/api/paradigms/mwm", "viewer", 404, "paradigm.notFound"},
		{"GET", "/api/paradigms/MWM/versions/2", "viewer", 404, "paradigm.versionNotFound"},
		{"GET", "/api/paradigms/MWM/versions/one", "viewer", 404, "paradigm.versionNotFound"},
		{"GET", "/api/paradigms/MWM", "", 401, "auth.unauthenticated"},
		{"POST", "/api/paradigms", "viewer", 405, ""},
		{"PATCH", "/api/paradigms/MWM", "viewer", 405, ""},
		{"PUT", "/api/paradigms/POLE", "viewer", 405, ""},
		{"DELETE", "/api/paradigms/MWM/versions/1", "viewer", 405, ""},
	} {
		if status, body := call(tc.method, tc.path, tc.who); status != tc.status || (tc.code != "" && body["code"] != tc.code) {
			t.Errorf("%s %s as %q: %d %v", tc.method, tc.path, tc.who, status, body)
		}
	}

	status, list := call("GET", "/api/paradigms", "viewer")
	data, _ := list["data"].([]any)
	want := []string{"MWM", "NOVEL_OBJECT", "POLE", "TREADMILL"}
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
	for _, path := range []string{"/api/paradigms/MWM", "/api/paradigms/MWM/versions/1"} {
		status, manifest := call("GET", path, "viewer")
		metrics, _ := manifest["metrics"].([]any)
		if status != 200 || manifest["version"] != float64(1) || manifest["metricEngineVersion"] != float64(1) ||
			manifest["resultSchemaVersion"] != float64(1) || len(metrics) != 8 || manifest["automatedAnalysis"] != false {
			t.Fatalf("%s: %d %v", path, status, manifest)
		}
		first := metrics[0].(map[string]any)
		if first["key"] != "duration_s" || first["missingWhen"] == "" || first["tolerance"] == nil || first["valueType"] != "number" {
			t.Fatalf("metric contract: %v", first)
		}
	}

	_, mwm := call("GET", "/api/paradigms/MWM/versions/1", "viewer")
	zone := mwm["zones"].([]any)[0].(map[string]any)
	inputs, _ := mwm["inputEvents"].([]any)
	if zone["key"] != "platform" || zone["calibrated"] != false || zone["required"] != true || inputs == nil || len(inputs) != 0 {
		t.Fatalf("MWM contract: %v %v", zone, mwm["inputEvents"])
	}
	_, pole := call("GET", "/api/paradigms/POLE/versions/1", "viewer")
	input := pole["inputEvents"].([]any)[0].(map[string]any)
	if input["type"] != "turn_complete" || input["kind"] != "POINT" || input["labels"] == nil {
		t.Fatalf("POLE contract: %v", input)
	}
	_, treadmill := call("GET", "/api/paradigms/TREADMILL", "viewer")
	session := treadmill["parameters"].([]any)[1].(map[string]any)
	zones, _ := treadmill["zones"].([]any)
	if session["key"] != "accelerating" || session["scope"] != "SESSION" || session["valueType"] != "integer" || zones == nil || len(zones) != 0 {
		t.Fatalf("TREADMILL contract: %v %v", session, treadmill["zones"])
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
func TestCatalogTextFollowsTheRequestedLanguage(t *testing.T) {
	mux := newMux()
	for _, tc := range []struct{ header, query, name, label string }{
		{"", "", "Morris water maze", "Pool diameter"},
		{"tr-TR,tr;q=0.9", "", "Morris su labirenti", "Havuz çapı"},
		{"en", "?lang=tr", "Morris su labirenti", "Havuz çapı"},
		{"de", "", "Morris water maze", "Pool diameter"},
	} {
		req := httptest.NewRequest("GET", "/api/paradigms/MWM"+tc.query, nil)
		req.Header.Set("Authorization", "viewer")
		if tc.header != "" {
			req.Header.Set("Accept-Language", tc.header)
		}
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		var m struct {
			Name       string `json:"name"`
			Parameters []struct {
				Label string `json:"label"`
				Scope string `json:"scope"`
			} `json:"parameters"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &m); err != nil || m.Name != tc.name || m.Parameters[0].Label != tc.label ||
			m.Parameters[0].Scope != "APPARATUS" || rr.Header().Get("Content-Language") == "" {
			t.Errorf("%+v: %d %+v %v", tc, rr.Code, m, err)
		}
	}
}

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

func TestListLinksEachParadigmToAnArticleInTheRequestedLanguage(t *testing.T) {
	mux := newMux()
	// Without a Turkish article the English one is linked in Turkish too.
	for _, query := range []string{"", "?lang=tr"} {
		_, raw := get(mux, "GET", "/api/paradigms"+query, "viewer")
		var list struct {
			Data []struct {
				Key       string `json:"key"`
				Reference string `json:"reference"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &list); err != nil {
			t.Fatal(err)
		}
		links := map[string]string{}
		for _, p := range list.Data {
			links[p.Key] = p.Reference
		}
		if links["MWM"] != "https://en.wikipedia.org/wiki/Morris_water_navigation_task" || links["POLE"] != "" {
			t.Errorf("%q: %v", query, links)
		}
	}
}
