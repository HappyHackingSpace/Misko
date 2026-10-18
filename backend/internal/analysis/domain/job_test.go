package domain

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)

func queued() Job {
	end := int64(10_000_000)
	return Job{ID: "job-1", Input: Input{Requester: Requester{Kind: RequesterTrial, ID: "trial-1"}, ParadigmKey: "MWM", ParadigmVersion: 1,
		Source: Source{AssetID: "asset", Generation: 7, ClipStartUs: 2_000_000, ClipEndUs: &end}},
		Status: Queued, MaxAttempts: 3, AvailableAt: now.Add(-time.Second)}
}

func TestLeasesFenceLateAttempts(t *testing.T) {
	run, err := queued().Claim("worker-a", now, time.Minute)
	if err != nil || run.Status != Running || run.Attempt != 1 || run.WorkerID != "worker-a" || !run.LeaseExpiresAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("claim: %+v %v", run, err)
	}
	if _, err := run.Claim("worker-b", now, time.Minute); !errors.Is(err, ErrNotClaimable) {
		t.Fatalf("claim a running job: %v", err)
	}
	future := queued()
	future.AvailableAt = now.Add(time.Minute)
	if _, err := future.Claim("worker-a", now, time.Minute); !errors.Is(err, ErrNotClaimable) {
		t.Fatalf("claim before it is available: %v", err)
	}
	if err := run.CheckLease("worker-a", 1); err != nil {
		t.Fatalf("current lease: %v", err)
	}
	for name, err := range map[string]error{
		"other worker":  run.CheckLease("worker-b", 1),
		"other attempt": run.CheckLease("worker-a", 2),
	} {
		if !errors.Is(err, ErrStaleAttempt) {
			t.Errorf("%s: %v", name, err)
		}
	}
	beat, err := run.Heartbeat("worker-a", 1, now.Add(30*time.Second), time.Minute)
	if err != nil || !beat.LeaseExpiresAt.Equal(now.Add(90*time.Second)) {
		t.Fatalf("heartbeat: %+v %v", beat, err)
	}

	// The lease expires: the job is queued again and the old attempt is fenced off.
	if _, expired := beat.Expire(now.Add(time.Minute)); expired {
		t.Fatal("expired before the lease ended")
	}
	requeued, expired := beat.Expire(now.Add(2 * time.Minute))
	if !expired || requeued.Status != Queued || requeued.WorkerID != "" || requeued.LeaseExpiresAt != nil || requeued.Attempt != 1 {
		t.Fatalf("expire: %+v %v", requeued, expired)
	}
	second, err := requeued.Claim("worker-b", now.Add(2*time.Minute), time.Minute)
	if err != nil || second.Attempt != 2 {
		t.Fatalf("reclaim: %+v %v", second, err)
	}
	if err := second.CheckLease("worker-a", 1); !errors.Is(err, ErrStaleAttempt) {
		t.Fatalf("late attempt: %v", err)
	}

	// After the last attempt a lost lease fails the job.
	last := second
	last.Attempt = 3
	failed, expired := last.Expire(now.Add(time.Hour))
	if !expired || failed.Status != Failed || failed.FailureReason != ReasonLeaseExpired || failed.FinishedAt == nil {
		t.Fatalf("attempts exhausted: %+v", failed)
	}
}

func TestWorkerFailuresRetryWithinTheLimit(t *testing.T) {
	run, _ := queued().Claim("worker-a", now, time.Minute)
	retry, err := run.Fail("worker-a", 1, "decoder crashed", true, now)
	if err != nil || retry.Status != Queued || retry.WorkerID != "" || retry.FailureReason != "decoder crashed" {
		t.Fatalf("retryable failure: %+v %v", retry, err)
	}
	final, err := run.Fail("worker-a", 1, "unsupported codec", false, now)
	if err != nil || final.Status != Failed || final.FinishedAt == nil {
		t.Fatalf("permanent failure: %+v %v", final, err)
	}
	last := run
	last.Attempt = 3
	if exhausted, _ := last.Fail("worker-a", 3, "decoder crashed", true, now); exhausted.Status != Failed {
		t.Fatalf("retry beyond the limit: %+v", exhausted)
	}
	if _, err := run.Fail("worker-b", 1, "x", true, now); !errors.Is(err, ErrStaleAttempt) {
		t.Fatalf("failure from another worker: %v", err)
	}
	if _, err := run.Fail("worker-a", 1, strings.Repeat("r", 2001), true, now); !errors.Is(err, ErrInvalidReason) {
		t.Fatalf("long reason: %v", err)
	}
}

func TestWorkersDeclareCapabilities(t *testing.T) {
	w, err := NewWorker(" tracker-1 ", "mwm-tracker 1.4.0", []Capability{{ParadigmKey: "MWM", ParadigmVersion: 1}})
	if err != nil || w.Name != "tracker-1" || !w.Can("MWM", 1) || w.Can("MWM", 2) || w.Can("POLE", 1) {
		t.Fatalf("worker: %+v %v", w, err)
	}
	for name, tc := range map[string]struct {
		name, model string
		caps        []Capability
		want        error
	}{
		"no name":         {"", "m", []Capability{{ParadigmKey: "POLE", ParadigmVersion: 1}}, ErrInvalidWorkerName},
		"no model":        {"w", " ", []Capability{{ParadigmKey: "POLE", ParadigmVersion: 1}}, ErrInvalidModelVersion},
		"no capabilities": {"w", "m", nil, ErrInvalidCapability},
		"duplicate":       {"w", "m", []Capability{{ParadigmKey: "POLE", ParadigmVersion: 1}, {ParadigmKey: "POLE", ParadigmVersion: 1}}, ErrInvalidCapability},
		"version zero":    {"w", "m", []Capability{{ParadigmKey: "POLE", ParadigmVersion: 0}}, ErrInvalidCapability},
	} {
		if _, err := NewWorker(tc.name, tc.model, tc.caps); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}
}

