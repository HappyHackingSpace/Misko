//go:build integration

package bootstrap

import (
	"context"
	mediaapp "github.com/HappyHackingSpace/Misko/backend/internal/media/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/media/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/config"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
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
	lastObject string
}

func (f *fakeObjects) Bucket() string { return "misko-test-videos" }

func (f *fakeObjects) UploadURL(_ context.Context, object, contentType string, expires time.Time) (mediaapp.SignedRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastObject = object
	return mediaapp.SignedRequest{Method: "POST", URL: "https://storage.test/" + object + "?X-Goog-Signature=upload-secret", ExpiresAt: expires,
		Headers: map[string]string{"Content-Type": contentType, "x-goog-resumable": "start", "x-goog-if-generation-match": "0"}}, nil
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
	subject := c.create("/api/subjects", admin, map[string]any{"code": "M-1", "species": "MOUSE", "sex": "MALE"})
	exp := c.create("/api/experiments", admin, map[string]any{"code": "EXP-V", "title": "Video"})
	base := "/api/experiments/" + id(exp)
	enrollment := c.create(base+"/enrollments", admin, map[string]any{"subjectId": id(subject), "enrolledAt": time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339)})
	arena := c.create("/api/environments", admin, map[string]any{"name": "Arena", "paradigmKey": "OPEN_FIELD",
		"revision": map[string]any{"paradigmVersion": 1, "apparatus": map[string]any{"arena_width_cm": 50, "arena_height_cm": 50, "center_fraction": 0.5}}})
	protocol := c.create(base+"/protocols", admin, map[string]any{"name": "Open field", "version": map[string]any{"steps": []any{map[string]any{
		"position": 1, "paradigmKey": "OPEN_FIELD", "paradigmVersion": 1, "environmentRevisionId": id(arena["revision"].(map[string]any)), "trialType": "STANDARD", "trials": 1,
	}}}})
	plan := map[string]any{"enrollmentId": id(enrollment), "protocolVersionId": id(protocol["version"].(map[string]any)), "stepPosition": 1, "scheduledAt": time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)}
	test, other := c.create(base+"/tests", admin, plan), c.create(base+"/tests", admin, plan)
	recordings := "/api/tests/" + id(test) + "/recordings"
	video := map[string]any{"fileName": "trial-1.mp4", "contentType": "video/mp4", "sizeBytes": 4096, "crc32c": domain.FormatCRC32C(0xe2932d1b), "clipStartUs": 0}

	for _, role := range []string{"VIEWER"} {
		c.expect(role+" uploads", c.call("POST", recordings, tokens[role], video), 403, "auth.forbidden")
	}
	started := c.create(recordings, tokens["TECHNICIAN"], video)
	rec, upload := started["recording"].(map[string]any), started["upload"].(map[string]any)
	headers := upload["headers"].(map[string]any)
	if upload["method"] != "POST" || headers["x-goog-if-generation-match"] != "0" || headers["x-goog-resumable"] != "start" ||
		rec["video"].(map[string]any)["status"] != "PENDING" || rec["video"].(map[string]any)["generation"] != nil {
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
	c.expect("recording through another test", c.call("GET", "/api/tests/"+id(other)+"/recordings/"+id(rec)+"/read-url", tokens["VIEWER"], nil), 404, "recording.notFound")

	corrupt := c.create(recordings, tokens["TECHNICIAN"], video)["recording"].(map[string]any)
	objects.finishUpload(4096, 7)
	rejected := c.expect("finalize corrupt upload", c.call("POST", recordings+"/"+id(corrupt)+"/finalize", tokens["TECHNICIAN"], map[string]any{}), 200, "").body
	if v := rejected["video"].(map[string]any); v["status"] != "REJECTED" || v["rejectionReason"] != "CHECKSUM_MISMATCH" {
		t.Fatalf("corrupt upload: %v", rejected)
	}
	c.expect("read a rejected video", c.call("GET", recordings+"/"+id(corrupt)+"/read-url", tokens["VIEWER"], nil), 409, "media.notVerified")
	oversized := map[string]any{"fileName": "big.mp4", "contentType": "video/mp4", "sizeBytes": int64(2 << 30), "crc32c": "AAAAAQ=="}
	c.expect("oversized upload", c.call("POST", recordings, tokens["TECHNICIAN"], oversized), 400, "media.invalidSize")
	image := map[string]any{"fileName": "x.png", "contentType": "image/png", "sizeBytes": 10, "crc32c": "AAAAAQ=="}
	c.expect("not a video", c.call("POST", recordings, tokens["TECHNICIAN"], image), 400, "media.invalidContentType")
	if list := c.call("GET", recordings, tokens["VIEWER"], nil).body["data"].([]any); len(list) != 2 {
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
