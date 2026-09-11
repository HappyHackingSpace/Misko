// Command e2e runs the API for browser tests with storage on local disk.
//
// It installs the schema into an empty database whose name contains "e2e",
// creates the administrator, seeds one analyzed Open Field test through the real
// API (including the worker protocol) and serves signed upload and read URLs
// itself. It is test support: it never talks to Cloud Storage and must not run
// against a real installation.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"hash/crc32"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/HappyHackingSpace/Misko/backend/internal/bootstrap"
	mediaapp "github.com/HappyHackingSpace/Misko/backend/internal/media/application"
	mediadomain "github.com/HappyHackingSpace/Misko/backend/internal/media/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/config"
	"github.com/HappyHackingSpace/Misko/backend/schema"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func main() { os.Exit(run()) }

func run() int {
	addr := flag.String("addr", "127.0.0.1:4010", "listen address")
	fixtures := flag.String("fixtures", "../frontend/tests/fixtures", "directory with analyzed.mp4 and trajectory.json")
	flag.Parse()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	database := os.Getenv("E2E_DATABASE_URL")
	if !strings.Contains(database, "e2e") {
		fmt.Fprintln(os.Stderr, "E2E_DATABASE_URL must name an empty database whose name contains \"e2e\"; it is dropped and recreated")
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, database)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid E2E_DATABASE_URL")
		return 1
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, "DROP SCHEMA IF EXISTS misko CASCADE"); err != nil {
		fmt.Fprintln(os.Stderr, "could not reset the schema:", err)
		return 1
	}
	if err := schema.Install(ctx, pool); err != nil {
		fmt.Fprintln(os.Stderr, "schema installation failed:", err)
		return 1
	}
	setup, err := bootstrap.Setup(ctx, pool, config.Setup{AdminEmail: "admin@e2e.local", LabName: "End to end laboratory", LabTimezone: "UTC", BcryptCost: bcrypt.MinCost})
	if err != nil {
		fmt.Fprintln(os.Stderr, "setup failed:", err)
		return 1
	}

	store := &diskStore{base: "http://" + *addr, objects: map[string]storedObject{}, sessions: map[string]*session{}}
	api, err := bootstrap.NewAPI(pool, config.Config{ProbeTimeout: 2 * time.Second},
		config.Auth{JWTSecret: []byte(strings.Repeat("e2e-signing-secret", 2)), JWTIssuer: "misko", JWTAudience: "misko-api", TokenTTL: 12 * time.Hour, BcryptCost: bcrypt.MinCost},
		logger, bootstrap.WithStorage(store, mediaapp.Settings{MaxBytes: 1 << 30, UploadTTL: time.Hour, ReadTTL: time.Hour}))
	if err != nil {
		fmt.Fprintln(os.Stderr, "wiring failed:", err)
		return 1
	}

	seed, err := seedAnalyzedTest(ctx, api, store, *fixtures, setup.AdminPassword)
	if err != nil {
		fmt.Fprintln(os.Stderr, "seeding failed:", err)
		return 1
	}
	seed["adminEmail"], seed["adminPassword"] = "admin@e2e.local", setup.AdminPassword

	mux := http.NewServeMux()
	mux.Handle("/api/", api.Handler)
	mux.HandleFunc("GET /e2e/seed", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, seed) })
	store.routes(mux)
	server := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	body, _ := json.Marshal(seed)
	fmt.Println(string(body))
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, "server stopped:", err)
		return 1
	}
	return 0
}

// storedObject is a finished upload.
type storedObject struct {
	data        []byte
	contentType string
	generation  int64
	crc         uint32
}

type session struct {
	object      string
	contentType string
	data        []byte
}

// diskStore serves signed upload and read URLs from memory. Browsers reach it
// cross-origin, so it answers the CORS preflight and exposes Location, the way
// a bucket's CORS policy must for the real upload.
type diskStore struct {
	mu         sync.Mutex
	base       string
	objects    map[string]storedObject
	sessions   map[string]*session
	generation int64
}

func (s *diskStore) Bucket() string { return "misko-e2e" }

