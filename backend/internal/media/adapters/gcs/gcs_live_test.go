//go:build gcslive

package gcs

// The live smoke test uses a real bucket and Application Default Credentials.
// It writes one object under smoke/ and never deletes anything. Run it with:
//
//	GCS_LIVE_BUCKET=<bucket> GCS_LIVE_ORIGIN=<allowed UI origin> [GCS_SIGNER_EMAIL=<service account>] \
//	  go test -tags=gcslive -run TestLive ./internal/media/adapters/gcs/

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"hash/crc32"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestLiveResumableUploadRangeExpiryAndCORS(t *testing.T) {
	bucket, origin := os.Getenv("GCS_LIVE_BUCKET"), os.Getenv("GCS_LIVE_ORIGIN")
	if bucket == "" || origin == "" {
		t.Fatal("GCS_LIVE_BUCKET and GCS_LIVE_ORIGIN are required for the live smoke test")
	}
	ctx := context.Background()
	s, err := New(ctx, bucket, os.Getenv("GCS_SIGNER_EMAIL"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	body := make([]byte, 3<<20)
	if _, err := rand.Read(body); err != nil {
		t.Fatal(err)
	}
	object := "smoke/" + time.Now().UTC().Format("20060102T150405.000000000")

	start, err := s.UploadURL(ctx, object, "video/mp4", time.Now().Add(10*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	session := startSession(t, start.URL, start.Headers, origin)
	put, _ := http.NewRequest(http.MethodPut, session, bytes.NewReader(body))
	put.Header.Set("Content-Type", "video/mp4")
	if res := do(t, put); res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		t.Fatalf("upload: %d", res.StatusCode)
	}

	attrs, err := s.Attrs(ctx, object)
	sum := crc32.Checksum(body, crc32.MakeTable(crc32.Castagnoli))
	if err != nil || attrs.Size != int64(len(body)) || attrs.CRC32C != sum || attrs.Generation <= 0 {
		t.Fatalf("attrs: %+v %v want crc %d", attrs, err, sum)
	}

	// A second upload to the same object fails its precondition.
	again, err := s.UploadURL(ctx, object, "video/mp4", time.Now().Add(10*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if res := startRequest(t, again.URL, again.Headers, origin); res.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("overwrite start: %d, want 412", res.StatusCode)
	}

	read, err := s.ReadURL(ctx, object, attrs.Generation, time.Now().Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	get, _ := http.NewRequest(http.MethodGet, read, nil)
	get.Header.Set("Range", "bytes=100-199")
	get.Header.Set("Origin", origin)
	res := do(t, get)
	got, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusPartialContent || !bytes.Equal(got, body[100:200]) || res.Header.Get("Access-Control-Allow-Origin") == "" {
		t.Fatalf("range read: %d %d bytes, CORS %q", res.StatusCode, len(got), res.Header.Get("Access-Control-Allow-Origin"))
	}

	preflight, _ := http.NewRequest(http.MethodOptions, read, nil)
	preflight.Header.Set("Origin", origin)
	preflight.Header.Set("Access-Control-Request-Method", "GET")
	preflight.Header.Set("Access-Control-Request-Headers", "range")
	if res := do(t, preflight); res.Header.Get("Access-Control-Allow-Origin") == "" {
		t.Fatalf("CORS preflight: %d %v", res.StatusCode, res.Header)
	}

	short, err := s.ReadURL(ctx, object, attrs.Generation, time.Now().Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(4 * time.Second)
	expired, _ := http.NewRequest(http.MethodGet, short, nil)
	if res := do(t, expired); res.StatusCode != http.StatusBadRequest && res.StatusCode != http.StatusForbidden {
		t.Fatalf("expired URL: %d", res.StatusCode)
	}
	var checksum [4]byte
	binary.BigEndian.PutUint32(checksum[:], sum)
	t.Logf("uploaded %s generation %d", object, attrs.Generation)
}

func startSession(t *testing.T, signed string, headers map[string]string, origin string) string {
	t.Helper()
	res := startRequest(t, signed, headers, origin)
	if res.StatusCode != http.StatusCreated || res.Header.Get("Location") == "" {
		t.Fatalf("start resumable upload: %d", res.StatusCode)
	}
	return res.Header.Get("Location")
}

func startRequest(t *testing.T, signed string, headers map[string]string, origin string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, signed, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Origin", origin)
	return do(t, req)
}

func do(t *testing.T, req *http.Request) *http.Response {
	t.Helper()
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}
