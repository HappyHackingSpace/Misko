// Package application contains video upload and read use cases. Uploads and
// finalization need test:run; listing and read URLs need *:read. Signed URLs
// are returned to the caller only and never stored or logged.
//
// A test has one current video. Starting an upload attaches a new recording
// that supersedes the current one in the same transaction, so the chain stays
// linear; the superseded recordings stay for provenance.
package application

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/media/domain"
	"time"
)

var (
	ErrStorageNotConfigured = errors.New("video storage is not configured")
	ErrTestNotFound         = errors.New("test not found")
	ErrRecordingNotFound    = errors.New("recording not found on this test")
	ErrConcurrentUpload     = errors.New("another video was attached to this test at the same time; try again")
	ErrNotUploader          = errors.New("only the uploader, a lab manager or a super admin can finalize an upload")
	ErrUploadIncomplete     = errors.New("the upload has not finished; finalize after it completes")
	ErrNotVerified          = errors.New("the video has not been verified")
	ErrObjectNotFound       = errors.New("stored object not found")
)

// SignedRequest is a request the caller may send directly to storage.
type SignedRequest struct {
	Method    string
	URL       string
	Headers   map[string]string
	ExpiresAt time.Time
}

// ObjectStore is private object storage for videos.
type ObjectStore interface {
	Bucket() string
	// UploadURL signs the start of a resumable upload that fails if the object
	// already exists. Analysis outputs use it; their size is not known ahead.
	UploadURL(ctx context.Context, object, contentType string, expires time.Time) (SignedRequest, error)
	// SizedUploadURL is UploadURL for an upload of a declared size: storage
	// refuses more than size bytes.
	SizedUploadURL(ctx context.Context, object, contentType string, size int64, expires time.Time) (SignedRequest, error)
	// ReadURL signs a read of one object generation; range requests are allowed.
	ReadURL(ctx context.Context, object string, generation int64, expires time.Time) (string, error)
	// Attrs returns ErrObjectNotFound when the object does not exist yet.
	Attrs(ctx context.Context, object string) (domain.ObjectAttrs, error)
	// Read returns one generation of a small object, such as an analysis
	// trajectory, of at most limit bytes, or ErrObjectNotFound. Videos are never
	// read by the API.
	Read(ctx context.Context, object string, generation, limit int64) ([]byte, error)
}

type TestRef struct {
	ID, ExperimentID, Status string
}

type Store interface {
	Transaction(ctx context.Context, fn func(Store) error) error
	Test(ctx context.Context, id string) (TestRef, error)
	// LockTest reads a test and serializes the videos attached to it.
	LockTest(ctx context.Context, id string) (TestRef, error)
	// CurrentRecording returns the test's newest verified recording, or ErrRecordingNotFound.
	CurrentRecording(ctx context.Context, testID string) (domain.Recording, error)
	// NewestRecordingID returns the end of the test's chain, verified or not, or "".
	NewestRecordingID(ctx context.Context, testID string) (string, error)
	// CreateRecording stores a pending asset with a new object name and its
	// recording, which supersedes supersedesID (empty for the first video).
	CreateRecording(ctx context.Context, test TestRef, asset domain.Asset, clip domain.Clip, supersedesID string) (domain.Recording, error)
	// Recordings lists every recording of a test, oldest first.
	Recordings(ctx context.Context, testID string) ([]domain.Recording, error)
	Recording(ctx context.Context, testID, recordingID string) (domain.Recording, error)
	LockRecording(ctx context.Context, testID, recordingID string) (domain.Recording, error)
	// FinishAsset stores a pending asset's verification result.
	FinishAsset(ctx context.Context, a domain.Asset) (domain.Asset, error)
	// CreateFolderAsset stores a pending original under folder, named after its own id.
	CreateFolderAsset(ctx context.Context, folder string, asset domain.Asset) (domain.Asset, error)
	// Asset returns ErrAssetNotFound for an unknown id.
	Asset(ctx context.Context, id string) (domain.Asset, error)
	// LockAsset reads an asset and locks it until the transaction ends.
	LockAsset(ctx context.Context, id string) (domain.Asset, error)
}

type Settings struct {
	MaxBytes           int64
	UploadTTL, ReadTTL time.Duration
}

type Service struct {
	store    Store
	objects  ObjectStore
	settings Settings
	now      func() time.Time
}

// New builds the service. A nil objects store means storage is not configured.
func New(store Store, objects ObjectStore, settings Settings, now func() time.Time) *Service {
	return &Service{store: store, objects: objects, settings: settings, now: now}
}

type Upload struct {
	Recording domain.Recording
	Request   SignedRequest
}

