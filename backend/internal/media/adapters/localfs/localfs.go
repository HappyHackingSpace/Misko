// Package localfs stores videos and analysis outputs as files in a local
// directory, for development and single machine installations without Google
// Cloud Storage. It speaks the same signed URL protocol the panel and the
// worker use with a bucket: a signed POST starts a resumable upload, chunks
// are PUT to the returned session URL, and signed reads pin one generation and
// answer Range requests. Its HTTP handler is mounted on the API under Prefix.
//
// Layout under the directory: objects/<sha256(name)> holds the bytes and
// objects/<sha256(name)>.json the attributes; sessions/<id>.part and
// sessions/<id>.json hold unfinished uploads. Objects are never overwritten,
// as with x-goog-if-generation-match: 0 on a bucket.
package localfs

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/HappyHackingSpace/Misko/backend/internal/media/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/media/domain"
)

const (
	// Prefix is the URL path the handler is mounted under on the API.
	Prefix = "/api/local-storage"
	// BucketName is recorded as the bucket of every asset kept here.
	BucketName = "local-storage"
	// sessionTTL is how long an upload session stays usable, as on a bucket.
	sessionTTL = 7 * 24 * time.Hour
)

var castagnoli = crc32.MakeTable(crc32.Castagnoli)

// attrs is the stored metadata of one finished object.
type attrs struct {
	Object      string `json:"object"`
	Generation  int64  `json:"generation"`
	Size        int64  `json:"size"`
	CRC32C      uint32 `json:"crc32c"`
	ContentType string `json:"contentType"`
}

type Store struct {
	dir string
	key []byte
	now func() time.Time

	// mu guards the generation counter, object creation and sessionLocks.
	mu           sync.Mutex
	generation   int64
	sessionLocks map[string]*sync.Mutex
}

// New opens or creates the store under dir. Signatures use a key derived from
// secret, so URLs signed before a restart stay valid until they expire.
func New(dir string, secret []byte, now func() time.Time) (*Store, error) {
	if len(secret) < 32 {
		return nil, errors.New("local storage signing secret must contain at least 32 bytes")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve local storage directory: %w", err)
	}
	for _, sub := range []string{"objects", "sessions"} {
		if err := os.MkdirAll(filepath.Join(abs, sub), 0o700); err != nil {
			return nil, fmt.Errorf("create local storage directory: %w", err)
		}
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("misko local storage url signing v1"))
	s := &Store{dir: abs, key: mac.Sum(nil), now: now, sessionLocks: map[string]*sync.Mutex{}}
	if raw, err := os.ReadFile(filepath.Join(abs, "generation")); err == nil {
		s.generation, _ = strconv.ParseInt(string(raw), 10, 64)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read generation counter: %w", err)
	}
	s.sweepSessions()
	return s, nil
}

func (s *Store) Bucket() string { return BucketName }

// UploadURL signs the POST that starts a resumable upload of a new object.
func (s *Store) UploadURL(_ context.Context, object, contentType string, expires time.Time) (application.SignedRequest, error) {
	return s.signUpload(object, contentType, 0, expires)
}

// SizedUploadURL is UploadURL limited to size bytes, as the signed
// x-goog-content-length-range header does on a bucket.
func (s *Store) SizedUploadURL(_ context.Context, object, contentType string, size int64, expires time.Time) (application.SignedRequest, error) {
	if size <= 0 {
		return application.SignedRequest{}, errors.New("upload size must be positive")
	}
	return s.signUpload(object, contentType, size, expires)
}

func (s *Store) signUpload(object, contentType string, limit int64, expires time.Time) (application.SignedRequest, error) {
	if object == "" {
		return application.SignedRequest{}, errors.New("object name is required")
	}
	values := url.Values{"object": {object}, "type": {contentType}, "expires": {strconv.FormatInt(expires.Unix(), 10)}}
	headers := map[string]string{"Content-Type": contentType, "x-goog-resumable": "start", "x-goog-if-generation-match": "0"}
	if limit > 0 {
		values.Set("limit", strconv.FormatInt(limit, 10))
		headers["x-goog-content-length-range"] = "0," + strconv.FormatInt(limit, 10)
	}
	return application.SignedRequest{Method: http.MethodPost, URL: s.signedURL("upload", values), Headers: headers, ExpiresAt: expires}, nil
}

// ReadURL signs a GET of one generation; the handler answers Range requests.
func (s *Store) ReadURL(_ context.Context, object string, generation int64, expires time.Time) (string, error) {
	values := url.Values{"object": {object}, "generation": {strconv.FormatInt(generation, 10)}, "expires": {strconv.FormatInt(expires.Unix(), 10)}}
	return s.signedURL("object", values), nil
}

func (s *Store) Attrs(_ context.Context, object string) (domain.ObjectAttrs, error) {
	a, err := s.attrs(object)
	if err != nil {
		return domain.ObjectAttrs{}, err
	}
	return domain.ObjectAttrs{Generation: a.Generation, Size: a.Size, CRC32C: a.CRC32C, ContentType: a.ContentType}, nil
}

