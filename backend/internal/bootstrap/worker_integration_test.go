//go:build integration

package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	analysisapp "github.com/HappyHackingSpace/Misko/backend/internal/analysis/application"
	analysisdomain "github.com/HappyHackingSpace/Misko/backend/internal/analysis/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/config"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
	"golang.org/x/crypto/bcrypt"
)

const seedToken = "mw_dev_local_only_not_a_secret"

// TestWorkerBootstrap drives both token paths of issue #274: the dev seed that
// setup creates and the register-worker command's direct registration. Both
// must authenticate on the worker routes exactly like an API-registered worker.
func TestWorkerBootstrap(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.Database(t)
	mwm := []analysisdomain.Capability{{ParadigmKey: "MWM", ParadigmVersion: 1}}

	// Registration records the administrator as the creator, so setup comes first.
	if _, err := RegisterWorker(ctx, pool, analysisapp.WorkerInput{Name: "early", ModelVersion: "m 1", Capabilities: mwm}); !errors.Is(err, ErrNoAdministrator) {
		t.Fatalf("register before setup: %v", err)
	}

	setup, err := Setup(ctx, pool, config.Setup{AdminEmail: "admin@lab.io", LabName: "Lab", LabTimezone: "UTC", BcryptCost: bcrypt.MinCost,
		WorkerSeedToken: seedToken})
	if err != nil || !setup.WorkerSeeded || !setup.AdministratorCreated {
		t.Fatalf("setup with seed: %+v %v", setup, err)
	}
	again, err := Setup(ctx, pool, config.Setup{AdminEmail: "admin@lab.io", LabName: "Lab", LabTimezone: "UTC", BcryptCost: bcrypt.MinCost,
		WorkerSeedToken: seedToken})
	if err != nil || again.WorkerSeeded || again.AdministratorCreated {
		t.Fatalf("repeated setup must seed nothing: %+v %v", again, err)
	}

	registered, err := RegisterWorker(ctx, pool, analysisapp.WorkerInput{Name: "worker-1", ModelVersion: "misko-tracker-bgsub 2.2.0", Capabilities: mwm})
	if err != nil || !strings.HasPrefix(registered.Token, "mw_") || registered.Worker.ID == "" || registered.Worker.CreatedBy == "" {
		t.Fatalf("register-worker: %+v %v", registered, err)
	}
	if _, err := RegisterWorker(ctx, pool, analysisapp.WorkerInput{Name: "Worker-1", ModelVersion: "m 1", Capabilities: mwm}); !errors.Is(err, analysisapp.ErrWorkerNameTaken) {
		t.Fatalf("duplicate name: %v", err)
	}
	unknown := []analysisdomain.Capability{{ParadigmKey: "UNKNOWN", ParadigmVersion: 1}}
	if _, err := RegisterWorker(ctx, pool, analysisapp.WorkerInput{Name: "worker-2", ModelVersion: "m 1", Capabilities: unknown}); !errors.Is(err, analysisapp.ErrUnknownCapability) {
		t.Fatalf("unknown capability: %v", err)
	}

	// Both tokens pass worker authentication over HTTP; the identity model is
	// unchanged, so user JWTs are refused there and worker tokens on user routes.
	api, err := NewAPI(pool, config.Config{ProbeTimeout: time.Second},
		config.Auth{JWTSecret: []byte(strings.Repeat("s", 32)), JWTIssuer: "misko", JWTAudience: "misko-api", TokenTTL: time.Hour, BcryptCost: bcrypt.MinCost},
		slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.Handler)
	defer server.Close()
	call := func(method, path, authorization string, body any) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(body)
		req, _ := http.NewRequest(method, server.URL+path, bytes.NewReader(raw))
		if authorization != "" {
			req.Header.Set("Authorization", authorization)
		}
		res, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var decoded map[string]any
		_ = json.NewDecoder(res.Body).Decode(&decoded)
		return res.StatusCode, decoded
	}
	// No storage is configured, so an authenticated claim answers 503 where an
	// unauthenticated one answers 401.
	for name, token := range map[string]string{"seeded dev-worker": seedToken, "registered worker": registered.Token} {
		if status, body := call("POST", "/api/worker/claim", "Worker "+token, map[string]any{}); status != 503 || body["code"] != "media.storageNotConfigured" {
			t.Fatalf("%s claim: %d %v", name, status, body)
		}
	}
	if status, body := call("POST", "/api/worker/claim", "Worker mw_wrong-token", map[string]any{}); status != 401 || body["code"] != "worker.unauthenticated" {
		t.Fatalf("wrong token: %d %v", status, body)
	}
	status, body := call("POST", "/api/auth/login", "", map[string]string{"email": "admin@lab.io", "password": setup.AdminPassword})
	if status != 200 {
		t.Fatalf("login: %d %v", status, body)
	}
	admin := "Bearer " + body["token"].(string)
	if status, body := call("POST", "/api/worker/claim", admin, map[string]any{}); status != 401 || body["code"] != "worker.unauthenticated" {
		t.Fatalf("user JWT on a worker route: %d %v", status, body)
	}
	if status, _ := call("GET", "/api/analysis/workers", "Worker "+registered.Token, nil); status != 401 {
		t.Fatalf("worker token on a user route: %d", status)
	}
	status, body = call("GET", "/api/analysis/workers", admin, nil)
	if status != 200 {
		t.Fatalf("list workers: %d %v", status, body)
	}
	names := map[string]bool{}
	for _, w := range body["data"].([]any) {
		names[w.(map[string]any)["name"].(string)] = true
	}
	if !names["dev-worker"] || !names["worker-1"] {
		t.Fatalf("workers missing: %v", names)
	}
}
