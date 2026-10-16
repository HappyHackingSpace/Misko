package localfs

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// chunkQuantum is the granularity of a resumable upload: a chunk that is not
// the last one is kept only up to a multiple of 256 KiB, as on a bucket.
const chunkQuantum = 256 << 10

// transferTimeout replaces the API server's short read and write deadlines
// on storage routes, so large chunks and whole video downloads can finish.
const transferTimeout = 6 * time.Hour

// session is an unfinished resumable upload.
type session struct {
	Object      string `json:"object"`
	ContentType string `json:"contentType"`
	// Limit is the signed maximum size in bytes; 0 has none.
	Limit   int64 `json:"limit"`
	Expires int64 `json:"expires"`
	Done    bool  `json:"done"`
}

var sessionID = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

// Handler serves the signed storage URLs under Prefix.
func (s *Store) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("OPTIONS "+Prefix+"/", func(w http.ResponseWriter, _ *http.Request) {
		cors(w)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST "+Prefix+"/upload", s.start)
	mux.HandleFunc("PUT "+Prefix+"/session/{id}", s.put)
	mux.HandleFunc("GET "+Prefix+"/object", s.get)
	return mux
}

func cors(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Range, Range, x-goog-resumable, x-goog-if-generation-match, x-goog-content-length-range")
	w.Header().Set("Access-Control-Expose-Headers", "Location, Range, Content-Range, Content-Length, ETag")
	w.Header().Set("Access-Control-Max-Age", "3600")
}

func fail(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, message+"\n")
}

func (s *Store) verified(w http.ResponseWriter, r *http.Request, op string) bool {
	switch err := s.verify(op, r.URL.Query()); {
	case err == nil:
		return true
	case errors.Is(err, errExpired):
		fail(w, http.StatusForbidden, "the signed URL has expired")
	default:
		fail(w, http.StatusForbidden, "the signature does not match the request")
	}
	return false
}

// start opens a resumable upload session; its URL is the Location header.
func (s *Store) start(w http.ResponseWriter, r *http.Request) {
	cors(w)
	if !s.verified(w, r, "upload") {
		return
	}
	q := r.URL.Query()
	var limit int64
	if raw := q.Get("limit"); raw != "" {
		limit, _ = strconv.ParseInt(raw, 10, 64)
	}
	// The signed headers must be sent as signed, as a bucket requires.
	switch {
	case r.Header.Get("x-goog-resumable") != "start":
		fail(w, http.StatusBadRequest, "x-goog-resumable: start is required")
		return
	case r.Header.Get("Content-Type") != q.Get("type"),
		r.Header.Get("x-goog-if-generation-match") != "0",
		limit > 0 && r.Header.Get("x-goog-content-length-range") != "0,"+strconv.FormatInt(limit, 10):
		fail(w, http.StatusForbidden, "the signed headers do not match the request")
		return
	}
	object := q.Get("object")
	if _, err := s.attrs(object); err == nil {
		fail(w, http.StatusPreconditionFailed, "the object already exists")
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		fail(w, http.StatusInternalServerError, "could not open an upload session")
		return
	}
	id := base64.RawURLEncoding.EncodeToString(raw)
	sess := session{Object: object, ContentType: q.Get("type"), Limit: limit, Expires: s.now().Add(sessionTTL).Unix()}
	part, meta := s.sessionPaths(id)
	if err := os.WriteFile(part, nil, 0o600); err != nil {
		fail(w, http.StatusInternalServerError, "could not open an upload session")
		return
	}
	if err := writeJSON(meta, sess); err != nil {
		_ = os.Remove(part)
		fail(w, http.StatusInternalServerError, "could not open an upload session")
		return
	}
	w.Header().Set("Location", Prefix+"/session/"+id)
	w.WriteHeader(http.StatusOK)
}

// contentRange is a parsed "bytes first-last/total" header. first is -1 for a
// status query ("bytes */total"); total is -1 when unknown ("/*").
type contentRange struct{ first, last, total int64 }