// Read returns one generation of a small object of at most limit bytes.
func (s *Store) Read(_ context.Context, object string, generation, limit int64) ([]byte, error) {
	a, err := s.attrs(object)
	if err != nil {
		return nil, err
	}
	if a.Generation != generation {
		return nil, application.ErrObjectNotFound
	}
	if a.Size > limit {
		return nil, fmt.Errorf("object exceeds %d bytes", limit)
	}
	data, err := os.ReadFile(s.dataPath(object))
	if err != nil {
		return nil, fmt.Errorf("read object: %w", err)
	}
	return data, nil
}

// signedURL returns a relative URL: the panel reaches the API on its own
// origin, and the worker prefixes its API base URL.
func (s *Store) signedURL(op string, values url.Values) string {
	values.Set("sig", s.signature(op, values))
	return Prefix + "/" + op + "?" + values.Encode()
}

// signature covers the operation and every query value except sig itself.
func (s *Store) signature(op string, values url.Values) string {
	clean := url.Values{}
	for k, v := range values {
		if k != "sig" {
			clean[k] = v
		}
	}
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(op + "?" + clean.Encode()))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

var (
	errBadSignature = errors.New("the signature does not match")
	errExpired      = errors.New("the signed URL has expired")
)

// verify checks the signature in constant time, then the expiry.
func (s *Store) verify(op string, values url.Values) error {
	got, err := base64.RawURLEncoding.DecodeString(values.Get("sig"))
	if err != nil {
		return errBadSignature
	}
	want, _ := base64.RawURLEncoding.DecodeString(s.signature(op, values))
	if !hmac.Equal(got, want) {
		return errBadSignature
	}
	expires, err := strconv.ParseInt(values.Get("expires"), 10, 64)
	if err != nil {
		return errBadSignature
	}
	if !s.now().Before(time.Unix(expires, 0)) {
		return errExpired
	}
	return nil
}

func (s *Store) objectKey(object string) string {
	sum := sha256.Sum256([]byte(object))
	return hex.EncodeToString(sum[:])
}

func (s *Store) dataPath(object string) string {
	return filepath.Join(s.dir, "objects", s.objectKey(object))
}

func (s *Store) attrsPath(object string) string { return s.dataPath(object) + ".json" }

func (s *Store) attrs(object string) (attrs, error) {
	raw, err := os.ReadFile(s.attrsPath(object))
	if errors.Is(err, os.ErrNotExist) {
		return attrs{}, application.ErrObjectNotFound
	}
	if err != nil {
		return attrs{}, fmt.Errorf("read object attributes: %w", err)
	}
	var a attrs
	if err := json.Unmarshal(raw, &a); err != nil || a.Object != object {
		return attrs{}, errors.New("object attributes are corrupt")
	}
	return a, nil
}

// errExists is the if-generation-match: 0 precondition failing.
var errExists = errors.New("the object already exists")

// commit turns a finished session file into a new object generation. The
// attributes are written last, so a half committed object is never visible.
func (s *Store) commit(object, contentType, part string) (attrs, error) {
	size, crc, err := checksum(part)
	if err != nil {
		return attrs{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := os.Stat(s.attrsPath(object)); err == nil {
		return attrs{}, errExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return attrs{}, err
	}
	generation, err := s.nextGeneration()
	if err != nil {
		return attrs{}, err
	}
	a := attrs{Object: object, Generation: generation, Size: size, CRC32C: crc, ContentType: contentType}
	if err := os.Rename(part, s.dataPath(object)); err != nil {
		return attrs{}, fmt.Errorf("store object: %w", err)
	}
	if err := writeJSON(s.attrsPath(object), a); err != nil {
		_ = os.Remove(s.dataPath(object))
		return attrs{}, err
	}
	return a, nil
}

// nextGeneration is the clock in microseconds, like a bucket's generations,
// but always above the last one issued, even after the clock steps back.
// Callers hold mu.
func (s *Store) nextGeneration() (int64, error) {
	g := max(s.now().UnixMicro(), s.generation+1)
	path := filepath.Join(s.dir, "generation")
	if err := os.WriteFile(path+".tmp", []byte(strconv.FormatInt(g, 10)), 0o600); err != nil {
		return 0, fmt.Errorf("write generation counter: %w", err)
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		return 0, fmt.Errorf("write generation counter: %w", err)
	}
	s.generation = g
	return g, nil
}

func checksum(path string) (int64, uint32, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, fmt.Errorf("open upload: %w", err)
	}
	defer f.Close()
	h := crc32.New(castagnoli)
	n, err := io.Copy(h, f)
	if err != nil {
		return 0, 0, fmt.Errorf("checksum upload: %w", err)
	}
	return n, h.Sum32(), nil
}

// writeJSON replaces path atomically.
func writeJSON(path string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path+".tmp", raw, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	return os.Rename(path+".tmp", path)
}
