package gcs

import (
	"cloud.google.com/go/storage"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"github.com/HappyHackingSpace/Misko/backend/internal/media/application"
	"google.golang.org/api/option"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

// newTestStore signs with a throwaway key and talks to a local server; no
// Google credentials or network are used. This does not prove real GCS
// behavior; see gcs_live_test.go.
func newTestStore(t *testing.T, handler http.HandlerFunc) *Store {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := storage.NewClient(context.Background(), option.WithoutAuthentication(), option.WithEndpoint(server.URL+"/storage/v1/"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return &Store{client: client, bucket: "misko-test-videos", signer: "api@misko-lab.iam.gserviceaccount.com",
		privateKey: pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})}
}

func signedQuery(t *testing.T, raw string) (*url.URL, url.Values) {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u, u.Query()
}

func TestUploadURLRequiresANewObject(t *testing.T) {
	s := newTestStore(t, http.NotFound)
	expires := time.Now().Add(time.Hour)
	req, err := s.UploadURL(context.Background(), "tests/t1/originals/a1", "video/mp4", expires)
	if err != nil {
		t.Fatal(err)
	}
	u, q := signedQuery(t, req.URL)
	if req.Method != "POST" || u.Path != "/misko-test-videos/tests/t1/originals/a1" || q.Get("X-Goog-Algorithm") != "GOOG4-RSA-SHA256" ||
		!strings.HasPrefix(q.Get("X-Goog-Credential"), "api@misko-lab.iam.gserviceaccount.com/") {
		t.Fatalf("upload URL: %s", req.URL)
	}
	if signed := q.Get("X-Goog-SignedHeaders"); signed != "content-type;host;x-goog-if-generation-match;x-goog-resumable" {
		t.Fatalf("signed headers: %s", signed)
	}
	if seconds, _ := strconv.Atoi(q.Get("X-Goog-Expires")); seconds < 3590 || seconds > 3600 {
		t.Fatalf("expiry: %s", q.Get("X-Goog-Expires"))
	}
	if req.Headers["x-goog-if-generation-match"] != "0" || req.Headers["x-goog-resumable"] != "start" || req.Headers["Content-Type"] != "video/mp4" || !req.ExpiresAt.Equal(expires) {
		t.Fatalf("headers the client must send: %v", req.Headers)
	}
}

func TestReadURLPinsTheGeneration(t *testing.T) {
	s := newTestStore(t, http.NotFound)
	raw, err := s.ReadURL(context.Background(), "tests/t1/originals/a1", 1757592000000001, time.Now().Add(15*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	u, q := signedQuery(t, raw)
	if u.Path != "/misko-test-videos/tests/t1/originals/a1" || q.Get("generation") != "1757592000000001" || q.Get("X-Goog-SignedHeaders") != "host" {
		t.Fatalf("read URL: %s", raw)
	}
}

func TestAttrsReadsGenerationSizeAndChecksum(t *testing.T) {
	s := newTestStore(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/o/missing"):
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":404,"message":"No such object"}}`))
		case strings.HasSuffix(r.URL.Path, "/o/tests%2Ft1%2Foriginals%2Fa1"), strings.HasSuffix(r.URL.Path, "/o/tests/t1/originals/a1"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"bucket":"misko-test-videos","name":"tests/t1/originals/a1","generation":"42","size":"1024","crc32c":"AAAAAQ==","contentType":"video/mp4"}`))
		default:
			t.Errorf("unexpected request %s", r.URL.String())
			w.WriteHeader(http.StatusTeapot)
		}
	})
	attrs, err := s.Attrs(context.Background(), "tests/t1/originals/a1")
	if err != nil || attrs.Generation != 42 || attrs.Size != 1024 || attrs.CRC32C != 1 || attrs.ContentType != "video/mp4" {
		t.Fatalf("attrs: %+v %v", attrs, err)
	}
	if _, err := s.Attrs(context.Background(), "missing"); !errors.Is(err, application.ErrObjectNotFound) {
		t.Fatalf("missing object: %v", err)
	}
}