func parseContentRange(raw string) (contentRange, bool) {
	spec, ok := strings.CutPrefix(raw, "bytes ")
	if !ok {
		return contentRange{}, false
	}
	span, totalRaw, ok := strings.Cut(spec, "/")
	if !ok {
		return contentRange{}, false
	}
	c := contentRange{first: -1, last: -1, total: -1}
	if totalRaw != "*" {
		n, err := strconv.ParseInt(totalRaw, 10, 64)
		if err != nil || n < 0 {
			return contentRange{}, false
		}
		c.total = n
	}
	if span == "*" {
		return c, true
	}
	firstRaw, lastRaw, ok := strings.Cut(span, "-")
	if !ok {
		return contentRange{}, false
	}
	first, err1 := strconv.ParseInt(firstRaw, 10, 64)
	last, err2 := strconv.ParseInt(lastRaw, 10, 64)
	if err1 != nil || err2 != nil || first < 0 || last < first || (c.total >= 0 && last >= c.total) {
		return contentRange{}, false
	}
	c.first, c.last = first, last
	return c, true
}

func (s *Store) sessionPaths(id string) (part, meta string) {
	base := filepath.Join(s.dir, "sessions", id)
	return base + ".part", base + ".json"
}

func (s *Store) lockSession(id string) func() {
	s.mu.Lock()
	l, ok := s.sessionLocks[id]
	if !ok {
		l = &sync.Mutex{}
		s.sessionLocks[id] = l
	}
	s.mu.Unlock()
	l.Lock()
	return l.Unlock
}

// put receives one chunk of a session, or answers a status query. It replies
// 308 with the persisted Range while bytes remain, without Range when none
// are kept, and 200 once the object is committed.
func (s *Store) put(w http.ResponseWriter, r *http.Request) {
	cors(w)
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(transferTimeout))
	_ = rc.SetWriteDeadline(time.Now().Add(transferTimeout))
	id := r.PathValue("id")
	if !sessionID.MatchString(id) {
		fail(w, http.StatusNotFound, "unknown upload session")
		return
	}
	defer s.lockSession(id)()
	part, meta := s.sessionPaths(id)
	var sess session
	if raw, err := os.ReadFile(meta); err != nil || json.Unmarshal(raw, &sess) != nil {
		fail(w, http.StatusNotFound, "unknown upload session")
		return
	}
	if sess.Done {
		w.WriteHeader(http.StatusOK)
		return
	}
	if s.now().Unix() >= sess.Expires {
		s.dropSession(id)
		fail(w, http.StatusGone, "the upload session has expired")
		return
	}
	info, err := os.Stat(part)
	if err != nil {
		fail(w, http.StatusInternalServerError, "could not read the upload session")
		return
	}
	persisted := info.Size()

	// Without Content-Range the body is the whole object.
	c := contentRange{first: 0, last: -1, total: -1}
	whole := r.Header.Get("Content-Range") == ""
	if whole {
		persisted = 0
	} else {
		parsed, ok := parseContentRange(r.Header.Get("Content-Range"))
		if !ok {
			fail(w, http.StatusBadRequest, "malformed Content-Range header")
			return
		}
		c = parsed
	}
	if sess.Limit > 0 && c.total > sess.Limit {
		fail(w, http.StatusBadRequest, "the upload exceeds the signed content length range")
		return
	}
	if c.first < 0 || c.first > persisted || (!whole && c.last < persisted) {
		// A status query, a chunk after a gap or a chunk already kept: report
		// what is kept, or commit when that is everything.
		if c.total >= 0 && persisted == c.total {
			s.finish(w, id, sess, part)
			return
		}
		resume(w, persisted)
		return
	}

	// Skip bytes already kept from an earlier attempt of this chunk.
	if _, err := io.CopyN(io.Discard, r.Body, persisted-c.first); err != nil {
		resume(w, persisted)
		return
	}
	allowed := int64(-1) // bytes this request may still add; -1 is unbounded
	if !whole {
		allowed = c.last - persisted + 1
	}
	if sess.Limit > 0 && (allowed < 0 || persisted+allowed > sess.Limit) {
		allowed = max(sess.Limit-persisted, 0)
	}
	f, err := os.OpenFile(part, os.O_WRONLY, 0o600)
	if err != nil {
		fail(w, http.StatusInternalServerError, "could not write the upload")
		return
	}
	body := io.Reader(r.Body)
	if allowed >= 0 {
		body = io.LimitReader(r.Body, allowed+1)
	}
	_ = f.Truncate(persisted)
	_, _ = f.Seek(persisted, io.SeekStart)
	n, copyErr := io.Copy(f, body)
	size := persisted + n
	if allowed >= 0 && n > allowed {
		_ = f.Truncate(persisted)
		_ = f.Close()
		if sess.Limit > 0 && persisted+n > sess.Limit {
			fail(w, http.StatusBadRequest, "the upload exceeds the signed content length range")
		} else {
			fail(w, http.StatusBadRequest, "the body is longer than its Content-Range")
		}
		return
	}
	if whole && copyErr == nil {
		c.total = size
	}
	final := c.total >= 0 && size == c.total
	if !final {
		// Only whole 256 KiB blocks of an unfinished upload are kept.
		size -= size % chunkQuantum
		_ = f.Truncate(size)
	}
	if err := f.Close(); err != nil {
		fail(w, http.StatusInternalServerError, "could not write the upload")
		return
	}
	if final {
		s.finish(w, id, sess, part)
		return
	}
	resume(w, size)
}

