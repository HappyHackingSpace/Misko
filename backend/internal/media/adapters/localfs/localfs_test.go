package localfs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/HappyHackingSpace/Misko/backend/internal/media/application"
)

var secret = []byte("local-storage-test-secret-of-at-least-32-bytes")

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newStore(t *testing.T) (*Store, *clock, *httptest.Server) {
	t.Helper()
	c := &clock{t: time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)}
	s, err := New(t.TempDir(), secret, c.now)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(s.Handler())
	t.Cleanup(server.Close)
	return s, c, server
}

func send(t *testing.T, method, target string, headers map[string]string, body []byte) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, target, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultTransport.RoundTrip(req) // never follow a 308
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// startSession sends the signed POST and returns the absolute session URL.
func startSession(t *testing.T, server *httptest.Server, signed application.SignedRequest) string {
	t.Helper()
	resp := send(t, signed.Method, server.URL+signed.URL, signed.Headers, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("start: HTTP %d", resp.StatusCode)
	}
	location := resp.Header.Get("Location")
	if !strings.HasPrefix(location, Prefix+"/session/") {
		t.Fatalf("Location %q", location)
	}
	return server.URL + location
}

func payload(n int) []byte {
	data := make([]byte, n)
	for i := range data {
		data[i] = byte(i*7 + i/251)
	}
	return data
}

func TestFullUploadIsCommittedWithAttributes(t *testing.T) {
	s, _, server := newStore(t)
	ctx := context.Background()
	data := payload(1000)
	signed, err := s.SizedUploadURL(ctx, "tests/t1/originals/a1", "video/mp4", int64(len(data)), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(signed.URL, Prefix+"/upload?") || signed.Headers["x-goog-content-length-range"] != "0,1000" {
		t.Fatalf("signed request %+v", signed)
	}
	session := startSession(t, server, signed)
	resp := send(t, http.MethodPut, session, map[string]string{"Content-Range": "bytes 0-999/1000"}, data)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put: HTTP %d", resp.StatusCode)
	}
	attrs, err := s.Attrs(ctx, "tests/t1/originals/a1")
	if err != nil {
		t.Fatal(err)
	}
	if attrs.Size != 1000 || attrs.CRC32C != crc32.Checksum(data, crc32.MakeTable(crc32.Castagnoli)) || attrs.ContentType != "video/mp4" || attrs.Generation <= 0 {
		t.Fatalf("attrs %+v", attrs)
	}
	got, err := s.Read(ctx, "tests/t1/originals/a1", attrs.Generation, 1000)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("read back: %v", err)
	}
	if _, err := s.Read(ctx, "tests/t1/originals/a1", attrs.Generation, 999); err == nil {
		t.Fatal("a read over the limit succeeded")
	}
	if _, err := s.Read(ctx, "tests/t1/originals/a1", attrs.Generation+1, 1000); !errors.Is(err, application.ErrObjectNotFound) {
		t.Fatalf("another generation: %v", err)
	}
	if _, err := s.Attrs(ctx, "missing"); !errors.Is(err, application.ErrObjectNotFound) {
		t.Fatalf("missing object: %v", err)
	}
	// A repeated PUT on a finished session is answered, not stored again.
	if resp := send(t, http.MethodPut, session, map[string]string{"Content-Range": "bytes */1000"}, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("finished session: HTTP %d", resp.StatusCode)
	}
}

