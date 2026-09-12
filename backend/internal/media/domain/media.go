// Package domain holds video assets and test recordings. An asset is one
// object in private cloud storage, identified by bucket, object name and
// generation once verified. A recording links a verified original to a test.
package domain

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalidContentType = errors.New("content type must be video/mp4, video/quicktime or video/webm")
	ErrInvalidSize        = errors.New("size must be at least 1 byte and at most the configured maximum")
	ErrInvalidChecksum    = errors.New("crc32c must be the base64 encoding of 4 big-endian bytes")
	ErrInvalidFileName    = errors.New("file name must contain 1 to 255 printable characters without path separators")
	ErrInvalidClip        = errors.New("clip start must be non-negative and the clip end after its start")
	ErrInvalidObject      = errors.New("stored object has no generation")
	ErrAlreadyFinalized   = errors.New("the upload was already finalized")
)

type Kind string

const (
	Original Kind = "ORIGINAL"
	Analyzed Kind = "ANALYZED"
)

type Status string

const (
	Pending  Status = "PENDING"
	Verified Status = "VERIFIED"
	Rejected Status = "REJECTED"
)

// Reasons a finished upload is rejected.
const (
	RejectedOversized           = "OVERSIZED"
	RejectedSizeMismatch        = "SIZE_MISMATCH"
	RejectedChecksumMismatch    = "CHECKSUM_MISMATCH"
	RejectedContentTypeMismatch = "CONTENT_TYPE_MISMATCH"
)

var contentTypes = []string{"video/mp4", "video/quicktime", "video/webm"}

// Asset is a video object. Generation is set when the stored object was read;
// a verified asset never changes.
type Asset struct {
	ID              string
	Kind            Kind
	Bucket          string
	ObjectName      string
	ContentType     string
	FileName        string
	SizeBytes       int64
	CRC32C          uint32
	Status          Status
	Generation      int64
	RejectionReason string
	CreatedBy       string
	CreatedAt       time.Time
	VerifiedAt      *time.Time
}

// Clip is the part of a source video that belongs to the test, in microseconds
// from the start of the video. A nil EndUs runs to the end of the video.
type Clip struct {
	StartUs int64
	EndUs   *int64
}

type Recording struct {
	ID           string
	ExperimentID string
	TestID       string
	Clip         Clip
	Asset        Asset
	CreatedBy    string
	CreatedAt    time.Time
}

type UploadRequest struct {
	FileName    string
	ContentType string
	SizeBytes   int64
	// CRC32C is the base64 encoding of the big-endian checksum, as reported by GCS.
	CRC32C      string
	ClipStartUs int64
	ClipEndUs   *int64
}

// ObjectAttrs are the attributes of a stored object.
type ObjectAttrs struct {
	Generation  int64
	Size        int64
	CRC32C      uint32
	ContentType string
}

// NewUpload validates what the uploader declares about an original video.
func NewUpload(req UploadRequest, maxBytes int64, createdBy string) (Asset, Clip, error) {
	switch {
	case !contains(contentTypes, req.ContentType):
		return Asset{}, Clip{}, ErrInvalidContentType
	case req.SizeBytes < 1 || req.SizeBytes > maxBytes:
		return Asset{}, Clip{}, ErrInvalidSize
	}
	checksum, err := ParseCRC32C(req.CRC32C)
	if err != nil {
		return Asset{}, Clip{}, err
	}
	name := strings.TrimSpace(req.FileName)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 255 || strings.ContainsAny(name, `/\`) || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return Asset{}, Clip{}, ErrInvalidFileName
	}
	if req.ClipStartUs < 0 || (req.ClipEndUs != nil && *req.ClipEndUs <= req.ClipStartUs) {
		return Asset{}, Clip{}, ErrInvalidClip
	}
	asset := Asset{Kind: Original, ContentType: req.ContentType, FileName: name, SizeBytes: req.SizeBytes, CRC32C: checksum, Status: Pending, CreatedBy: createdBy}
	return asset, Clip{StartUs: req.ClipStartUs, EndUs: req.ClipEndUs}, nil
}

// Verify compares the stored object with the declared upload. A mismatch
// rejects the asset; it is not an error, because the rejection is recorded.
func (a Asset) Verify(attrs ObjectAttrs, maxBytes int64, now time.Time) (Asset, error) {
	if a.Status != Pending {
		return Asset{}, ErrAlreadyFinalized
	}
	if attrs.Generation <= 0 {
		return Asset{}, ErrInvalidObject
	}
	a.Generation = attrs.Generation
	switch {
	case attrs.Size > maxBytes:
		a.Status, a.RejectionReason = Rejected, RejectedOversized
	case attrs.Size != a.SizeBytes:
		a.Status, a.RejectionReason = Rejected, RejectedSizeMismatch
	case attrs.CRC32C != a.CRC32C:
		a.Status, a.RejectionReason = Rejected, RejectedChecksumMismatch
	case attrs.ContentType != a.ContentType:
		a.Status, a.RejectionReason = Rejected, RejectedContentTypeMismatch
	default:
		a.Status, a.VerifiedAt = Verified, &now
	}
	return a, nil
}

func ParseCRC32C(raw string) (uint32, error) {
	b, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(b) != 4 {
		return 0, ErrInvalidChecksum
	}
	return binary.BigEndian.Uint32(b), nil
}

func FormatCRC32C(v uint32) string {
	return base64.StdEncoding.EncodeToString(binary.BigEndian.AppendUint32(nil, v))
}

func contains(values []string, v string) bool {
	for _, candidate := range values {
		if candidate == v {
			return true
		}
	}
	return false
}