func (s *diskStore) UploadURL(_ context.Context, object, contentType string, expires time.Time) (mediaapp.SignedRequest, error) {
	return mediaapp.SignedRequest{
		Method:    http.MethodPost,
		URL:       s.base + "/e2e/storage/start?object=" + url.QueryEscape(object) + "&contentType=" + url.QueryEscape(contentType),
		Headers:   map[string]string{"Content-Type": contentType, "x-goog-resumable": "start", "x-goog-if-generation-match": "0"},
		ExpiresAt: expires,
	}, nil
}

func (s *diskStore) ReadURL(_ context.Context, object string, generation int64, _ time.Time) (string, error) {
	return s.base + "/e2e/storage/object?object=" + url.QueryEscape(object) + "&generation=" + strconv.FormatInt(generation, 10), nil
}

func (s *diskStore) Attrs(_ context.Context, object string) (mediadomain.ObjectAttrs, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.objects[object]
	if !ok {
		return mediadomain.ObjectAttrs{}, mediaapp.ErrObjectNotFound
	}
	return mediadomain.ObjectAttrs{Generation: stored.generation, Size: int64(len(stored.data)), CRC32C: stored.crc, ContentType: stored.contentType}, nil
}

func (s *diskStore) Read(_ context.Context, object string, generation, limit int64) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.objects[object]
	if !ok || stored.generation != generation {
		return nil, mediaapp.ErrObjectNotFound
	}
	if int64(len(stored.data)) > limit {
		return nil, fmt.Errorf("object exceeds %d bytes", limit)
	}
	return stored.data, nil
}

// put stores bytes directly, for seeding.
func (s *diskStore) put(object string, data []byte, contentType string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.generation++
	s.objects[object] = storedObject{data: data, contentType: contentType, generation: 1_757_600_000_000_000 + s.generation, crc: crc32c(data)}
}

func (s *diskStore) routes(mux *http.ServeMux) {
	mux.HandleFunc("OPTIONS /e2e/storage/{rest...}", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /e2e/storage/start", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		object, contentType := r.URL.Query().Get("object"), r.URL.Query().Get("contentType")
		s.mu.Lock()
		id := fmt.Sprintf("s%d", len(s.sessions)+1)
		s.sessions[id] = &session{object: object, contentType: contentType}
		s.mu.Unlock()
		w.Header().Set("Location", s.base+"/e2e/storage/session?id="+id)
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("PUT /e2e/storage/session", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		s.mu.Lock()
		defer s.mu.Unlock()
		current, ok := s.sessions[r.URL.Query().Get("id")]
		if !ok {
			http.Error(w, "unknown session", http.StatusNotFound)
			return
		}
		chunk, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read failed", http.StatusBadRequest)
			return
		}
		current.data = append(current.data, chunk...)
		if raw := r.Header.Get("Content-Range"); raw != "" {
			// "bytes first-last/total": keep uploading while bytes remain.
			if _, after, found := strings.Cut(raw, "/"); found {
				if declared, err := strconv.Atoi(strings.TrimSpace(after)); err == nil && len(current.data) < declared {
					w.Header().Set("Range", fmt.Sprintf("bytes=0-%d", len(current.data)-1))
					w.WriteHeader(308)
					return
				}
			}
		}
		s.generation++
		s.objects[current.object] = storedObject{data: current.data, contentType: current.contentType, generation: 1_757_600_000_000_000 + s.generation, crc: crc32c(current.data)}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /e2e/storage/object", func(w http.ResponseWriter, r *http.Request) {
		cors(w)
		s.mu.Lock()
		stored, ok := s.objects[r.URL.Query().Get("object")]
		s.mu.Unlock()
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", stored.contentType)
		// ServeContent adds Accept-Ranges and answers Range requests, so players can seek.
		http.ServeContent(w, r, "", time.Unix(0, 0), bytes.NewReader(stored.data))
	})
}

func cors(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Range, Range, x-goog-resumable, x-goog-if-generation-match")
	w.Header().Set("Access-Control-Expose-Headers", "Location, Range, Content-Range, ETag")
	w.Header().Set("Access-Control-Max-Age", "3600")
}