func request() Request {
	end := int64(6_000_000)
	return Request{Requester: Requester{Kind: RequesterTestSubject, ID: "subject-1"}, ParadigmKey: "NOVEL_OBJECT", ParadigmVersion: 1,
		Parameters: map[string]float64{"arena_width_cm": 40}, SourceAssetID: "asset-1", ClipEndUs: &end, CalibrationID: "cal-1",
		Trigger: Automatic, IdempotencyKey: "key-1", CreatedBy: "user-1"}
}

func zonedCalibration() *Calibration {
	return &Calibration{ID: "cal-1", FrameWidth: 640, FrameHeight: 480, Crop: Rect{X: 10, Y: 10, Width: 400, Height: 400}, Plane: "ARENA_FLOOR",
		Transform: [9]float64{1, 0, 0, 0, 1, 0, 0, 0, 1},
		Zones:     []Zone{{Key: "novel_object", Pixels: Shape{Circle: &Circle{X: 50, Y: 50, Radius: 5}}}, {Key: "familiar_object"}}}
}

func TestNewJobSnapshotsTheRequest(t *testing.T) {
	contract := Contract{MetricEngineVersion: 4, ResultSchemaVersion: 2, CalibrationRequired: true, TrajectoryAnalyzable: true,
		RequiredZones: []string{"novel_object", "familiar_object"}}
	video := SourceVideo{ID: "asset-1", Verified: true, Generation: 42, CRC32C: 9}
	calibration := zonedCalibration()
	job, err := NewJob(request(), contract, video, calibration, now, 3)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != Queued || job.MaxAttempts != 3 || !job.AvailableAt.Equal(now) || job.MetricEngineVersion != 4 || job.Source.Generation != 42 ||
		job.Source.CRC32C != 9 || job.CalibrationID() != "cal-1" || len(job.Calibration.Zones) != 2 || job.Requester.ID != "subject-1" {
		t.Fatalf("job: %+v", job)
	}
	calibration.Zones[0].Key = "changed"
	if job.Calibration.Zones[0].Key != "novel_object" {
		t.Fatal("the snapshot shares the caller's zones")
	}
	untracked := contract
	untracked.CalibrationRequired = false
	if free, err := NewJob(request(), untracked, video, zonedCalibration(), now, 3); err != nil || free.Calibration != nil {
		t.Fatalf("a paradigm without calibration keeps none: %+v %v", free.Calibration, err)
	}

	with := func(mutate func(*Request)) Request { r := request(); mutate(&r); return r }
	badCrop := zonedCalibration()
	badCrop.Crop.Width = 700
	oneZone := zonedCalibration()
	oneZone.Zones = oneZone.Zones[:1]
	for name, tc := range map[string]struct {
		req         Request
		contract    Contract
		video       SourceVideo
		calibration *Calibration
		want        error
	}{
		"unknown requester":    {with(func(r *Request) { r.Requester.Kind = "EXPERIMENT" }), contract, video, calibration, ErrInvalidRequest},
		"no requester id":      {with(func(r *Request) { r.Requester.ID = " " }), contract, video, calibration, ErrInvalidRequest},
		"bad paradigm":         {with(func(r *Request) { r.ParadigmKey = "mwm" }), contract, video, calibration, ErrInvalidRequest},
		"no source":            {with(func(r *Request) { r.SourceAssetID = "" }), contract, video, calibration, ErrInvalidRequest},
		"empty clip":           {with(func(r *Request) { end := int64(0); r.ClipEndUs = &end }), contract, video, calibration, ErrInvalidRequest},
		"unknown trigger":      {with(func(r *Request) { r.Trigger = "LATER" }), contract, video, calibration, ErrInvalidRequest},
		"long key":             {with(func(r *Request) { r.IdempotencyKey = strings.Repeat("k", 201) }), contract, video, calibration, ErrInvalidRequest},
		"infinite parameter":   {with(func(r *Request) { r.Parameters = map[string]float64{"x": math.Inf(1)} }), contract, video, calibration, ErrInvalidRequest},
		"not analysable":       {request(), Contract{}, video, calibration, ErrNotAnalysable},
		"unverified video":     {request(), contract, SourceVideo{ID: "asset-1", Generation: 42}, calibration, ErrSourceNotVerified},
		"another video":        {request(), contract, SourceVideo{ID: "asset-2", Verified: true, Generation: 42}, calibration, ErrSourceNotVerified},
		"no calibration":       {request(), contract, video, nil, ErrCalibrationRequired},
		"crop outside frame":   {request(), contract, video, badCrop, ErrInvalidCalibration},
		"missing a zone shape": {request(), contract, video, oneZone, ErrMissingZones},
	} {
		if _, err := NewJob(tc.req, tc.contract, tc.video, tc.calibration, now, 3); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}
}
