// Command e2e runs the API for browser tests with storage in memory.
//
// It installs the schema into an empty database whose name contains "e2e",
// creates the administrator, seeds two running studies through the real API
// (users per role, catalog entries, calibrated arenas, groups with regimens,
// timepoints, a protocol, allocated and weighed animals, doses, planned and
// in-progress test sessions with verified videos), registers an analysis
// worker and drives it through the worker protocol (claim, outputs, result) so
// there are results to review, and serves signed upload and read URLs itself.
// It is test support: it never talks to Cloud Storage and must not run against
// a real installation.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	fixtures := flag.String("fixtures", "../frontend/tests/fixtures", "directory with analyzed.mp4 (the seeded video) and trajectory.json")
	// Signed reads last an hour by default. The browser test for an expiring
	// URL runs its own server with a short one instead of waiting.
	readTTL := flag.Duration("read-ttl", time.Hour, "how long a signed read URL stays valid")
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
	if _, err := schema.Migrate(ctx, pool); err != nil {
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
		logger, bootstrap.WithStorage(store, mediaapp.Settings{MaxBytes: 1 << 30, UploadTTL: time.Hour, ReadTTL: *readTTL}))
	if err != nil {
		fmt.Fprintln(os.Stderr, "wiring failed:", err)
		return 1
	}

	seed, err := seedStudy(api, store, *fixtures, setup.AdminPassword)
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
	// limit is the signed x-goog-content-length-range maximum; 0 has none.
	limit int
	data  []byte
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

// SizedUploadURL limits the upload to size bytes, as the signed
// x-goog-content-length-range header does on a bucket.
func (s *diskStore) SizedUploadURL(ctx context.Context, object, contentType string, size int64, expires time.Time) (mediaapp.SignedRequest, error) {
	signed, err := s.UploadURL(ctx, object, contentType, expires)
	signed.URL += "&limit=" + strconv.FormatInt(size, 10)
	signed.Headers["x-goog-content-length-range"] = "0," + strconv.FormatInt(size, 10)
	return signed, err
}