func crc32c(data []byte) uint32 {
	return crc32.Checksum(data, crc32.MakeTable(crc32.Castagnoli))
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// caller drives the API in process, the way a browser or a worker would.
type caller struct {
	handler http.Handler
	token   string
	worker  string
}

func (c *caller) do(method, path string, body any) (map[string]any, error) {
	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = bytes.NewReader(raw)
	}
	request := httptest.NewRequest(method, path, payload)
	request.Header.Set("Content-Type", "application/json")
	switch {
	case c.worker != "":
		request.Header.Set("Authorization", "Worker "+c.worker)
	case c.token != "":
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	recorder := httptest.NewRecorder()
	c.handler.ServeHTTP(recorder, request)
	if recorder.Code >= 400 {
		return nil, fmt.Errorf("%s %s: %d %s", method, path, recorder.Code, strings.TrimSpace(recorder.Body.String()))
	}
	out := map[string]any{}
	if recorder.Body.Len() > 0 {
		if err := json.Unmarshal(recorder.Body.Bytes(), &out); err != nil {
			return nil, fmt.Errorf("%s %s: %w", method, path, err)
		}
	}
	return out, nil
}

func text(m map[string]any, key string) string {
	value, _ := m[key].(string)
	return value
}

func nested(m map[string]any, key string) map[string]any {
	value, _ := m[key].(map[string]any)
	return value
}

// point is one calibration correspondence of the fixture's camera.
type point struct{ worldX, worldY, pixelX, pixelY float64 }

// The synthetic fixture's camera: a 320 x 240 view of a 50 x 50 cm arena
// (worker/tests/synthetic.py). Corners fit the transform, the other three check it.
// clipStartUs is where the analyzed clip begins inside the fixture video. It is
// not zero on purpose: the panel has to offset both players to play an event.
const clipStartUs int64 = 1_000_000

var fitPoints = []point{{0, 0, 52, 28}, {50, 0, 268, 36}, {50, 50, 292, 222}, {0, 50, 30, 214}}
var checkPoints = []point{{25, 25, 160.9466, 116.0652}, {10, 40, 85.7359, 172.3152}, {40, 10, 227.6114, 66.2066}}

func correspondences(points []point) []map[string]float64 {
	out := make([]map[string]float64, 0, len(points))
	for _, p := range points {
		out = append(out, map[string]float64{"pixelX": p.pixelX, "pixelY": p.pixelY, "worldX": p.worldX, "worldY": p.worldY})
	}
	return out
}

// seedAnalyzedTest creates one experiment with an analyzed Open Field test and
// returns the identifiers the browser test needs.
func seedAnalyzedTest(ctx context.Context, api bootstrap.API, store *diskStore, fixtures, adminPassword string) (map[string]any, error) {
	overlay, err := os.ReadFile(fixtures + "/analyzed.mp4")
	if err != nil {
		return nil, err
	}
	trajectory, err := os.ReadFile(fixtures + "/trajectory.json")
	if err != nil {
		return nil, err
	}
	c := &caller{handler: api.Handler}
	session, err := c.do(http.MethodPost, "/api/auth/login", map[string]any{"email": "admin@e2e.local", "password": adminPassword})
	if err != nil {
		return nil, err
	}
	c.token = text(session, "token")

	subject, err := c.do(http.MethodPost, "/api/subjects", map[string]any{"code": "E2E-1", "species": "MOUSE", "sex": "FEMALE"})
	if err != nil {
		return nil, err
	}
	experiment, err := c.do(http.MethodPost, "/api/experiments", map[string]any{"code": "E2E", "title": "End to end study"})
	if err != nil {
		return nil, err
	}
	experimentID := text(experiment, "id")
	enrollment, err := c.do(http.MethodPost, "/api/experiments/"+experimentID+"/enrollments",
		map[string]any{"subjectId": text(subject, "id"), "enrolledAt": time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339)})
	if err != nil {
		return nil, err
	}
	environment, err := c.do(http.MethodPost, "/api/environments", map[string]any{
		"name": "E2E arena", "paradigmKey": "OPEN_FIELD",
		"revision": map[string]any{"paradigmVersion": 1, "apparatus": map[string]any{"arena_width_cm": 50, "arena_height_cm": 50, "center_fraction": 0.5}},
	})
	if err != nil {
		return nil, err
	}
	protocol, err := c.do(http.MethodPost, "/api/experiments/"+experimentID+"/protocols", map[string]any{
		"name": "Open field", "version": map[string]any{"steps": []any{map[string]any{
			"position": 1, "paradigmKey": "OPEN_FIELD", "paradigmVersion": 1,
			"environmentRevisionId": text(nested(environment, "revision"), "id"), "trialType": "STANDARD", "trials": 1,
		}}},
	})
	if err != nil {
		return nil, err
	}
	test, err := c.do(http.MethodPost, "/api/experiments/"+experimentID+"/tests", map[string]any{
		"enrollmentId": text(enrollment, "id"), "protocolVersionId": text(nested(protocol, "version"), "id"),
		"stepPosition": 1, "scheduledAt": time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		return nil, err
	}
	testID := text(test, "id")
	if _, err := c.do(http.MethodPost, "/api/tests/"+testID+"/start", map[string]any{}); err != nil {
		return nil, err
	}

	// The uploaded source is the fixture video; the browser test uploads its own.
	// The clip starts one second in, so the analyzed window is shorter than the
	// video and both players need a real offset to stay on the same timeline.
	started, err := c.do(http.MethodPost, "/api/tests/"+testID+"/recordings", map[string]any{
		"fileName": "source.mp4", "contentType": "video/mp4", "sizeBytes": len(overlay), "crc32c": mediadomain.FormatCRC32C(crc32c(overlay)),
		"clipStartUs": clipStartUs, "clipEndUs": 4_000_000,
	})
	if err != nil {
		return nil, err
	}
	recording := nested(started, "recording")
	recordingID := text(recording, "id")
	uploaded, err := url.Parse(text(nested(started, "upload"), "url"))
	if err != nil {
		return nil, err
	}
	store.put(uploaded.Query().Get("object"), overlay, "video/mp4")
	if _, err := c.do(http.MethodPost, "/api/tests/"+testID+"/recordings/"+recordingID+"/finalize", map[string]any{}); err != nil {
		return nil, err
	}
	if _, err := c.do(http.MethodPost, "/api/tests/"+testID+"/recordings/"+recordingID+"/calibrations", map[string]any{
		"cameraId": "e2e-camera", "frameWidth": 320, "frameHeight": 240,
		"crop": map[string]any{"x": 0, "y": 0, "width": 320, "height": 240}, "referenceFrameUs": 0, "measurementPlane": "ARENA_FLOOR",
		"fitPoints": correspondences(fitPoints), "checkPoints": correspondences(checkPoints),
	}); err != nil {
		return nil, err
	}

	registered, err := c.do(http.MethodPost, "/api/analysis/workers", map[string]any{
		"name": "e2e tracker", "modelVersion": "misko-open-field-bgsub 1.0.0",
		"capabilities": []any{map[string]any{"paradigmKey": "OPEN_FIELD", "paradigmVersion": 1}},
	})
	if err != nil {
		return nil, err
	}
	if err := api.AnalysisTick(ctx); err != nil {
		return nil, err
	}
	worker := &caller{handler: api.Handler, worker: text(registered, "token")}
	job, err := worker.do(http.MethodPost, "/api/worker/claim", map[string]any{})
	if err != nil {
		return nil, err
	}
	run := nested(job, "run")
	runID, attempt := text(run, "id"), int(nested(job, "run")["attempt"].(float64))
	prefix := text(job, "outputPrefix")

	// The fixture trajectory belongs to another run and to the whole video: point
	// it at this run and at the clip that was analyzed.
	patched, err := clipTo(trajectory, runID, attempt, clipStartUs, 4_000_000)
	if err != nil {
		return nil, err
	}
	for _, output := range []struct {
		kind, name, contentType string
		data                    []byte
	}{
		{"ANALYZED_VIDEO", "overlay.mp4", "video/mp4", overlay},
		{"TRAJECTORY", "trajectory.json", "application/json", patched},
	} {
		if _, err := worker.do(http.MethodPost, "/api/worker/runs/"+runID+"/outputs", map[string]any{
			"attempt": attempt, "kind": output.kind, "fileName": output.name, "contentType": output.contentType,
			"sizeBytes": len(output.data), "crc32c": mediadomain.FormatCRC32C(crc32c(output.data)),
		}); err != nil {
			return nil, err
		}
		store.put(prefix+output.name, output.data, output.contentType)
	}
	result, err := worker.do(http.MethodPost, "/api/worker/runs/"+runID+"/result", map[string]any{
		"attempt": attempt, "modelVersion": "misko-open-field-bgsub 1.0.0",
		"artifacts": []any{
			map[string]any{"kind": "ANALYZED_VIDEO", "objectName": prefix + "overlay.mp4"},
			map[string]any{"kind": "TRAJECTORY", "objectName": prefix + "trajectory.json"},
		},
		// The worker rendered the whole recording, so a clip time is that far into
		// both videos: source offset is the clip start the API requires, and the
		// output offset is the same because the analyzed video covers the video too.
		"pair": map[string]any{"analyzedObjectName": prefix + "overlay.mp4", "sourceOffsetUs": clipStartUs, "outputOffsetUs": clipStartUs, "timeMappingVersion": "identity-v1"},
	})
	if err != nil {
		return nil, err
	}
	if text(result, "status") != "SUCCEEDED" {
		return nil, fmt.Errorf("seeded run is %s", text(result, "status"))
	}
	// A read-only account, and a second recording whose analysis fails quality
	// control, so the panel's roles and failure states have real data.
	if _, err := c.do(http.MethodPost, "/api/users", map[string]any{
		"email": "viewer@e2e.local", "name": "Viewer", "role": "VIEWER", "password": viewerPassword,
	}); err != nil {
		return nil, err
	}
	failedRunID, err := seedFailedRun(ctx, api, store, c, worker, testID, overlay, trajectory)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"experimentId": experimentID, "experimentCode": "E2E", "subjectCode": "E2E-1",
		"testId": testID, "recordingId": recordingID, "runId": runID, "failedRunId": failedRunID,
		"viewerEmail": "viewer@e2e.local", "viewerPassword": viewerPassword,
	}, nil
}

