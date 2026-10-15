//go:build integration

package bootstrap

import (
	"context"
	"errors"
	mediaapp "github.com/HappyHackingSpace/Misko/backend/internal/media/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/media/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/config"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
	"log/slog"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeObjects is an in-memory object store. It proves the API contract only;
// real GCS behavior is covered by the separate live smoke test.
type fakeObjects struct {
	mu         sync.Mutex
	stored     map[string]domain.ObjectAttrs
	contents   map[string][]byte
	lastObject string
}

func (f *fakeObjects) Read(_ context.Context, object string, generation, limit int64) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	attrs, ok := f.stored[object]
	if !ok || attrs.Generation != generation {
		return nil, mediaapp.ErrObjectNotFound
	}
	if int64(len(f.contents[object])) > limit {
		return nil, errors.New("object too large")
	}
	return f.contents[object], nil
}

// finishUploadWith stores the bytes of the last signed upload with their real size.
func (f *fakeObjects) finishUploadWith(data []byte, crc uint32, contentType string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.contents == nil {
		f.contents = map[string][]byte{}
	}
	f.contents[f.lastObject] = data
	f.stored[f.lastObject] = domain.ObjectAttrs{Generation: 1757592000000000 + int64(len(f.stored)) + 1, Size: int64(len(data)), CRC32C: crc, ContentType: contentType}
}

func (f *fakeObjects) Bucket() string { return "misko-test-videos" }

func (f *fakeObjects) UploadURL(_ context.Context, object, contentType string, expires time.Time) (mediaapp.SignedRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastObject = object
	return mediaapp.SignedRequest{Method: "POST", URL: "https://storage.test/" + object + "?X-Goog-Signature=upload-secret", ExpiresAt: expires,
		Headers: map[string]string{"Content-Type": contentType, "x-goog-resumable": "start", "x-goog-if-generation-match": "0"}}, nil
}

// SizedUploadURL signs like Cloud Storage does with x-goog-content-length-range.
func (f *fakeObjects) SizedUploadURL(ctx context.Context, object, contentType string, size int64, expires time.Time) (mediaapp.SignedRequest, error) {
	signed, err := f.UploadURL(ctx, object, contentType, expires)
	signed.Headers["x-goog-content-length-range"] = "0," + strconv.FormatInt(size, 10)
	return signed, err
}

func (f *fakeObjects) ReadURL(_ context.Context, object string, generation int64, expires time.Time) (string, error) {
	return "https://storage.test/" + object + "?generation=" + strconv.FormatInt(generation, 10) + "&X-Goog-Signature=read-secret", nil
}

func (f *fakeObjects) Attrs(_ context.Context, object string) (domain.ObjectAttrs, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if attrs, ok := f.stored[object]; ok {
		return attrs, nil
	}
	return domain.ObjectAttrs{}, mediaapp.ErrObjectNotFound
}

func (f *fakeObjects) finishUpload(size int64, crc uint32) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stored[f.lastObject] = domain.ObjectAttrs{Generation: 1757592000000000 + int64(len(f.stored)) + 1, Size: size, CRC32C: crc, ContentType: "video/mp4"}
}

// mediaStudy is a running experiment seeded with SQL, because the contexts
// that own experiments, allocations and tests are ported separately: a test in
// progress filming one animal per arena, and a planned test.
type mediaStudy struct {
	experiment, test, planned string
	subjects, arenas          []string
	environments              []string
}

const seedUser = "01a00000-0000-7000-8000-0000000000aa"

