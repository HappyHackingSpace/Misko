package application

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/media/domain"
	"slices"
	"strings"
	"testing"
	"time"
)

var (
	ctx      = context.Background()
	now      = time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	clock    = func() time.Time { return now }
	settings = Settings{MaxBytes: 1 << 30, UploadTTL: time.Hour, ReadTTL: 15 * time.Minute}
)

var errStoreCalled = errors.New("store called")

func guard(fn func() error) (err error) {
	defer func() {
		if recover() != nil {
			err = errStoreCalled
		}
	}()
	return fn()
}

func request() domain.UploadRequest {
	return domain.UploadRequest{FileName: "trial.mp4", ContentType: "video/mp4", SizeBytes: 1024, CRC32C: "AAAAAQ=="}
}

func TestEveryOperationRequiresItsPermission(t *testing.T) {
	operations := []struct {
		name       string
		permission access.Permission
		run        func(*Service, access.Actor) error
	}{
		{"start upload", access.TestRun, func(s *Service, a access.Actor) error { return second(s.StartUpload(ctx, a, "test", request())) }},
		{"finalize upload", access.TestRun, func(s *Service, a access.Actor) error { return second(s.FinalizeUpload(ctx, a, "test", "rec")) }},
		{"list recordings", access.Read, func(s *Service, a access.Actor) error { return second(s.Recordings(ctx, a, "test")) }},
		{"read URL", access.Read, func(s *Service, a access.Actor) error { return second(s.ReadURL(ctx, a, "test", "rec")) }},
	}
	for _, role := range append(access.Roles(), "ROOT") {
		for _, op := range operations {
			err := guard(func() error {
				return op.run(New(&fakeStore{}, &fakeObjects{}, settings, clock), access.Actor{UserID: "u", Role: role})
			})
			if role.Allows(op.permission) {
				if errors.Is(err, access.ErrForbidden) {
					t.Errorf("%s %s: denied", role, op.name)
				}
				continue
			}
			if !errors.Is(err, access.ErrForbidden) {
				t.Errorf("%s %s: err=%v want forbidden before any call", role, op.name, err)
			}
		}
		for _, op := range operations {
			if err := guard(func() error {
				return op.run(New(&fakeStore{}, nil, settings, clock), access.Actor{UserID: "u", Role: access.SuperAdmin})
			}); op.name != "list recordings" && !errors.Is(err, ErrStorageNotConfigured) {
				t.Errorf("%s without storage: %v", op.name, err)
			}
		}
	}
}

func TestStartUploadSignsANewObjectOnly(t *testing.T) {
	store, objects := &fakeStore{}, &fakeObjects{}
	technician := access.Actor{UserID: "tech", Role: access.Technician}
	upload, err := New(store, objects, settings, clock).StartUpload(ctx, technician, "test", request())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(store.calls, []string{"Transaction", "Test", "CreateRecording"}) {
		t.Fatalf("store calls: %v", store.calls)
	}
	rec := upload.Recording
	if rec.Asset.Status != domain.Pending || rec.Asset.CreatedBy != "tech" || rec.Asset.Bucket != "misko-videos" || rec.TestID != "test" {
		t.Fatalf("recording: %+v", rec)
	}
	if upload.Request.URL != "signed-upload:"+rec.Asset.ObjectName || !upload.Request.ExpiresAt.Equal(now.Add(time.Hour)) || objects.signedType != "video/mp4" {
		t.Fatalf("upload request: %+v", upload.Request)
	}
	for name, tc := range map[string]struct {
		store *fakeStore
		req   domain.UploadRequest
		want  error
	}{
		"cancelled test": {&fakeStore{status: "CANCELLED"}, request(), ErrTestCancelled},
		"unknown test":   {&fakeStore{missing: true}, request(), ErrTestNotFound},
		"invalid upload": {&fakeStore{}, func() domain.UploadRequest { r := request(); r.SizeBytes = 2 << 30; return r }(), domain.ErrInvalidSize},
	} {
		if _, err := New(tc.store, &fakeObjects{}, settings, clock).StartUpload(ctx, technician, "test", tc.req); !errors.Is(err, tc.want) || slices.Contains(tc.store.calls, "CreateRecording") {
			t.Errorf("%s: err=%v calls=%v want %v", name, err, tc.store.calls, tc.want)
		}
	}
}

