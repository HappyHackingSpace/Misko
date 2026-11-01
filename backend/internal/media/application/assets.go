package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/HappyHackingSpace/Misko/backend/internal/media/domain"
)

// ErrAssetNotFound means no video asset has the id.
var ErrAssetNotFound = errors.New("video asset not found")

// The asset port. Other modules use it in process: the analysis service reads
// source videos, and modules that keep videos outside any test (paradigm
// trials) upload and verify them. Callers authorize their own users.

// Asset returns one video asset, or ErrAssetNotFound.
func (s *Service) Asset(ctx context.Context, id string) (domain.Asset, error) {
	return s.store.Asset(ctx, id)
}

// StartAssetUpload records a pending original under folder and signs the
// resumable upload the caller sends straight to storage, limited to the
// declared size. The request's clip is validated but belongs to the caller.
func (s *Service) StartAssetUpload(ctx context.Context, folder string, req domain.UploadRequest, createdBy string) (domain.Asset, SignedRequest, error) {
	if s.objects == nil {
		return domain.Asset{}, SignedRequest{}, ErrStorageNotConfigured
	}
	if err := domain.CheckFolder(folder); err != nil {
		return domain.Asset{}, SignedRequest{}, err
	}
	asset, _, err := domain.NewUpload(req, s.settings.MaxBytes, createdBy)
	if err != nil {
		return domain.Asset{}, SignedRequest{}, err
	}
	asset.Bucket = s.objects.Bucket()
	var signed SignedRequest
	err = s.store.Transaction(ctx, func(tx Store) error {
		created, err := tx.CreateFolderAsset(ctx, folder, asset)
		if err != nil {
			return err
		}
		if signed, err = s.objects.SizedUploadURL(ctx, created.ObjectName, created.ContentType, created.SizeBytes, s.now().Add(s.settings.UploadTTL)); err != nil {
			return fmt.Errorf("sign upload: %w", err)
		}
		asset = created
		return nil
	})
	if err != nil {
		return domain.Asset{}, SignedRequest{}, err
	}
	return asset, signed, nil
}

// FinalizeAsset reads the stored object and records whether it matches the
// upload. Finalizing again returns the stored result.
func (s *Service) FinalizeAsset(ctx context.Context, id string) (domain.Asset, error) {
	if s.objects == nil {
		return domain.Asset{}, ErrStorageNotConfigured
	}
	asset, err := s.store.Asset(ctx, id)
	if err != nil {
		return domain.Asset{}, err
	}
	if asset.Status != domain.Pending {
		return asset, nil
	}
	attrs, err := s.objects.Attrs(ctx, asset.ObjectName)
	if errors.Is(err, ErrObjectNotFound) {
		return domain.Asset{}, ErrUploadIncomplete
	}
	if err != nil {
		return domain.Asset{}, fmt.Errorf("read object attributes: %w", err)
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	err = s.store.Transaction(ctx, func(tx Store) error {
		locked, err := tx.LockAsset(ctx, id)
		if err != nil {
			return err
		}
		if locked.Status != domain.Pending {
			asset = locked
			return nil
		}
		checked, err := locked.Verify(attrs, s.settings.MaxBytes, now)
		if err != nil {
			return err
		}
		asset, err = tx.FinishAsset(ctx, checked)
		return err
	})
	if err != nil {
		return domain.Asset{}, err
	}
	return asset, nil
}

// AssetReadURL signs a short-lived read of a verified asset's generation.
func (s *Service) AssetReadURL(ctx context.Context, id string) (ReadLink, error) {
	if s.objects == nil {
		return ReadLink{}, ErrStorageNotConfigured
	}
	asset, err := s.store.Asset(ctx, id)
	if err != nil {
		return ReadLink{}, err
	}
	if asset.Status != domain.Verified {
		return ReadLink{}, ErrNotVerified
	}
	expires := s.now().Add(s.settings.ReadTTL)
	url, err := s.objects.ReadURL(ctx, asset.ObjectName, asset.Generation, expires)
	if err != nil {
		return ReadLink{}, fmt.Errorf("sign read: %w", err)
	}
	return ReadLink{URL: url, ExpiresAt: expires}, nil
}