// resume answers 308 with the kept bytes; without a Range header the client
// restarts at 0.
func resume(w http.ResponseWriter, persisted int64) {
	if persisted > 0 {
		w.Header().Set("Range", fmt.Sprintf("bytes=0-%d", persisted-1))
	}
	w.WriteHeader(http.StatusPermanentRedirect)
}

func (s *Store) finish(w http.ResponseWriter, id string, sess session, part string) {
	a, err := s.commit(sess.Object, sess.ContentType, part)
	if errors.Is(err, errExists) {
		s.dropSession(id)
		fail(w, http.StatusPreconditionFailed, "the object already exists")
		return
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, "could not store the object")
		return
	}
	sess.Done = true
	_, meta := s.sessionPaths(id)
	_ = writeJSON(meta, sess)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"bucket": BucketName, "name": a.Object, "generation": strconv.FormatInt(a.Generation, 10),
		"size": strconv.FormatInt(a.Size, 10), "contentType": a.ContentType,
	})
}

func (s *Store) dropSession(id string) {
	part, meta := s.sessionPaths(id)
	_ = os.Remove(part)
	_ = os.Remove(meta)
}

// sweepSessions removes expired sessions left from earlier runs.
func (s *Store) sweepSessions() {
	entries, err := os.ReadDir(filepath.Join(s.dir, "sessions"))
	if err != nil {
		return
	}
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || !sessionID.MatchString(id) {
			continue
		}
		_, meta := s.sessionPaths(id)
		var sess session
		raw, err := os.ReadFile(meta)
		if err != nil || json.Unmarshal(raw, &sess) != nil || s.now().Unix() >= sess.Expires {
			s.dropSession(id)
		}
	}
}

// get serves one generation of an object; ServeContent answers Range and
// HEAD requests, so players can seek and the worker can download.
func (s *Store) get(w http.ResponseWriter, r *http.Request) {
	cors(w)
	if !s.verified(w, r, "object") {
		return
	}
	q := r.URL.Query()
	a, err := s.attrs(q.Get("object"))
	if err != nil || strconv.FormatInt(a.Generation, 10) != q.Get("generation") {
		fail(w, http.StatusNotFound, "no such object generation")
		return
	}
	f, err := os.Open(s.dataPath(a.Object))
	if err != nil {
		fail(w, http.StatusNotFound, "no such object generation")
		return
	}
	defer f.Close()
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(transferTimeout))
	w.Header().Set("Content-Type", a.ContentType)
	w.Header().Set("Cache-Control", "private, max-age=0")
	w.Header().Set("ETag", strconv.Quote(strconv.FormatInt(a.Generation, 10)))
	w.Header().Set("X-Goog-Generation", strconv.FormatInt(a.Generation, 10))
	http.ServeContent(w, r, "", time.UnixMicro(a.Generation), f)
}