func seedMediaStudy(t *testing.T, pool *pgxpool.Pool, code string, arenas int) mediaStudy {
	t.Helper()
	ctx := context.Background()
	one := func(sql string, args ...any) string {
		t.Helper()
		var out string
		if err := pool.QueryRow(ctx, sql, args...).Scan(&out); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return out
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	var st mediaStudy
	st.experiment = one("INSERT INTO misko.experiments (code, title) VALUES ($1, 'Video study') RETURNING id", code)
	group := one("INSERT INTO misko.experiment_groups (experiment_id, code, name, role) VALUES ($1, 'G1', 'Healthy', 'HEALTHY_CONTROL') RETURNING id", st.experiment)
	timepoint := one("INSERT INTO misko.timepoints (experiment_id, name, study_day, position) VALUES ($1, 'Day 1', 1, 1) RETURNING id", st.experiment)
	protocol := one("INSERT INTO misko.protocols (experiment_id, name) VALUES ($1, 'Battery') RETURNING id", st.experiment)
	var version string
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, "INSERT INTO misko.protocol_versions (experiment_id, protocol_id, number, step_count, created_by) VALUES ($1, $2, 1, 2, $3) RETURNING id",
			st.experiment, protocol, seedUser).Scan(&version); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO misko.protocol_steps (protocol_version_id, position, paradigm_key, paradigm_version, trial_type, trials, session)
			VALUES ($1, 1, 'OPEN_FIELD', 1, 'STANDARD', 1, '{}'), ($1, 2, 'ROTAROD', 1, 'ACCELERATING', 1, '{}')`, version)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	exec("UPDATE misko.experiments SET status = 'RUNNING', day_zero = current_date, started_at = now() WHERE id = $1", st.experiment)
	plan := `INSERT INTO misko.tests (experiment_id, timepoint_id, protocol_version_id, step_position, paradigm_key, paradigm_version, planned_trials, scheduled_at, created_by)
		VALUES ($1, $2, $3, 1, 'OPEN_FIELD', 1, 1, now(), $4) RETURNING id`
	st.test, st.planned = one(plan, st.experiment, timepoint, version, seedUser), one(plan, st.experiment, timepoint, version, seedUser)
	for i := range arenas {
		name := code + "-" + strconv.Itoa(i+1)
		subject := one("INSERT INTO misko.subjects (code, species, sex) VALUES ($1, 'MOUSE', 'MALE') RETURNING id", name)
		allocation := one(`INSERT INTO misko.allocations (experiment_id, group_id, subject_id, allocated_at, created_by)
			VALUES ($1, $2, $3, now() - interval '1 day', $4) RETURNING id`, st.experiment, group, subject, seedUser)
		env := one("INSERT INTO misko.environments (name, paradigm_key) VALUES ($1, 'OPEN_FIELD') RETURNING id", "Arena "+name)
		arena := one(`INSERT INTO misko.environment_revisions (environment_id, paradigm_key, paradigm_version, number, parameters, created_by)
			VALUES ($1, 'OPEN_FIELD', 1, 1, '{"arena_width_cm": 50, "arena_height_cm": 40, "center_fraction": 0.5}', $2) RETURNING id`, env, seedUser)
		st.subjects = append(st.subjects, one(`INSERT INTO misko.test_subjects (experiment_id, test_id, allocation_id, environment_revision_id, paradigm_key, paradigm_version, created_by)
			VALUES ($1, $2, $3, $4, 'OPEN_FIELD', 1, $5) RETURNING id`, st.experiment, st.test, allocation, arena, seedUser))
		st.arenas, st.environments = append(st.arenas, arena), append(st.environments, env)
	}
	exec("UPDATE misko.tests SET status = 'IN_PROGRESS', started_at = now() WHERE id = $1", st.test)
	return st
}

func TestVideoUploadsOverHTTP(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.Database(t)
	setup, err := Setup(ctx, pool, config.Setup{AdminEmail: "admin@lab.io", LabName: "Lab", LabTimezone: "UTC", BcryptCost: bcrypt.MinCost})
	if err != nil {
		t.Fatal(err)
	}
	auth := config.Auth{JWTSecret: []byte(strings.Repeat("s", 32)), JWTIssuer: "misko", JWTAudience: "misko-api", TokenTTL: time.Hour, BcryptCost: bcrypt.MinCost}
	logs := &lockedBuffer{}
	objects := &fakeObjects{stored: map[string]domain.ObjectAttrs{}}
	api, err := NewAPI(pool, config.Config{ProbeTimeout: time.Second}, auth, slog.New(slog.NewJSONHandler(logs, nil)),
		WithStorage(objects, mediaapp.Settings{MaxBytes: 1 << 30, UploadTTL: time.Hour, ReadTTL: 15 * time.Minute}))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.Handler)
	defer server.Close()
	c := client{t: t, url: server.URL, http: server.Client()}

	admin := c.login("admin@lab.io", setup.AdminPassword)
	tokens := map[string]string{"SUPERADMIN": admin}
	for _, role := range []string{"LAB_MANAGER", "RESEARCHER", "TECHNICIAN", "VIEWER"} {
		email := strings.ToLower(role) + "@lab.io"
		c.create("/api/users", admin, map[string]any{"email": email, "name": role, "role": role, "password": "Password-123"})
		tokens[role] = c.login(email, "Password-123")
	}
	study, other := seedMediaStudy(t, pool, "EXP-V", 1), seedMediaStudy(t, pool, "EXP-W", 1)
	recordings := "/api/tests/" + study.test + "/recordings"
	video := map[string]any{"fileName": "trial-1.mp4", "contentType": "video/mp4", "sizeBytes": 4096, "crc32c": domain.FormatCRC32C(0xe2932d1b), "clipStartUs": 0}

	c.expect("current before any video", c.call("GET", recordings+"/current", tokens["VIEWER"], nil), 404, "recording.notFound")
	c.expect("viewer uploads", c.call("POST", recordings, tokens["VIEWER"], video), 403, "auth.forbidden")
	c.expect("upload to a planned test", c.call("POST", "/api/tests/"+study.planned+"/recordings", tokens["TECHNICIAN"], video), 409, "media.testNotStarted")
	c.expect("upload to an unknown test", c.call("POST", "/api/tests/01a00000-0000-7000-8000-00000000dead/recordings", tokens["TECHNICIAN"], video), 404, "test.notFound")
	started := c.create(recordings, tokens["TECHNICIAN"], video)
	rec, upload := started["recording"].(map[string]any), started["upload"].(map[string]any)
	headers := upload["headers"].(map[string]any)
	if upload["method"] != "POST" || headers["x-goog-if-generation-match"] != "0" || headers["x-goog-resumable"] != "start" || headers["x-goog-content-length-range"] != "0,4096" ||
		rec["video"].(map[string]any)["status"] != "PENDING" || rec["video"].(map[string]any)["generation"] != nil || rec["current"] != false || rec["supersedesId"] != nil {
		t.Fatalf("upload start: %v", started)
	}
	recPath := recordings + "/" + id(rec)
	c.expect("viewer finalizes", c.call("POST", recPath+"/finalize", tokens["VIEWER"], map[string]any{}), 403, "auth.forbidden")
	c.expect("researcher finalizes another user's upload", c.call("POST", recPath+"/finalize", tokens["RESEARCHER"], map[string]any{}), 403, "media.notUploader")
	c.expect("finalize before the upload finished", c.call("POST", recPath+"/finalize", tokens["TECHNICIAN"], map[string]any{}), 409, "media.uploadIncomplete")
	c.expect("read before verification", c.call("GET", recPath+"/read-url", tokens["VIEWER"], nil), 409, "media.notVerified")

	objects.finishUpload(4096, 0xe2932d1b)
	verified := c.expect("technician finalizes", c.call("POST", recPath+"/finalize", tokens["TECHNICIAN"], map[string]any{}), 200, "").body
	generation := verified["video"].(map[string]any)["generation"]
	if verified["video"].(map[string]any)["status"] != "VERIFIED" || generation == nil {
		t.Fatalf("verified: %v", verified)
	}
	again := c.expect("finalize again", c.call("POST", recPath+"/finalize", tokens["LAB_MANAGER"], map[string]any{}), 200, "").body
	if again["video"].(map[string]any)["generation"] != generation {
		t.Fatalf("repeated finalization changed the video: %v", again)
	}
	for role, token := range tokens {
		link := c.expect(role+" read URL", c.call("GET", recPath+"/read-url", token, nil), 200, "").body
		if !strings.Contains(link["url"].(string), "generation="+generation.(string)) || link["expiresAt"] == nil {
			t.Fatalf("%s read URL: %v", role, link)
		}
		c.expect(role+" lists recordings", c.call("GET", recordings, token, nil), 200, "")
	}
	c.expect("anonymous read URL", c.call("GET", recPath+"/read-url", "", nil), 401, "auth.unauthenticated")
	c.expect("recording through another test", c.call("GET", "/api/tests/"+other.test+"/recordings/"+id(rec)+"/read-url", tokens["VIEWER"], nil), 404, "recording.notFound")

	// A replacement supersedes the newest video but becomes current only once
	// it is verified; a rejected replacement leaves the verified video current.
	corrupt := c.create(recordings, tokens["TECHNICIAN"], video)["recording"].(map[string]any)
	if corrupt["supersedesId"] != id(rec) || corrupt["current"] != false {
		t.Fatalf("replacement: %v", corrupt)
	}
	objects.finishUpload(4096, 7)
	rejected := c.expect("finalize corrupt upload", c.call("POST", recordings+"/"+id(corrupt)+"/finalize", tokens["TECHNICIAN"], map[string]any{}), 200, "").body
	if v := rejected["video"].(map[string]any); v["status"] != "REJECTED" || v["rejectionReason"] != "CHECKSUM_MISMATCH" {
		t.Fatalf("corrupt upload: %v", rejected)
	}
	c.expect("read a rejected video", c.call("GET", recordings+"/"+id(corrupt)+"/read-url", tokens["VIEWER"], nil), 409, "media.notVerified")
	c.expect("read the superseded video", c.call("GET", recPath+"/read-url", tokens["VIEWER"], nil), 200, "")
	if current := c.expect("current", c.call("GET", recordings+"/current", tokens["VIEWER"], nil), 200, "").body; current["id"] != id(rec) {
		t.Fatalf("current: %v", current)
	}
	oversized := map[string]any{"fileName": "big.mp4", "contentType": "video/mp4", "sizeBytes": int64(2 << 30), "crc32c": "AAAAAQ=="}
	c.expect("oversized upload", c.call("POST", recordings, tokens["TECHNICIAN"], oversized), 400, "media.invalidSize")
	image := map[string]any{"fileName": "x.png", "contentType": "image/png", "sizeBytes": 10, "crc32c": "AAAAAQ=="}
	c.expect("not a video", c.call("POST", recordings, tokens["TECHNICIAN"], image), 400, "media.invalidContentType")
	list := c.call("GET", recordings, tokens["VIEWER"], nil).body["data"].([]any)
	if len(list) != 2 || list[0].(map[string]any)["current"] != true || list[1].(map[string]any)["current"] != false {
		t.Fatalf("recordings: %v", list)
	}
	if strings.Contains(logs.String(), "X-Goog-Signature") || strings.Contains(logs.String(), "secret") {
		t.Fatal("logs contain a signed URL")
	}

	// Without a bucket the video routes say so instead of failing later.
	unconfigured, err := NewAPI(pool, config.Config{ProbeTimeout: time.Second}, auth, slog.New(slog.NewJSONHandler(logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	bare := httptest.NewServer(unconfigured.Handler)
	defer bare.Close()
	nc := client{t: t, url: bare.URL, http: bare.Client()}
	nc.expect("upload without storage", nc.call("POST", recordings, tokens["TECHNICIAN"], video), 503, "media.storageNotConfigured")
}