func TestFinalizeVerifiesBeforeRecording(t *testing.T) {
	uploader := access.Actor{UserID: "tech", Role: access.Technician}
	for name, tc := range map[string]struct {
		actor   access.Actor
		objects *fakeObjects
		status  domain.Status
		reason  string
		want    error
	}{
		"another technician":       {access.Actor{UserID: "other", Role: access.Technician}, &fakeObjects{attrs: good()}, domain.Pending, "", ErrNotUploader},
		"upload not finished":      {uploader, &fakeObjects{}, domain.Pending, "", ErrUploadIncomplete},
		"verified by the uploader": {uploader, &fakeObjects{attrs: good()}, domain.Verified, "", nil},
		"lab manager on behalf":    {access.Actor{UserID: "manager", Role: access.LabManager}, &fakeObjects{attrs: good()}, domain.Verified, "", nil},
		"corrupt upload": {uploader, &fakeObjects{attrs: func() *domain.ObjectAttrs { a := good(); a.CRC32C = 9; return a }()},
			domain.Rejected, domain.RejectedChecksumMismatch, nil},
	} {
		store := &fakeStore{}
		rec, err := New(store, tc.objects, settings, clock).FinalizeUpload(ctx, tc.actor, "test", "rec")
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
			continue
		}
		if tc.want != nil {
			if slices.Contains(store.calls, "FinishAsset") {
				t.Errorf("%s: asset changed", name)
			}
			continue
		}
		if rec.Asset.Status != tc.status || rec.Asset.RejectionReason != tc.reason || !slices.Contains(store.calls, "FinishAsset") {
			t.Errorf("%s: %+v calls=%v", name, rec.Asset, store.calls)
		}
		// The object is read before the row is locked; GCS and PostgreSQL share no transaction.
		if i, j := slices.Index(tc.objects.calls, "Attrs"), slices.Index(store.calls, "LockRecording"); i < 0 || j < 0 {
			t.Errorf("%s: attrs=%v lock=%v", name, tc.objects.calls, store.calls)
		}
	}

	done := &fakeStore{asset: func() *domain.Asset {
		a := pendingAsset()
		a.Status, a.Generation = domain.Verified, 7
		v := now
		a.VerifiedAt = &v
		return &a
	}()}
	objects := &fakeObjects{attrs: good()}
	rec, err := New(done, objects, settings, clock).FinalizeUpload(ctx, uploader, "test", "rec")
	if err != nil || rec.Asset.Generation != 7 || len(objects.calls) > 0 || slices.Contains(done.calls, "FinishAsset") {
		t.Fatalf("repeated finalization must return the stored result: %+v %v %v %v", rec, err, objects.calls, done.calls)
	}
}

func TestReadURLNeedsAVerifiedVideo(t *testing.T) {
	viewer := access.Actor{UserID: "viewer", Role: access.Viewer}
	verified := pendingAsset()
	verified.Status, verified.Generation = domain.Verified, 42
	objects := &fakeObjects{}
	link, err := New(&fakeStore{asset: &verified}, objects, settings, clock).ReadURL(ctx, viewer, "test", "rec")
	if err != nil || link.URL != "signed-read:"+verified.ObjectName+"#42" || !link.ExpiresAt.Equal(now.Add(15*time.Minute)) {
		t.Fatalf("read URL: %+v %v", link, err)
	}
	pending := pendingAsset()
	if _, err := New(&fakeStore{asset: &pending}, &fakeObjects{}, settings, clock).ReadURL(ctx, viewer, "test", "rec"); !errors.Is(err, ErrNotVerified) {
		t.Fatalf("pending video: %v", err)
	}
	if _, err := New(&fakeStore{}, &fakeObjects{}, settings, clock).ReadURL(ctx, viewer, "other-test", "rec"); !errors.Is(err, ErrRecordingNotFound) {
		t.Fatalf("recording of another test: %v", err)
	}
}

func good() *domain.ObjectAttrs {
	return &domain.ObjectAttrs{Generation: 99, Size: 1024, CRC32C: 1, ContentType: "video/mp4"}
}

func pendingAsset() domain.Asset {
	a, _, err := domain.NewUpload(request(), settings.MaxBytes, "tech")
	if err != nil {
		panic(err)
	}
	a.ID, a.Bucket, a.ObjectName = "asset", "misko-videos", "tests/test/originals/asset"
	return a
}

type fakeObjects struct {
	attrs      *domain.ObjectAttrs
	calls      []string
	signedType string
}

