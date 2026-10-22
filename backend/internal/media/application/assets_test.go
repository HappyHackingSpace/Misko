package application

import (
	"context"
	"errors"
	"testing"

	"github.com/HappyHackingSpace/Misko/backend/internal/media/domain"
)

// assetStore keeps one asset for the asset port.
type assetStore struct {
	fakeStore
	stored *domain.Asset
	folder string
}

func (a *assetStore) Transaction(_ context.Context, fn func(Store) error) error { return fn(a) }

func (a *assetStore) CreateFolderAsset(_ context.Context, folder string, asset domain.Asset) (domain.Asset, error) {
	a.folder = folder
	asset.ID, asset.ObjectName = "asset", folder+"/originals/asset"
	a.stored = &asset
	return asset, nil
}

func (a *assetStore) Asset(context.Context, string) (domain.Asset, error) {
	if a.stored == nil {
		return domain.Asset{}, ErrAssetNotFound
	}
	return *a.stored, nil
}

func (a *assetStore) LockAsset(c context.Context, id string) (domain.Asset, error) {
	return a.Asset(c, id)
}

func (a *assetStore) FinishAsset(_ context.Context, asset domain.Asset) (domain.Asset, error) {
	a.stored = &asset
	return asset, nil
}

func TestAssetsOutsideTestsAreUploadedAndVerified(t *testing.T) {
	const folder = "trials/01a00000-0000-7000-8000-0000000000c1"
	store := &assetStore{}
	objects := &fakeObjects{}
	s := New(store, objects, settings, clock)
	for name, tc := range map[string]struct {
		folder string
		req    domain.UploadRequest
		want   error
	}{
		"folder outside a module": {"../etc", request(), domain.ErrInvalidFolder},
		"folder without an id":    {"trials/x", request(), domain.ErrInvalidFolder},
		"bad content type":        {folder, domain.UploadRequest{FileName: "a.avi", ContentType: "video/avi", SizeBytes: 1, CRC32C: "AAAAAQ=="}, domain.ErrInvalidContentType},
		"bad clip":                {folder, domain.UploadRequest{FileName: "a.mp4", ContentType: "video/mp4", SizeBytes: 1, CRC32C: "AAAAAQ==", ClipStartUs: -1}, domain.ErrInvalidClip},
	} {
		if _, _, err := s.StartAssetUpload(ctx, tc.folder, tc.req, "tech"); !errors.Is(err, tc.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
	asset, signed, err := s.StartAssetUpload(ctx, folder, request(), "tech")
	if err != nil || asset.Status != domain.Pending || asset.ObjectName != folder+"/originals/asset" || signed.URL != "signed-upload:"+asset.ObjectName ||
		objects.signedSize != 1024 || store.folder != folder {
		t.Fatalf("start: %+v %+v %v", asset, signed, err)
	}
	if _, err := s.FinalizeAsset(ctx, asset.ID); !errors.Is(err, ErrUploadIncomplete) {
		t.Fatalf("finalize before the upload: %v", err)
	}
	if _, err := s.AssetReadURL(ctx, asset.ID); !errors.Is(err, ErrNotVerified) {
		t.Fatalf("read a pending asset: %v", err)
	}
	objects.attrs = good()
	verified, err := s.FinalizeAsset(ctx, asset.ID)
	if err != nil || verified.Status != domain.Verified || verified.Generation != good().Generation {
		t.Fatalf("finalize: %+v %v", verified, err)
	}
	if again, err := s.FinalizeAsset(ctx, asset.ID); err != nil || again.Status != domain.Verified {
		t.Fatalf("finalize again: %+v %v", again, err)
	}
	if link, err := s.AssetReadURL(ctx, asset.ID); err != nil || link.URL == "" {
		t.Fatalf("read: %+v %v", link, err)
	}
	if _, _, err := New(store, nil, settings, clock).StartAssetUpload(ctx, folder, request(), "tech"); !errors.Is(err, ErrStorageNotConfigured) {
		t.Fatalf("no storage: %v", err)
	}
}