// ReadURL signs a read that stops working after `expires`, the way a signed
// bucket URL does, so the panel's refresh path can be driven from a test.
func (s *diskStore) ReadURL(_ context.Context, object string, generation int64, expires time.Time) (string, error) {
	return s.base + "/e2e/storage/object?object=" + url.QueryEscape(object) +
		"&generation=" + strconv.FormatInt(generation, 10) +
		"&expires=" + strconv.FormatInt(expires.UnixMilli(), 10), nil
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
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		s.mu.Lock()
		id := fmt.Sprintf("s%d", len(s.sessions)+1)
		s.sessions[id] = &session{object: object, contentType: contentType, limit: limit}
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
		if current.limit > 0 && len(current.data)+len(chunk) > current.limit {
			http.Error(w, "the upload exceeds the signed content length range", http.StatusBadRequest)
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
		query := r.URL.Query()
		// A signed read stops working once it expires, and it names one
		// generation: a bucket refuses both, and so does this.
		if raw := query.Get("expires"); raw != "" {
			at, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || time.Now().After(time.UnixMilli(at)) {
				http.Error(w, "the signed read expired", http.StatusForbidden)
				return
			}
		}
		s.mu.Lock()
		stored, ok := s.objects[query.Get("object")]
		s.mu.Unlock()
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if raw := query.Get("generation"); raw != "" && raw != strconv.FormatInt(stored.generation, 10) {
			http.Error(w, "the object moved on to another generation", http.StatusNotFound)
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
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Range, Range, x-goog-resumable, x-goog-if-generation-match, x-goog-content-length-range")
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
var fitPoints = []point{{0, 0, 52, 28}, {50, 0, 268, 36}, {50, 50, 292, 222}, {0, 50, 30, 214}}
var checkPoints = []point{{25, 25, 160.9466, 116.0652}, {10, 40, 85.7359, 172.3152}, {40, 10, 227.6114, 66.2066}}

func correspondences(points []point) []map[string]float64 {
	out := make([]map[string]float64, 0, len(points))
	for _, p := range points {
		out = append(out, map[string]float64{"pixelX": p.pixelX, "pixelY": p.pixelY, "worldX": p.worldX, "worldY": p.worldY})
	}
	return out
}

// Passwords of the seeded accounts, one per role below the administrator.
var rolePasswords = map[string]string{
	"LAB_MANAGER": "Manager-password-1",
	"RESEARCHER":  "Researcher-password-1",
	"TECHNICIAN":  "Technician-password-1",
	"VIEWER":      "Viewer-password-1",
}

// seeder creates rows through the API and stops at the first failure.
type seeder struct {
	c   *caller
	err error
}

func (s *seeder) post(path string, body any) map[string]any {
	if s.err != nil {
		return map[string]any{}
	}
	out, err := s.c.do(http.MethodPost, path, body)
	if err != nil {
		s.err = err
		return map[string]any{}
	}
	return out
}

// seedStudy builds two running studies in the lab's time zone (UTC) and drives
// their analysis through the worker protocol.
//
// The main study (E2E): day 0 was three days ago, so daily doses are due today.
// It has an overdue planned baseline session, a baseline session in progress
// with two animals whose results wait for review (M02 has a newer run as well),
// and a Day 14 session with three animals: two accepted results and one still
// pending, so the study report has numbers and a provisional banner.
//
// The blinded study (E2E-BLIND) has one session with two pending results.
//
// It returns the identifiers and credentials the browser tests need.
func seedStudy(api bootstrap.API, store *diskStore, fixtures, adminPassword string) (map[string]any, error) {
	video, err := os.ReadFile(fixtures + "/analyzed.mp4")
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
	s := &seeder{c: c}

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	dayZero := today.AddDate(0, 0, -3)
	at := func(t time.Time) string { return t.Format(time.RFC3339) }
	// Sessions running now were scheduled an hour ago, started half an hour
	// ago and filmed a trial since, all on today's date even just after midnight.
	scheduled := now.Add(-time.Hour)
	if scheduled.Before(today) {
		scheduled = today
	}
	started := scheduled.Add(now.Sub(scheduled) / 2)
	trialStart, trialEnd := started.Add(now.Sub(started)/4), started.Add(now.Sub(started)/2)

	accounts := map[string]any{}
	for _, role := range []string{"LAB_MANAGER", "RESEARCHER", "TECHNICIAN", "VIEWER"} {
		email := strings.ToLower(strings.ReplaceAll(role, "_", "")) + "@e2e.local"
		s.post("/api/users", map[string]any{"email": email, "name": strings.ToLower(role), "role": role, "password": rolePasswords[role]})
		accounts[role] = map[string]any{"email": email, "password": rolePasswords[role]}
	}

	// Catalog.
	substance := func(name, kind string) string {
		return text(s.post("/api/catalog/substances", map[string]any{"name": name, "kind": kind}), "id")
	}
	mptp, saline, compound := substance("MPTP", "INDUCER"), substance("Saline", "VEHICLE"), substance("Compound X", "COMPOUND")
	substance("L-DOPA", "REFERENCE_DRUG")
	model := text(s.post("/api/catalog/disease-models", map[string]any{"name": "MPTP Parkinson", "inductionType": "CHEMICAL",
		"confirmationCriterion": "Rotarod latency below 50% of baseline"}), "id")

	// Three calibrated open field arenas filmed by one camera.
	arena := func(name string) (string, string) {
		created := s.post("/api/environments", map[string]any{"name": name, "paradigmKey": "OPEN_FIELD",
			"revision": map[string]any{"paradigmVersion": 1, "parameters": map[string]any{"arena_width_cm": 50, "arena_height_cm": 50}}})
		id := text(nested(created, "environment"), "id")
		s.post("/api/environments/"+id+"/revisions/1/calibrations", map[string]any{
			"cameraId": "e2e-camera", "frameWidth": 320, "frameHeight": 240,
			"crop": map[string]any{"x": 0, "y": 0, "width": 320, "height": 240}, "measurementPlane": "ARENA_FLOOR",
			"fitPoints": correspondences(fitPoints), "checkPoints": correspondences(checkPoints),
		})
		return id, text(nested(created, "revision"), "id")
	}
	boxA, boxARevision := arena("Box A")
	boxB, boxBRevision := arena("Box B")
	boxC, boxCRevision := arena("Box C")

	// The study design.
	experiment := s.post("/api/experiments", map[string]any{"code": "E2E", "title": "Compound X in MPTP mice",
		"hypothesis": "Compound X rescues the motor deficit", "randomisation": "Block randomisation"})
	experimentID := text(experiment, "id")
	base := "/api/experiments/" + experimentID
	group := func(code, name, role string, diseased bool) string {
		body := map[string]any{"code": code, "name": name, "role": role, "targetSize": 3}
		if diseased {
			body["diseaseModelId"] = model
		}
		return text(s.post(base+"/groups", body), "id")
	}
	healthy := group("G1", "Healthy", "HEALTHY_CONTROL", false)
	disease := group("G2", "MPTP + vehicle", "DISEASE_CONTROL", true)
	treated := group("G3", "MPTP + Compound X", "TREATMENT", true)
	item := func(groupID, purpose, substanceID, amount, unit, route, frequency string, start, end int) string {
		return text(s.post(base+"/regimen-items", map[string]any{"groupId": groupID, "purpose": purpose, "substanceId": substanceID, "amount": amount,
			"unit": unit, "route": route, "frequency": frequency, "startDay": start, "endDay": end}), "id")
	}
	item(healthy, "VEHICLE", saline, "10", "mL/kg", "ORAL", "DAILY", 0, 14)
	induction := item(disease, "INDUCTION", mptp, "20", "mg/kg", "INTRAPERITONEAL", "ONCE", 0, 0)
	item(disease, "VEHICLE", saline, "10", "mL/kg", "ORAL", "DAILY", 0, 14)
	item(treated, "INDUCTION", mptp, "20", "mg/kg", "INTRAPERITONEAL", "ONCE", 0, 0)
	item(treated, "TREATMENT", compound, "5", "mg/kg", "ORAL", "DAILY", 0, 14)
	timepoint := func(base, name string, day, position int) string {
		return text(s.post(base+"/timepoints", map[string]any{"name": name, "studyDay": day, "position": position}), "id")
	}
	baseline := timepoint(base, "Baseline", -1, 1)
	day7 := timepoint(base, "Day 7", 7, 2)
	day14 := timepoint(base, "Day 14", 14, 3)
	openField := map[string]any{"name": "Open field", "version": map[string]any{"steps": []any{
		map[string]any{"position": 1, "paradigmKey": "OPEN_FIELD", "paradigmVersion": 1, "trialType": "STANDARD", "trials": 2},
	}}}
	versionID := text(nested(s.post(base+"/protocols", openField), "version"), "id")
	s.post(base+"/endpoints", map[string]any{"timepointId": day14, "baselineTimepointId": baseline, "paradigmKey": "OPEN_FIELD",
		"metricKey": "distance_cm", "kind": "PRIMARY", "direction": "HIGHER_IS_BETTER"})

	// Animals: allocated ten days ago, two to three per group.
	allocations := map[string]string{}
	subjects := map[string]string{}
	for i, code := range []string{"M01", "M02", "M03", "M04", "M05", "M06", "M07", "M08"} {
		sex := "MALE"
		if i%2 == 1 {
			sex = "FEMALE"
		}
		subjects[code] = text(s.post("/api/subjects", map[string]any{"code": code, "species": "MOUSE", "sex": sex}), "id")
		groupID := []string{healthy, healthy, disease, disease, disease, treated, treated, treated}[i]
		allocations[code] = text(s.post(base+"/allocations", map[string]any{"subjectId": subjects[code], "groupId": groupID,
			"cage": fmt.Sprintf("C%d", i/3+1), "allocatedAt": at(today.AddDate(0, 0, -10))}), "id")
	}
	s.post(base+"/start", map[string]any{"dayZero": dayZero.Format("2006-01-02")})

	// Weights four days ago for the first five animals; the treated group was
	// never weighed, so its per-kilogram doses ask for a weight first.
	for i, code := range []string{"M01", "M02", "M03", "M04", "M05"} {
		s.post("/api/subjects/"+subjects[code]+"/weights", map[string]any{"grams": fmt.Sprintf("%d.5", 24+i), "measuredAt": at(today.AddDate(0, 0, -4).Add(9 * time.Hour))})
	}
	// Day 0: the disease control animals were induced; M03 is confirmed.
	for _, code := range []string{"M03", "M04", "M05"} {
		s.post(base+"/allocations/"+allocations[code]+"/administrations", map[string]any{"regimenItemId": induction,
			"administeredAt": at(dayZero.Add(10 * time.Hour))})
	}
	s.post(base+"/allocations/"+allocations["M03"]+"/induction-outcome", map[string]any{"status": "CONFIRMED", "assessedAt": at(dayZero.AddDate(0, 0, 2).Add(10 * time.Hour))})

	// film uploads the fixture as the session's video and verifies it.
	film := func(testID string) {
		started := s.post("/api/tests/"+testID+"/recordings", map[string]any{
			"fileName": "session.mp4", "contentType": "video/mp4", "sizeBytes": len(video), "crc32c": mediadomain.FormatCRC32C(crc32c(video)),
		})
		if s.err != nil {
			return
		}
		uploaded, err := url.Parse(text(nested(started, "upload"), "url"))
		if err != nil {
			s.err = err
			return
		}
		store.put(uploaded.Query().Get("object"), video, "video/mp4")
		s.post("/api/tests/"+testID+"/recordings/"+text(nested(started, "recording"), "id")+"/finalize", map[string]any{})
	}
	// running plans a session with animals in arenas (allocation, arena
	// revision pairs), starts it and returns it with its animals by allocation.
	running := func(base, timepointID, version, notes string, seats ...string) (string, map[string]string) {
		var list []any
		for i := 0; i+1 < len(seats); i += 2 {
			list = append(list, map[string]any{"allocationId": seats[i], "environmentRevisionId": seats[i+1]})
		}
		created := s.post(base+"/tests", map[string]any{"timepointId": timepointID, "protocolVersionId": version, "stepPosition": 1,
			"scheduledAt": at(scheduled), "notes": notes, "subjects": list})
		id := text(created, "id")
		s.post("/api/tests/"+id+"/start", map[string]any{"startedAt": at(started)})
		animals := map[string]string{}
		items, _ := created["subjects"].([]any)
		for _, raw := range items {
			if m, ok := raw.(map[string]any); ok {
				animals[text(m, "allocationId")] = text(m, "id")
			}
		}
		return id, animals
	}

	// Sessions: a planned baseline session that is now overdue, one in
	// progress today with two animals in two arenas and a verified video, and
	// a Day 14 session with three animals.
	overdue := s.post(base+"/tests", map[string]any{"timepointId": baseline, "protocolVersionId": versionID, "stepPosition": 1,
		"subjects": []any{map[string]any{"allocationId": allocations["M01"], "environmentRevisionId": boxARevision}}})
	runningID, baselineAnimals := running(base, baseline, versionID, "Morning session",
		allocations["M02"], boxARevision, allocations["M03"], boxBRevision)
	if s.err != nil {
		return nil, s.err
	}
	if len(baselineAnimals) != 2 {
		return nil, errors.New("the running session has no animals")
	}
	m02, m03 := baselineAnimals[allocations["M02"]], baselineAnimals[allocations["M03"]]
	s.post("/api/tests/"+runningID+"/subjects/"+m02+"/trials", map[string]any{"repetition": 1,
		"startedAt": at(trialStart), "endedAt": at(trialEnd)})
	s.post("/api/tests/"+runningID+"/comments", map[string]any{"body": "Box B lamp flickered at the start."})
	film(runningID)
	day14ID, day14Animals := running(base, day14, versionID, "Day 14 session",
		allocations["M04"], boxARevision, allocations["M06"], boxBRevision, allocations["M07"], boxCRevision)
	film(day14ID)
	m04, m06, m07 := day14Animals[allocations["M04"]], day14Animals[allocations["M06"]], day14Animals[allocations["M07"]]

	// The blinded study: one healthy group, one timepoint, one session.
	blind := s.post("/api/experiments", map[string]any{"code": "E2E-BLIND", "title": "Blinded open field",
		"hypothesis": "Handling does not change exploration", "randomisation": "Simple randomisation", "blinded": true})
	blindID := text(blind, "id")
	blindBase := "/api/experiments/" + blindID
	blindGroup := text(s.post(blindBase+"/groups", map[string]any{"code": "G1", "name": "Handled", "role": "HEALTHY_CONTROL", "targetSize": 2}), "id")
	blindDay := timepoint(blindBase, "Baseline", 0, 1)
	blindVersion := text(nested(s.post(blindBase+"/protocols", openField), "version"), "id")
	s.post(blindBase+"/endpoints", map[string]any{"timepointId": blindDay, "paradigmKey": "OPEN_FIELD",
		"metricKey": "distance_cm", "kind": "PRIMARY", "direction": "HIGHER_IS_BETTER"})
	blindAllocations := map[string]string{}
	for _, code := range []string{"B01", "B02"} {
		subject := text(s.post("/api/subjects", map[string]any{"code": code, "species": "MOUSE", "sex": "MALE"}), "id")
		blindAllocations[code] = text(s.post(blindBase+"/allocations", map[string]any{"subjectId": subject, "groupId": blindGroup}), "id")
	}
	s.post(blindBase+"/start", map[string]any{"dayZero": today.Format("2006-01-02")})
	blindTestID, blindAnimals := running(blindBase, blindDay, blindVersion, "Blinded session",
		blindAllocations["B01"], boxARevision, blindAllocations["B02"], boxBRevision)
	film(blindTestID)
	if s.err != nil {
		return nil, s.err
	}

	// Analysis: a worker analyses every queued animal; M02 is analysed again
	// so its review offers a newer run; two Day 14 results are accepted.
	registered := s.post("/api/analysis/workers", map[string]any{"name": "e2e-tracker", "modelVersion": "tracker 1.0",
		"capabilities": []any{map[string]any{"paradigmKey": "OPEN_FIELD", "paradigmVersion": 1}}})
	if s.err != nil {
		return nil, s.err
	}
	worker := &caller{handler: api.Handler, worker: text(registered, "token")}
	runs := 0
	analyse := func() error {
		if err := api.AnalysisTick(context.Background()); err != nil {
			return err
		}
		for {
			job, err := worker.do(http.MethodPost, "/api/worker/claim", map[string]any{})
			if err != nil {
				return err
			}
			if nested(job, "run") == nil {
				return nil
			}
			if err := submitRun(worker, store, job, video, trajectory); err != nil {
				return err
			}
			runs++
		}
	}
	if err := analyse(); err != nil {
		return nil, err
	}
	s.post("/api/analysis/test-subjects/"+m02+"/runs", map[string]any{})
	if s.err != nil {
		return nil, s.err
	}
	if err := analyse(); err != nil {
		return nil, err
	}
	if runs != 8 {
		return nil, fmt.Errorf("the worker analysed %d runs, want 8", runs)
	}
	for _, accepted := range []string{m04, m06} {
		s.post("/api/analysis/results/"+accepted+"/accept", map[string]any{"note": "Clean track"})
	}
	if s.err != nil {
		return nil, s.err
	}
	return map[string]any{
		"experimentId": experimentID, "experimentCode": "E2E", "dayZero": dayZero.Format("2006-01-02"),
		"groups":            map[string]any{"healthy": healthy, "diseaseControl": disease, "treatment": treated},
		"timepoints":        map[string]any{"baseline": baseline, "day7": day7, "day14": day14},
		"protocolVersionId": versionID, "allocations": allocations, "subjects": subjects,
		"environments":  map[string]any{"boxA": boxA, "boxB": boxB, "boxC": boxC},
		"arenas":        map[string]any{"boxA": boxARevision, "boxB": boxBRevision, "boxC": boxCRevision},
		"plannedTestId": text(overdue, "id"), "runningTestId": runningID, "day14TestId": day14ID, "accounts": accounts,
		// Test subjects (an animal in a session) with analysis results.
		"results": map[string]any{"M02": m02, "M03": m03, "M04": m04, "M06": m06, "M07": m07},
		"blind": map[string]any{"experimentId": blindID, "experimentCode": "E2E-BLIND", "testId": blindTestID,
			"results": map[string]any{"B01": blindAnimals[blindAllocations["B01"]], "B02": blindAnimals[blindAllocations["B02"]]}},
	}, nil
}

// submitRun does a worker's part for one claimed run: it uploads the analysed
// video and the fixture trajectory, rewritten for this run, to the store and
// submits the result. The API computes the metrics from the trajectory.
func submitRun(worker *caller, store *diskStore, job map[string]any, video, trajectory []byte) error {
	run := nested(job, "run")
	runID, attempt := text(run, "id"), run["attempt"]
	var track map[string]any
	if err := json.Unmarshal(trajectory, &track); err != nil {
		return err
	}
	track["runId"], track["attempt"] = runID, attempt
	duration, _ := track["durationUs"].(float64)
	rewritten, err := json.Marshal(track)
	if err != nil {
		return err
	}
	path := "/api/worker/runs/" + runID
	outputs := []struct {
		kind, fileName, contentType string
		data                        []byte
	}{
		{"ANALYZED_VIDEO", "overlay.mp4", "video/mp4", video},
		{"TRAJECTORY", "trajectory.json", "application/json", rewritten},
	}
	var artifacts []any
	analyzed := ""
	for _, o := range outputs {
		out, err := worker.do(http.MethodPost, path+"/outputs", map[string]any{"attempt": attempt, "kind": o.kind, "fileName": o.fileName,
			"contentType": o.contentType, "sizeBytes": len(o.data), "crc32c": mediadomain.FormatCRC32C(crc32c(o.data))})
		if err != nil {
			return err
		}
		object := text(out, "objectName")
		store.put(object, o.data, o.contentType)
		artifacts = append(artifacts, map[string]any{"kind": o.kind, "objectName": object})
		if o.kind == "ANALYZED_VIDEO" {
			analyzed = object
		}
	}
	_, err = worker.do(http.MethodPost, path+"/result", map[string]any{"attempt": attempt, "modelVersion": "tracker 1.0",
		"recordingDurationUs": int64(duration), "artifacts": artifacts,
		"pair": map[string]any{"analyzedObjectName": analyzed, "sourceOffsetUs": 0, "outputOffsetUs": 0, "timeMappingVersion": "identity-v1"}})
	return err
}