func (f *fakeObjects) Bucket() string { return "misko-videos" }

func (f *fakeObjects) UploadURL(_ context.Context, object, contentType string, expires time.Time) (SignedRequest, error) {
	f.calls, f.signedType = append(f.calls, "UploadURL"), contentType
	return SignedRequest{Method: "POST", URL: "signed-upload:" + object, Headers: map[string]string{"x-goog-resumable": "start"}, ExpiresAt: expires}, nil
}

func (f *fakeObjects) ReadURL(_ context.Context, object string, generation int64, expires time.Time) (string, error) {
	f.calls = append(f.calls, "ReadURL")
	return "signed-read:" + object + "#" + strings.TrimSpace(itoa(generation)), nil
}

func (f *fakeObjects) Read(context.Context, string, int64, int64) ([]byte, error) {
	f.calls = append(f.calls, "Read")
	return nil, ErrObjectNotFound
}

func (f *fakeObjects) Attrs(context.Context, string) (domain.ObjectAttrs, error) {
	f.calls = append(f.calls, "Attrs")
	if f.attrs == nil {
		return domain.ObjectAttrs{}, ErrObjectNotFound
	}
	return *f.attrs, nil
}

type fakeStore struct {
	Store
	calls       []string
	status      string
	missing     bool
	asset       *domain.Asset
	lockedAsset *domain.Asset
}

func (f *fakeStore) record(name string) { f.calls = append(f.calls, name) }

func (f *fakeStore) Transaction(_ context.Context, fn func(Store) error) error {
	f.record("Transaction")
	return fn(f)
}

func (f *fakeStore) Test(_ context.Context, id string) (TestRef, error) {
	f.record("Test")
	if f.missing {
		return TestRef{}, ErrTestNotFound
	}
	return TestRef{ID: id, ExperimentID: "exp", Status: withDefault(f.status, "IN_PROGRESS")}, nil
}

func (f *fakeStore) CreateRecording(_ context.Context, test TestRef, asset domain.Asset, clip domain.Clip) (domain.Recording, error) {
	f.record("CreateRecording")
	asset.ID, asset.ObjectName = "asset", "tests/"+test.ID+"/originals/asset"
	return domain.Recording{ID: "rec", ExperimentID: test.ExperimentID, TestID: test.ID, Clip: clip, Asset: asset, CreatedBy: asset.CreatedBy}, nil
}

func (f *fakeStore) recording(testID string) (domain.Recording, error) {
	if testID != "test" {
		return domain.Recording{}, ErrRecordingNotFound
	}
	asset := pendingAsset()
	if f.asset != nil {
		asset = *f.asset
	}
	return domain.Recording{ID: "rec", ExperimentID: "exp", TestID: testID, Asset: asset, CreatedBy: asset.CreatedBy}, nil
}

func (f *fakeStore) Recording(_ context.Context, testID, _ string) (domain.Recording, error) {
	f.record("Recording")
	return f.recording(testID)
}

func (f *fakeStore) LockRecording(_ context.Context, testID, _ string) (domain.Recording, error) {
	f.record("LockRecording")
	rec, err := f.recording(testID)
	if err == nil && f.lockedAsset != nil {
		rec.Asset = *f.lockedAsset
	}
	return rec, err
}

// A finalization that commits between the unlocked read and the row lock wins;
// the later one returns its stored result instead of verifying again.
func TestFinalizeRechecksAfterLocking(t *testing.T) {
	done := pendingAsset()
	done.Status, done.Generation = domain.Verified, 5
	verifiedAt := now.Add(-time.Second)
	done.VerifiedAt = &verifiedAt
	store := &fakeStore{lockedAsset: &done}
	rec, err := New(store, &fakeObjects{attrs: good()}, settings, clock).FinalizeUpload(ctx, access.Actor{UserID: "tech", Role: access.Technician}, "test", "rec")
	if err != nil || rec.Asset.Generation != 5 || !rec.Asset.VerifiedAt.Equal(verifiedAt) || slices.Contains(store.calls, "FinishAsset") {
		t.Fatalf("concurrent winner: %+v %v %v", rec.Asset, err, store.calls)
	}
}

func (f *fakeStore) FinishAsset(_ context.Context, a domain.Asset) (domain.Asset, error) {
	f.record("FinishAsset")
	return a, nil
}

func withDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

func second[T any](_ T, err error) error { return err }