func TestChunkedUploadResumesAfterA308(t *testing.T) {
	s, _, server := newStore(t)
	ctx := context.Background()
	data := payload(2*chunkQuantum + 1234)
	signed, _ := s.UploadURL(ctx, "runs/r1/attempts/1/trajectory.json", "application/json", time.Now().Add(time.Hour))
	session := startSession(t, server, signed)
	total := len(data)

	// The first chunk is not a multiple of 256 KiB: only whole blocks are kept.
	first := chunkQuantum + 100
	resp := send(t, http.MethodPut, session, map[string]string{"Content-Range": fmt.Sprintf("bytes 0-%d/%d", first-1, total)}, data[:first])
	if resp.StatusCode != http.StatusPermanentRedirect || resp.Header.Get("Range") != fmt.Sprintf("bytes=0-%d", chunkQuantum-1) {
		t.Fatalf("first chunk: HTTP %d Range %q", resp.StatusCode, resp.Header.Get("Range"))
	}
	// A status query reports the same offset.
	resp = send(t, http.MethodPut, session, map[string]string{"Content-Range": fmt.Sprintf("bytes */%d", total)}, nil)
	if resp.StatusCode != http.StatusPermanentRedirect || resp.Header.Get("Range") != fmt.Sprintf("bytes=0-%d", chunkQuantum-1) {
		t.Fatalf("status: HTTP %d Range %q", resp.StatusCode, resp.Header.Get("Range"))
	}
	// A chunk that overlaps kept bytes is accepted from where storage stands.
	resp = send(t, http.MethodPut, session, map[string]string{"Content-Range": fmt.Sprintf("bytes %d-%d/%d", chunkQuantum-10, 2*chunkQuantum-1, total)}, data[chunkQuantum-10:2*chunkQuantum])
	if resp.StatusCode != http.StatusPermanentRedirect || resp.Header.Get("Range") != fmt.Sprintf("bytes=0-%d", 2*chunkQuantum-1) {
		t.Fatalf("second chunk: HTTP %d Range %q", resp.StatusCode, resp.Header.Get("Range"))
	}
	resp = send(t, http.MethodPut, session, map[string]string{"Content-Range": fmt.Sprintf("bytes %d-%d/%d", 2*chunkQuantum, total-1, total)}, data[2*chunkQuantum:])
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("last chunk: HTTP %d", resp.StatusCode)
	}
	attrs, _ := s.Attrs(ctx, "runs/r1/attempts/1/trajectory.json")
	got, err := s.Read(ctx, "runs/r1/attempts/1/trajectory.json", attrs.Generation, int64(total))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("read back: %v", err)
	}
}

func TestA308WithoutRangeRestartsAtZero(t *testing.T) {
	s, _, server := newStore(t)
	data := payload(chunkQuantum + 10)
	signed, _ := s.UploadURL(context.Background(), "o", "video/mp4", time.Now().Add(time.Hour))
	session := startSession(t, server, signed)
	// Nothing is kept yet, and a short first chunk keeps no whole block.
	resp := send(t, http.MethodPut, session, map[string]string{"Content-Range": fmt.Sprintf("bytes */%d", len(data))}, nil)
	if resp.StatusCode != http.StatusPermanentRedirect || resp.Header.Get("Range") != "" {
		t.Fatalf("status: HTTP %d Range %q", resp.StatusCode, resp.Header.Get("Range"))
	}
	resp = send(t, http.MethodPut, session, map[string]string{"Content-Range": fmt.Sprintf("bytes 0-999/%d", len(data))}, data[:1000])
	if resp.StatusCode != http.StatusPermanentRedirect || resp.Header.Get("Range") != "" {
		t.Fatalf("short chunk: HTTP %d Range %q", resp.StatusCode, resp.Header.Get("Range"))
	}
	// A chunk after a gap is not stored either.
	resp = send(t, http.MethodPut, session, map[string]string{"Content-Range": fmt.Sprintf("bytes 1000-%d/%d", len(data)-1, len(data))}, data[1000:])
	if resp.StatusCode != http.StatusPermanentRedirect || resp.Header.Get("Range") != "" {
		t.Fatalf("gap: HTTP %d Range %q", resp.StatusCode, resp.Header.Get("Range"))
	}
	// The client restarts at 0 and sends everything.
	resp = send(t, http.MethodPut, session, map[string]string{"Content-Range": fmt.Sprintf("bytes 0-%d/%d", len(data)-1, len(data))}, data)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("restart: HTTP %d", resp.StatusCode)
	}
}