const viewerPassword = "Viewer-password-1"

// seedFailedRun analyzes a second recording of the same test with a trajectory
// that lost half of its samples, which the API refuses on quality control.
func seedFailedRun(ctx context.Context, api bootstrap.API, store *diskStore, c, worker *caller, testID string, overlay, trajectory []byte) (string, error) {
	started, err := c.do(http.MethodPost, "/api/tests/"+testID+"/recordings", map[string]any{
		"fileName": "second.mp4", "contentType": "video/mp4", "sizeBytes": len(overlay), "crc32c": mediadomain.FormatCRC32C(crc32c(overlay)),
		"clipStartUs": 0, "clipEndUs": 4_000_000,
	})
	if err != nil {
		return "", err
	}
	recordingID := text(nested(started, "recording"), "id")
	uploaded, err := url.Parse(text(nested(started, "upload"), "url"))
	if err != nil {
		return "", err
	}
	store.put(uploaded.Query().Get("object"), overlay, "video/mp4")
	if _, err := c.do(http.MethodPost, "/api/tests/"+testID+"/recordings/"+recordingID+"/finalize", map[string]any{}); err != nil {
		return "", err
	}
	if _, err := c.do(http.MethodPost, "/api/tests/"+testID+"/recordings/"+recordingID+"/calibrations", map[string]any{
		"cameraId": "e2e-camera", "frameWidth": 320, "frameHeight": 240,
		"crop": map[string]any{"x": 0, "y": 0, "width": 320, "height": 240}, "referenceFrameUs": 0, "measurementPlane": "ARENA_FLOOR",
		"fitPoints": correspondences(fitPoints), "checkPoints": correspondences(checkPoints),
	}); err != nil {
		return "", err
	}
	if err := api.AnalysisTick(ctx); err != nil {
		return "", err
	}
	job, err := worker.do(http.MethodPost, "/api/worker/claim", map[string]any{})
	if err != nil {
		return "", err
	}
	run := nested(job, "run")
	runID, attempt := text(run, "id"), int(run["attempt"].(float64))
	prefix := text(job, "outputPrefix")

	lossy, err := loseHalfTheSamples(trajectory, runID, attempt)
	if err != nil {
		return "", err
	}
	for _, output := range []struct {
		kind, name, contentType string
		data                    []byte
	}{
		{"ANALYZED_VIDEO", "overlay.mp4", "video/mp4", overlay},
		{"TRAJECTORY", "trajectory.json", "application/json", lossy},
	} {
		if _, err := worker.do(http.MethodPost, "/api/worker/runs/"+runID+"/outputs", map[string]any{
			"attempt": attempt, "kind": output.kind, "fileName": output.name, "contentType": output.contentType,
			"sizeBytes": len(output.data), "crc32c": mediadomain.FormatCRC32C(crc32c(output.data)),
		}); err != nil {
			return "", err
		}
		store.put(prefix+output.name, output.data, output.contentType)
	}
	result, err := worker.do(http.MethodPost, "/api/worker/runs/"+runID+"/result", map[string]any{
		"attempt": attempt, "modelVersion": "misko-open-field-bgsub 1.0.0",
		"artifacts": []any{
			map[string]any{"kind": "ANALYZED_VIDEO", "objectName": prefix + "overlay.mp4"},
			map[string]any{"kind": "TRAJECTORY", "objectName": prefix + "trajectory.json"},
		},
		"pair": map[string]any{"analyzedObjectName": prefix + "overlay.mp4", "sourceOffsetUs": 0, "outputOffsetUs": 0, "timeMappingVersion": "identity-v1"},
	})
	if err != nil {
		return "", err
	}
	if text(result, "status") != "FAILED" {
		return "", fmt.Errorf("the lossy run is %s, not FAILED", text(result, "status"))
	}
	return runID, nil
}

