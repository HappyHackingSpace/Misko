package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const maxBytes = 1 << 30

// "AAAAAQ==" is the base64 encoding of the big-endian CRC32C value 1.
func upload() UploadRequest {
	return UploadRequest{FileName: " trial-1.mp4 ", ContentType: "video/mp4", SizeBytes: 1024, CRC32C: "AAAAAQ=="}
}

func TestUploadRequestsAreValidated(t *testing.T) {
	end := int64(5_000_000)
	req := upload()
	req.ClipStartUs, req.ClipEndUs = 1_000_000, &end
	asset, clip, err := NewUpload(req, maxBytes, "user")
	if err != nil {
		t.Fatal(err)
	}
	if asset.Kind != Original || asset.Status != Pending || asset.FileName != "trial-1.mp4" || asset.SizeBytes != 1024 ||
		asset.CRC32C != 1 || asset.ContentType != "video/mp4" || asset.CreatedBy != "user" || asset.Generation != 0 {
		t.Fatalf("asset: %+v", asset)
	}
	if clip.StartUs != 1_000_000 || clip.EndUs == nil || *clip.EndUs != end {
		t.Fatalf("clip: %+v", clip)
	}
	before := int64(500_000)
	for name, tc := range map[string]struct {
		mutate func(*UploadRequest)
		want   error
	}{
		"image":               {func(r *UploadRequest) { r.ContentType = "image/png" }, ErrInvalidContentType},
		"content type params": {func(r *UploadRequest) { r.ContentType = "video/mp4; codecs=avc1" }, ErrInvalidContentType},
		"empty file":          {func(r *UploadRequest) { r.SizeBytes = 0 }, ErrInvalidSize},
		"over the maximum":    {func(r *UploadRequest) { r.SizeBytes = maxBytes + 1 }, ErrInvalidSize},
		"checksum not base64": {func(r *UploadRequest) { r.CRC32C = "not base64!" }, ErrInvalidChecksum},
		"checksum too long":   {func(r *UploadRequest) { r.CRC32C = "AAAAAAE=" }, ErrInvalidChecksum},
		"no checksum":         {func(r *UploadRequest) { r.CRC32C = "" }, ErrInvalidChecksum},
		"no file name":        {func(r *UploadRequest) { r.FileName = " " }, ErrInvalidFileName},
		"path in file name":   {func(r *UploadRequest) { r.FileName = "../trial.mp4" }, ErrInvalidFileName},
		"long file name":      {func(r *UploadRequest) { r.FileName = strings.Repeat("a", 256) }, ErrInvalidFileName},
		"negative clip start": {func(r *UploadRequest) { r.ClipStartUs = -1 }, ErrInvalidClip},
		"clip ends early":     {func(r *UploadRequest) { r.ClipStartUs, r.ClipEndUs = 1_000_000, &before }, ErrInvalidClip},
	} {
		req := upload()
		tc.mutate(&req)
		if _, _, err := NewUpload(req, maxBytes, "user"); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}
	if _, _, err := NewUpload(UploadRequest{FileName: "a.mov", ContentType: "video/quicktime", SizeBytes: maxBytes, CRC32C: "AAAAAQ=="}, maxBytes, "user"); err != nil {
		t.Fatalf("maximum size: %v", err)
	}
}

// Verification compares the stored object with what the uploader declared.
func TestVerifyComparesTheStoredObject(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	pending, _, err := NewUpload(upload(), maxBytes, "user")
	if err != nil {
		t.Fatal(err)
	}
	good := ObjectAttrs{Generation: 1757592000000001, Size: 1024, CRC32C: 1, ContentType: "video/mp4"}
	verified, err := pending.Verify(good, maxBytes, now)
	if err != nil || verified.Status != Verified || verified.Generation != good.Generation || verified.VerifiedAt == nil || !verified.VerifiedAt.Equal(now) {
		t.Fatalf("verified: %+v %v", verified, err)
	}
	for name, tc := range map[string]struct {
		attrs  ObjectAttrs
		reason string
	}{
		"partial object":         {ObjectAttrs{Generation: 2, Size: 512, CRC32C: 1, ContentType: "video/mp4"}, RejectedSizeMismatch},
		"oversized object":       {ObjectAttrs{Generation: 2, Size: maxBytes + 1, CRC32C: 1, ContentType: "video/mp4"}, RejectedOversized},
		"corrupt object":         {ObjectAttrs{Generation: 2, Size: 1024, CRC32C: 7, ContentType: "video/mp4"}, RejectedChecksumMismatch},
		"different content type": {ObjectAttrs{Generation: 2, Size: 1024, CRC32C: 1, ContentType: "text/html"}, RejectedContentTypeMismatch},
	} {
		rejected, err := pending.Verify(tc.attrs, maxBytes, now)
		if err != nil || rejected.Status != Rejected || rejected.RejectionReason != tc.reason || rejected.Generation != tc.attrs.Generation {
			t.Errorf("%s: %+v %v", name, rejected, err)
		}
	}
	if _, err := pending.Verify(ObjectAttrs{Size: 1024, CRC32C: 1, ContentType: "video/mp4"}, maxBytes, now); !errors.Is(err, ErrInvalidObject) {
		t.Errorf("object without generation: %v", err)
	}
	for _, done := range []Asset{verified, func() Asset {
		r, _ := pending.Verify(ObjectAttrs{Generation: 2, Size: 1, CRC32C: 1, ContentType: "video/mp4"}, maxBytes, now)
		return r
	}()} {
		if _, err := done.Verify(good, maxBytes, now); !errors.Is(err, ErrAlreadyFinalized) {
			t.Errorf("%s asset verified again: %v", done.Status, err)
		}
	}
}

func TestParseCRC32C(t *testing.T) {
	for raw, want := range map[string]uint32{"AAAAAA==": 0, "/////w==": 4294967295, "4pMtGw==": 0xe2932d1b} {
		if got, err := ParseCRC32C(raw); err != nil || got != want {
			t.Errorf("%s: %d %v want %d", raw, got, err, want)
		}
	}
	if FormatCRC32C(0xe2932d1b) != "4pMtGw==" {
		t.Error("format must round-trip")
	}
}