func TestSizeLimitAndExistingObjectsAreRefused(t *testing.T) {
	s, _, server := newStore(t)
	ctx := context.Background()
	signed, _ := s.SizedUploadURL(ctx, "o", "video/mp4", 100, time.Now().Add(time.Hour))
	session := startSession(t, server, signed)
	if resp := send(t, http.MethodPut, session, map[string]string{"Content-Range": "bytes 0-100/101"}, payload(101)); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("declared total over the limit: HTTP %d", resp.StatusCode)
	}
	if resp := send(t, http.MethodPut, session, nil, payload(101)); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("whole body over the limit: HTTP %d", resp.StatusCode)
	}
	if resp := send(t, http.MethodPut, session, map[string]string{"Content-Range": "bytes 0-9/*"}, payload(11)); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("body longer than its range: HTTP %d", resp.StatusCode)
	}
	if resp := send(t, http.MethodPut, session, map[string]string{"Content-Range": "bytes 0-99/100"}, payload(100)); resp.StatusCode != http.StatusOK {
		t.Fatalf("upload at the limit: HTTP %d", resp.StatusCode)
	}
	// The limit header must be sent as signed.
	headers := map[string]string{"Content-Type": "video/mp4", "x-goog-resumable": "start", "x-goog-if-generation-match": "0", "x-goog-content-length-range": "0,999999"}
	if resp := send(t, http.MethodPost, server.URL+signed.URL, headers, nil); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("changed limit header: HTTP %d", resp.StatusCode)
	}
	// if-generation-match: 0, so a finished object is never overwritten.
	again, _ := s.SizedUploadURL(ctx, "o", "video/mp4", 100, time.Now().Add(time.Hour))
	if resp := send(t, http.MethodPost, server.URL+again.URL, again.Headers, nil); resp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("existing object: HTTP %d", resp.StatusCode)
	}
	// Two sessions racing for one name: the second commit fails.
	a, _ := s.UploadURL(ctx, "race", "video/mp4", time.Now().Add(time.Hour))
	first, second := startSession(t, server, a), startSession(t, server, a)
	send(t, http.MethodPut, first, nil, payload(5))
	if resp := send(t, http.MethodPut, second, nil, payload(5)); resp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("second commit: HTTP %d", resp.StatusCode)
	}
}

func TestExpiredOrTamperedSignaturesAreRefused(t *testing.T) {
	s, c, server := newStore(t)
	ctx := context.Background()
	signed, _ := s.UploadURL(ctx, "o", "video/mp4", c.t.Add(time.Minute))

	tampered := strings.Replace(signed.URL, "object=o", "object=p", 1)
	if resp := send(t, http.MethodPost, server.URL+tampered, signed.Headers, nil); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("tampered object: HTTP %d", resp.StatusCode)
	}
	u, _ := url.Parse(signed.URL)
	q := u.Query()
	q.Set("expires", fmt.Sprint(c.t.Add(time.Hour).Unix()))
	if resp := send(t, http.MethodPost, server.URL+u.Path+"?"+q.Encode(), signed.Headers, nil); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("extended expiry: HTTP %d", resp.StatusCode)
	}
	other, _ := New(t.TempDir(), []byte("another-secret-that-is-also-32-bytes-long"), c.now)
	foreign, _ := other.UploadURL(ctx, "o", "video/mp4", c.t.Add(time.Minute))
	if resp := send(t, http.MethodPost, server.URL+foreign.URL, foreign.Headers, nil); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign key: HTTP %d", resp.StatusCode)
	}
	wrongType := map[string]string{"Content-Type": "video/webm", "x-goog-resumable": "start", "x-goog-if-generation-match": "0"}
	if resp := send(t, http.MethodPost, server.URL+signed.URL, wrongType, nil); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("unsigned content type: HTTP %d", resp.StatusCode)
	}
	c.t = c.t.Add(2 * time.Minute)
	if resp := send(t, http.MethodPost, server.URL+signed.URL, signed.Headers, nil); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expired upload: HTTP %d", resp.StatusCode)
	}
	read, _ := s.ReadURL(ctx, "o", 1, c.t.Add(-time.Second))
	if resp := send(t, http.MethodGet, server.URL+read, nil, nil); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expired read: HTTP %d", resp.StatusCode)
	}
	if resp := send(t, http.MethodPut, server.URL+Prefix+"/session/"+strings.Repeat("A", 43), nil, []byte("x")); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("guessed session: HTTP %d", resp.StatusCode)
	}
}