// clipTo rebases the fixture trajectory, which covers the whole video, onto the
// clip that was analyzed: samples before the clip start are dropped and the rest
// are measured from it, which is what the API requires of a clipped recording.
func clipTo(trajectory []byte, runID string, attempt int, startUs, endUs int64) ([]byte, error) {
	var document map[string]any
	if err := json.Unmarshal(trajectory, &document); err != nil {
		return nil, err
	}
	document["runId"], document["attempt"], document["durationUs"] = runID, attempt, endUs-startUs
	samples, ok := document["samples"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("trajectory has no samples")
	}
	times, _ := samples["tUs"].([]any)
	keep := make([]int, 0, len(times))
	for i, t := range times {
		at, ok := t.(float64)
		if !ok {
			return nil, fmt.Errorf("sample %d has no time", i)
		}
		if int64(at) >= startUs && int64(at) <= endUs {
			keep = append(keep, i)
		}
	}
	if len(keep) == 0 {
		return nil, fmt.Errorf("the clip keeps no samples")
	}
	for _, key := range []string{"tUs", "xCm", "yCm", "tracked", "confidence"} {
		column, _ := samples[key].([]any)
		clipped := make([]any, 0, len(keep))
		for _, i := range keep {
			value := column[i]
			if key == "tUs" {
				value = float64(int64(value.(float64)) - startUs)
			}
			clipped = append(clipped, value)
		}
		samples[key] = clipped
	}
	return json.Marshal(document)
}

// loseHalfTheSamples marks every second sample untracked, which fails the
// paradigm's max_lost_frame_ratio rule.
func loseHalfTheSamples(trajectory []byte, runID string, attempt int) ([]byte, error) {
	var document map[string]any
	if err := json.Unmarshal(trajectory, &document); err != nil {
		return nil, err
	}
	document["runId"], document["attempt"], document["durationUs"] = runID, attempt, 4_000_000
	samples, ok := document["samples"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("trajectory has no samples")
	}
	tracked, _ := samples["tracked"].([]any)
	x, _ := samples["xCm"].([]any)
	y, _ := samples["yCm"].([]any)
	confidence, _ := samples["confidence"].([]any)
	for i := range tracked {
		if i%2 == 1 {
			tracked[i], x[i], y[i], confidence[i] = false, nil, nil, 0.0
		}
	}
	return json.Marshal(document)
}
