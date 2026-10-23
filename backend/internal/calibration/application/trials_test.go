package application

import (
	"context"
	"errors"
	"slices"
	"testing"

	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/calibration/domain"
)

// trialStore adds one paradigm trial to the calibration fake.
type trialStore struct {
	fakeStore
	trial      TrialRef
	missing    bool
	trialChain []domain.Calibration
	effective  TrialEffective
}

func (f *trialStore) Transaction(_ context.Context, fn func(Store) error) error { return fn(f) }

func (f *trialStore) Trial(context.Context, string) (TrialRef, error) {
	if f.missing {
		return TrialRef{}, ErrTrialNotFound
	}
	return f.trial, nil
}

func (f *trialStore) LockTrial(c context.Context, id string) (TrialRef, error) { return f.Trial(c, id) }

func (f *trialStore) TrialCalibrations(context.Context, string) ([]domain.Calibration, error) {
	return f.trialChain, nil
}

func (f *trialStore) TrialEffective(context.Context, string) (TrialEffective, error) {
	return f.effective, nil
}

func newTrialStore() *trialStore {
	return &trialStore{trial: TrialRef{ID: "trial", EnvironmentRevisionID: "rev", ParadigmKey: "MWM", ParadigmVersion: 1, VideoAssetID: "video", VideoStatus: VideoVerified}}
}

func TestTrialCalibrationsBelongToTheTrialVideo(t *testing.T) {
	technician := access.Actor{UserID: "tech", Role: access.Technician}
	viewer := access.Actor{UserID: "v", Role: access.Viewer}
	if _, err := New(newTrialStore(), fakeCatalog{}, clock).CalibrateTrial(ctx, viewer, "trial", input()); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("viewer: %v", err)
	}
	store := newTrialStore()
	in := input()
	in.ReferenceFrameUs = 250_000
	c, err := New(store, fakeCatalog{}, clock).CalibrateTrial(ctx, technician, "trial", in)
	if err != nil || c.Scope() != domain.ScopeTrial || c.TrialID != "trial" || c.VideoAssetID != "video" || c.EnvironmentRevisionID != "rev" ||
		c.TestSubjectID != "" || c.ReferenceFrameUs != 250_000 || !slices.Contains(store.calls, "CreateCalibration") {
		t.Fatalf("trial calibration: %+v %v", c, err)
	}
	previous, _ := domain.New(input(), domain.Requirement{Required: true, ToleranceCm: 2}, "tech", now)
	previous.ID, previous.TrialID, previous.VideoAssetID = "t1", "trial", "video"
	for name, tc := range map[string]struct {
		mutate func(*trialStore)
		want   error
	}{
		"unknown trial":      {func(s *trialStore) { s.missing = true }, ErrTrialNotFound},
		"no video":           {func(s *trialStore) { s.trial.VideoAssetID = "" }, ErrTrialNoVideo},
		"video not verified": {func(s *trialStore) { s.trial.VideoStatus = "PENDING" }, ErrVideoNotVerified},
		"stale correction":   {func(s *trialStore) { s.trialChain = []domain.Calibration{previous} }, domain.ErrStaleCorrection},
	} {
		s := newTrialStore()
		tc.mutate(s)
		if _, err := New(s, fakeCatalog{}, clock).CalibrateTrial(ctx, technician, "trial", input()); !errors.Is(err, tc.want) || slices.Contains(s.calls, "CreateCalibration") {
			t.Errorf("%s: %v", name, err)
		}
	}
	corrected := input()
	corrected.SupersedesID = "t1"
	s := newTrialStore()
	s.trialChain = []domain.Calibration{previous}
	if c, err := New(s, fakeCatalog{}, clock).CalibrateTrial(ctx, technician, "trial", corrected); err != nil || c.SupersedesID != "t1" {
		t.Fatalf("correction: %+v %v", c, err)
	}
}

func TestTrialStatusPrefersTheTrialsOwnCalibration(t *testing.T) {
	viewer := access.Actor{UserID: "v", Role: access.Viewer}
	valid, _ := domain.New(input(), domain.Requirement{Required: true, ToleranceCm: 2}, "tech", now)
	valid.ID, valid.TrialID = "own", "trial"
	for name, tc := range map[string]struct {
		mutate func(*trialStore)
		status CalibrationStatus
		source domain.Source
	}{
		"waiting for video":        {func(s *trialStore) { s.trial.VideoStatus = "PENDING" }, WaitingForVideo, ""},
		"nothing calibrated":       {func(*trialStore) {}, Waiting, ""},
		"own calibration":          {func(s *trialStore) { s.effective = TrialEffective{Source: domain.SourceTrial, CalibrationID: "own"} }, Calibrated, domain.SourceTrial},
		"own calibration rejected": {func(s *trialStore) { s.effective = TrialEffective{Source: domain.SourceTrial} }, Waiting, domain.SourceTrial},
	} {
		s := newTrialStore()
		s.stored = valid
		tc.mutate(s)
		view, err := New(s, fakeCatalog{}, clock).TrialStatus(ctx, viewer, "trial")
		if err != nil || view.Status != tc.status || view.Source != tc.source {
			t.Errorf("%s: %+v %v", name, view, err)
		}
	}
	s := newTrialStore()
	s.effective = TrialEffective{Source: domain.SourceEnvironment, CalibrationID: "env"}
	if id, err := New(s, fakeCatalog{}, clock).TrialCalibration(ctx, "trial"); err != nil || id != "env" {
		t.Fatalf("port: %q %v", id, err)
	}
}
