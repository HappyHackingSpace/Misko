// Package gcs stores videos in a private Google Cloud Storage bucket with the
// native Go client and Application Default Credentials. Signed URLs need a
// service account: either the credentials carry a key, or SignerEmail names a
// service account that signs through the IAM Credentials API.
package gcs

import (
	"cloud.google.com/go/storage"
	"context"
	"errors"
	"fmt"
	"github.com/HappyHackingSpace/Misko/backend/internal/media/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/media/domain"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type Store struct {
	client *storage.Client
	bucket string
	signer string
	// privateKey signs locally; nil uses the client's credentials or IAM.
	privateKey []byte
}

func New(ctx context.Context, bucket, signerEmail string) (*Store, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("create storage client: %w", err)
	}
	return &Store{client: client, bucket: bucket, signer: signerEmail}, nil
}

func (s *Store) Close() error { return s.client.Close() }

func (s *Store) Bucket() string { return s.bucket }

// UploadURL signs the POST that starts a resumable upload. The client must send
// the returned headers; x-goog-if-generation-match: 0 makes the upload fail if
// the object already exists, so a finished video is never overwritten.
func (s *Store) UploadURL(_ context.Context, object, contentType string, expires time.Time) (application.SignedRequest, error) {
	opts := s.options(http.MethodPost, expires)
	opts.ContentType = contentType
	opts.Headers = []string{"x-goog-resumable:start", "x-goog-if-generation-match:0"}
	signed, err := s.client.Bucket(s.bucket).SignedURL(object, opts)
	if err != nil {
		return application.SignedRequest{}, fmt.Errorf("sign upload: %w", err)
	}
	return application.SignedRequest{
		Method: http.MethodPost, URL: signed, ExpiresAt: expires,
		Headers: map[string]string{"Content-Type": contentType, "x-goog-resumable": "start", "x-goog-if-generation-match": "0"},
	}, nil
}

// ReadURL signs a GET of one generation; GCS serves Range requests on it.
func (s *Store) ReadURL(_ context.Context, object string, generation int64, expires time.Time) (string, error) {
	opts := s.options(http.MethodGet, expires)
	opts.QueryParameters = url.Values{"generation": {strconv.FormatInt(generation, 10)}}
	signed, err := s.client.Bucket(s.bucket).SignedURL(object, opts)
	if err != nil {
		return "", fmt.Errorf("sign read: %w", err)
	}
	return signed, nil
}

func (s *Store) Attrs(ctx context.Context, object string) (domain.ObjectAttrs, error) {
	attrs, err := s.client.Bucket(s.bucket).Object(object).Attrs(ctx)
	if errors.Is(err, storage.ErrObjectNotExist) {
		return domain.ObjectAttrs{}, application.ErrObjectNotFound
	}
	if err != nil {
		return domain.ObjectAttrs{}, fmt.Errorf("read object attributes: %w", err)
	}
	return domain.ObjectAttrs{Generation: attrs.Generation, Size: attrs.Size, CRC32C: attrs.CRC32C, ContentType: attrs.ContentType}, nil
}

// Read downloads one generation of a small object, such as a trajectory. An
// object larger than limit is an error, so a large file never fills memory.
func (s *Store) Read(ctx context.Context, object string, generation, limit int64) ([]byte, error) {
	reader, err := s.client.Bucket(s.bucket).Object(object).Generation(generation).NewReader(ctx)
	if errors.Is(err, storage.ErrObjectNotExist) {
		return nil, application.ErrObjectNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("open object: %w", err)
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read object: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("object exceeds %d bytes", limit)
	}
	return data, nil
}

func (s *Store) options(method string, expires time.Time) *storage.SignedURLOptions {
	return &storage.SignedURLOptions{Scheme: storage.SigningSchemeV4, Method: method, Expires: expires, GoogleAccessID: s.signer, PrivateKey: s.privateKey}
}