// StartUpload records a pending original video for a test that has started,
// appends it to the test's chain (it becomes current once verified) and signs the resumable upload the caller
// sends straight to storage, limited to the declared size. The upload is
// signed inside the transaction, so a signing failure attaches nothing.
func (s *Service) StartUpload(ctx context.Context, actor access.Actor, testID string, req domain.UploadRequest) (Upload, error) {
	if err := actor.Require(access.TestRun); err != nil {
		return Upload{}, err
	}
	if s.objects == nil {
		return Upload{}, ErrStorageNotConfigured
	}
	asset, clip, err := domain.NewUpload(req, s.settings.MaxBytes, actor.UserID)
	if err != nil {
		return Upload{}, err
	}
	asset.Bucket = s.objects.Bucket()
	var upload Upload
	err = s.store.Transaction(ctx, func(tx Store) error {
		test, err := tx.LockTest(ctx, testID)
		if err != nil {
			return err
		}
		if err := domain.AcceptsVideo(test.Status); err != nil {
			return err
		}
		supersedes, err := tx.NewestRecordingID(ctx, test.ID)
		if err != nil {
			return err
		}
		rec, err := tx.CreateRecording(ctx, test, asset, clip, supersedes)
		if err != nil {
			return err
		}
		request, err := s.objects.SizedUploadURL(ctx, rec.Asset.ObjectName, rec.Asset.ContentType, rec.Asset.SizeBytes, s.now().Add(s.settings.UploadTTL))
		if err != nil {
			return err
		}
		upload = Upload{Recording: rec, Request: request}
		return nil
	})
	if err != nil {
		return Upload{}, err
	}
	return upload, nil
}

// FinalizeUpload reads the stored object and records whether it matches the
// upload. The object is read before the row is locked because storage and the
// database share no transaction. Finalizing again returns the stored result.
func (s *Service) FinalizeUpload(ctx context.Context, actor access.Actor, testID, recordingID string) (domain.Recording, error) {
	if err := actor.Require(access.TestRun); err != nil {
		return domain.Recording{}, err
	}
	if s.objects == nil {
		return domain.Recording{}, ErrStorageNotConfigured
	}
	rec, err := s.store.Recording(ctx, testID, recordingID)
	if err != nil {
		return domain.Recording{}, err
	}
	if rec.Asset.CreatedBy != actor.UserID && !actor.Role.Privileged() {
		return domain.Recording{}, ErrNotUploader
	}
	if rec.Asset.Status != domain.Pending {
		return rec, nil
	}
	attrs, err := s.objects.Attrs(ctx, rec.Asset.ObjectName)
	if errors.Is(err, ErrObjectNotFound) {
		return domain.Recording{}, ErrUploadIncomplete
	}
	if err != nil {
		return domain.Recording{}, err
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	err = s.store.Transaction(ctx, func(tx Store) error {
		locked, err := tx.LockRecording(ctx, testID, recordingID)
		if err != nil {
			return err
		}
		if locked.Asset.Status != domain.Pending {
			rec = locked
			return nil
		}
		checked, err := locked.Asset.Verify(attrs, s.settings.MaxBytes, now)
		if err != nil {
			return err
		}
		if locked.Asset, err = tx.FinishAsset(ctx, checked); err != nil {
			return err
		}
		rec = locked
		return nil
	})
	if err != nil {
		return domain.Recording{}, err
	}
	return rec, nil
}

func (s *Service) Recordings(ctx context.Context, actor access.Actor, testID string) ([]domain.Recording, error) {
	if err := actor.Require(access.Read); err != nil {
		return nil, err
	}
	if _, err := s.store.Test(ctx, testID); err != nil {
		return nil, err
	}
	return s.store.Recordings(ctx, testID)
}

// Current returns the test's current recording, or ErrRecordingNotFound
// while it has no video.
func (s *Service) Current(ctx context.Context, actor access.Actor, testID string) (domain.Recording, error) {
	if err := actor.Require(access.Read); err != nil {
		return domain.Recording{}, err
	}
	if _, err := s.store.Test(ctx, testID); err != nil {
		return domain.Recording{}, err
	}
	return s.store.CurrentRecording(ctx, testID)
}

type ReadLink struct {
	URL       string
	ExpiresAt time.Time
}

// ReadURL signs a short-lived read of the verified generation of a recording's video.
func (s *Service) ReadURL(ctx context.Context, actor access.Actor, testID, recordingID string) (ReadLink, error) {
	if err := actor.Require(access.Read); err != nil {
		return ReadLink{}, err
	}
	if s.objects == nil {
		return ReadLink{}, ErrStorageNotConfigured
	}
	rec, err := s.store.Recording(ctx, testID, recordingID)
	if err != nil {
		return ReadLink{}, err
	}
	if rec.Asset.Status != domain.Verified {
		return ReadLink{}, ErrNotVerified
	}
	expires := s.now().Add(s.settings.ReadTTL)
	url, err := s.objects.ReadURL(ctx, rec.Asset.ObjectName, rec.Asset.Generation, expires)
	if err != nil {
		return ReadLink{}, err
	}
	return ReadLink{URL: url, ExpiresAt: expires}, nil
}