func TestReadsArePinnedToAGenerationAndServeRanges(t *testing.T) {
	s, c, server := newStore(t)
	ctx := context.Background()
	data := payload(5000)
	signed, _ := s.UploadURL(ctx, "v", "video/mp4", c.t.Add(time.Hour))
	send(t, http.MethodPut, startSession(t, server, signed), nil, data)
	attrs, err := s.Attrs(ctx, "v")
	if err != nil {
		t.Fatal(err)
	}
	read, _ := s.ReadURL(ctx, "v", attrs.Generation, c.t.Add(time.Hour))
	resp := send(t, http.MethodGet, server.URL+read, map[string]string{"Range": "bytes=100-199"}, nil)
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusPartialContent || !bytes.Equal(body, data[100:200]) || resp.Header.Get("Content-Range") != "bytes 100-199/5000" || resp.Header.Get("Content-Type") != "video/mp4" {
		t.Fatalf("range read: HTTP %d %q", resp.StatusCode, resp.Header.Get("Content-Range"))
	}
	resp = send(t, http.MethodGet, server.URL+read, nil, nil)
	body, _ = io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !bytes.Equal(body, data) {
		t.Fatalf("full read: HTTP %d", resp.StatusCode)
	}
	stale, _ := s.ReadURL(ctx, "v", attrs.Generation-1, c.t.Add(time.Hour))
	if resp := send(t, http.MethodGet, server.URL+stale, nil, nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("other generation: HTTP %d", resp.StatusCode)
	}
}

func TestObjectsAndGenerationsSurviveARestart(t *testing.T) {
	dir := t.TempDir()
	c := &clock{t: time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)}
	s, _ := New(dir, secret, c.now)
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	ctx := context.Background()
	signed, _ := s.UploadURL(ctx, "a", "video/mp4", c.t.Add(time.Hour))
	send(t, http.MethodPut, startSession(t, server, signed), nil, payload(10))
	first, _ := s.Attrs(ctx, "a")

	c.t = c.t.Add(-time.Hour) // the clock steps back across the restart
	reopened, err := New(dir, secret, c.now)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := reopened.Attrs(ctx, "a"); err != nil || got != first {
		t.Fatalf("after restart: %+v %v", got, err)
	}
	server2 := httptest.NewServer(reopened.Handler())
	defer server2.Close()
	signed, _ = reopened.UploadURL(ctx, "b", "video/mp4", c.t.Add(time.Hour))
	send(t, http.MethodPut, startSession(t, server2, signed), nil, payload(10))
	second, _ := reopened.Attrs(ctx, "b")
	if second.Generation <= first.Generation {
		t.Fatalf("generation went back: %d after %d", second.Generation, first.Generation)
	}
}

func TestParseContentRange(t *testing.T) {
	for raw, want := range map[string]contentRange{
		"bytes 0-9/10": {0, 9, 10}, "bytes */10": {-1, -1, 10}, "bytes 5-9/*": {5, 9, -1},
	} {
		if got, ok := parseContentRange(raw); !ok || got != want {
			t.Errorf("%q: %+v %v", raw, got, ok)
		}
	}
	for _, raw := range []string{"", "bytes 0-10/10", "bytes 9-0/10", "items 0-1/2", "bytes 0-1", "bytes a-b/c"} {
		if _, ok := parseContentRange(raw); ok {
			t.Errorf("%q accepted", raw)
		}
	}
}
