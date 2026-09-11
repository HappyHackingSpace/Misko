package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)

func queued() Run {
	end := int64(10_000_000)
	return Run{ID: "run-1", TestID: "test", RecordingID: "rec", ClipStartUs: 2_000_000, ClipEndUs: &end,
		ParadigmKey: "OPEN_FIELD", ParadigmVersion: 1, Status: Queued, MaxAttempts: 3, AvailableAt: now.Add(-time.Second)}
}

func TestLeasesFenceLateAttempts(t *testing.T) {
	run, err := queued().Claim("worker-a", now, time.Minute)
	if err != nil || run.Status != Running || run.Attempt != 1 || run.WorkerID != "worker-a" || !run.LeaseExpiresAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("claim: %+v %v", run, err)
	}
	if _, err := run.Claim("worker-b", now, time.Minute); !errors.Is(err, ErrNotClaimable) {
		t.Fatalf("claim a running run: %v", err)
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

	// The lease expires: the run is queued again and the old attempt is fenced off.
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

	// After the last attempt a lost lease fails the run.
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

func contract() Contract {
	return Contract{
		Metrics: []MetricSpec{
			{Key: "distance_cm", Unit: "cm", Min: 0, Max: 1e6},
			{Key: "center_entries_count", Unit: "count", Integer: true, Min: 0, Max: 1e6},
			{Key: "center_time_ratio", Unit: "ratio", Min: 0, Max: 1},
		},
		Events:         []EventSpec{{Type: "in_center", Kind: "INTERVAL"}, {Type: "immobile", Kind: "INTERVAL"}, {Type: "center_entry", Kind: "POINT"}},
		MissingReasons: []string{"NO_VALID_INTERVALS", "ZERO_DENOMINATOR"},
	}
}

func value(v float64) *float64 { return &v }

func submission(run Run) Submission {
	prefix := run.OutputPrefix()
	return Submission{
		WorkerID: run.WorkerID, Attempt: run.Attempt, ModelVersion: "open-field-tracker 1.4.0",
		Metrics: []MetricInput{
			{Key: "distance_cm", Value: value(412.5)},
			{Key: "center_entries_count", Value: value(3)},
			{Key: "center_time_ratio", MissingReason: "ZERO_DENOMINATOR"},
		},
		Events: []EventInput{
			{Type: "in_center", StartUs: 1_000_000, EndUs: 3_000_000, Confidence: 0.9, TrialID: "trial-1"},
			// A different type may overlap in_center.
			{Type: "immobile", StartUs: 2_000_000, EndUs: 2_500_000, Confidence: 0.8},
			{Type: "in_center", StartUs: 5_000_000, EndUs: 8_000_000, Confidence: 0.95},
			{Type: "center_entry", StartUs: 1_000_000, EndUs: 1_000_000, Confidence: 1},
		},
		Artifacts: []ArtifactInput{
			{Kind: AnalyzedVideo, ObjectName: prefix + "overlay.mp4"},
			{Kind: Trajectory, ObjectName: prefix + "trajectory.parquet"},
		},
		Pair: PairInput{AnalyzedObjectName: prefix + "overlay.mp4", SourceOffsetUs: run.ClipStartUs, OutputOffsetUs: 0, TimeMappingVersion: "identity-v1"},
	}
}

func TestResultsAreValidatedAgainstTheContract(t *testing.T) {
	run, _ := queued().Claim("worker-a", now, time.Minute)
	result, err := Validate(run, contract(), submission(run), []string{"trial-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Metrics) != 3 || result.Metrics[2].Value != nil || result.Metrics[2].MissingReason != "ZERO_DENOMINATOR" || result.Metrics[0].Unit != "cm" {
		t.Fatalf("metrics: %+v", result.Metrics)
	}
	if len(result.Events) != 4 || result.Events[0].StartUs != 1_000_000 || result.Events[3].Kind != "INTERVAL" || result.DurationUs != 8_000_000 {
		t.Fatalf("events sorted by start and typed: %+v", result)
	}

	with := func(mutate func(*Submission)) Submission { s := submission(run); mutate(&s); return s }
	for name, tc := range map[string]struct {
		s    Submission
		want error
	}{
		"metric missing":          {with(func(s *Submission) { s.Metrics = s.Metrics[:2] }), ErrMissingMetric},
		"unknown metric":          {with(func(s *Submission) { s.Metrics = append(s.Metrics, MetricInput{Key: "speed", Value: value(1)}) }), ErrUnknownMetric},
		"metric twice":            {with(func(s *Submission) { s.Metrics[2] = s.Metrics[0] }), ErrInvalidMetric},
		"value above maximum":     {with(func(s *Submission) { s.Metrics[2] = MetricInput{Key: "center_time_ratio", Value: value(1.2)} }), ErrInvalidMetric},
		"fractional count":        {with(func(s *Submission) { s.Metrics[1].Value = value(2.5) }), ErrInvalidMetric},
		"missing without reason":  {with(func(s *Submission) { s.Metrics[2].MissingReason = "" }), ErrInvalidMetric},
		"zero instead of reason":  {with(func(s *Submission) { s.Metrics[2].MissingReason = "NOT_A_REASON" }), ErrInvalidMetric},
		"value and reason":        {with(func(s *Submission) { s.Metrics[0].MissingReason = "ZERO_DENOMINATOR" }), ErrInvalidMetric},
		"negative start":          {with(func(s *Submission) { s.Events[0].StartUs = -1 }), ErrEventOutOfRange},
		"reversed interval":       {with(func(s *Submission) { s.Events[0].EndUs = 500_000 }), ErrEventOutOfRange},
		"empty interval":          {with(func(s *Submission) { s.Events[0].EndUs = s.Events[0].StartUs }), ErrEventOutOfRange},
		"past the recording":      {with(func(s *Submission) { s.Events[2].EndUs = 8_000_001 }), ErrEventOutOfRange},
		"point with duration":     {with(func(s *Submission) { s.Events[3].EndUs = 1_000_001 }), ErrEventOutOfRange},
		"unknown event type":      {with(func(s *Submission) { s.Events[0].Type = "grooming" }), ErrInvalidEvent},
		"confidence above one":    {with(func(s *Submission) { s.Events[0].Confidence = 1.5 }), ErrInvalidEvent},
		"trial of another test":   {with(func(s *Submission) { s.Events[0].TrialID = "trial-9" }), ErrUnknownTrial},
		"same type overlaps":      {with(func(s *Submission) { s.Events[2].StartUs = 2_500_000 }), ErrOverlappingEvents},
		"no analyzed video":       {with(func(s *Submission) { s.Artifacts = s.Artifacts[1:] }), ErrMissingAnalyzedVideo},
		"artifact of another run": {with(func(s *Submission) { s.Artifacts[1].ObjectName = "runs/run-2/attempts/1/trajectory.parquet" }), ErrInvalidArtifact},
		"artifact of an old attempt": {with(func(s *Submission) {
			s.Artifacts[1].ObjectName = strings.Replace(s.Artifacts[1].ObjectName, "/attempts/1/", "/attempts/0/", 1)
		}), ErrInvalidArtifact},
		"pair names another object":        {with(func(s *Submission) { s.Pair.AnalyzedObjectName = run.OutputPrefix() + "trajectory.parquet" }), ErrInvalidPair},
		"pair ignores the clip start":      {with(func(s *Submission) { s.Pair.SourceOffsetUs = 0 }), ErrInvalidPair},
		"duration disagrees with the clip": {with(func(s *Submission) { s.RecordingDurationUs = 9_000_000 }), ErrInvalidDuration},
		"no model version":                 {with(func(s *Submission) { s.ModelVersion = "" }), ErrInvalidModelVersion},
	} {
		if _, err := Validate(run, contract(), tc.s, []string{"trial-1"}); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}

	// A clip without an end takes the duration the worker measured.
	open := queued()
	open.ClipEndUs = nil
	openRun, _ := open.Claim("worker-a", now, time.Minute)
	s := submission(openRun)
	if _, err := Validate(openRun, contract(), s, []string{"trial-1"}); !errors.Is(err, ErrInvalidDuration) {
		t.Fatalf("open clip without a measured duration: %v", err)
	}
	s.RecordingDurationUs = 8_000_000
	if r, err := Validate(openRun, contract(), s, []string{"trial-1"}); err != nil || r.DurationUs != 8_000_000 {
		t.Fatalf("open clip: %+v %v", r, err)
	}
}

func TestWorkersDeclareCapabilities(t *testing.T) {
	w, err := NewWorker(" tracker-1 ", "open-field-tracker 1.4.0", []Capability{{ParadigmKey: "OPEN_FIELD", ParadigmVersion: 1}})
	if err != nil || w.Name != "tracker-1" || !w.Can("OPEN_FIELD", 1) || w.Can("OPEN_FIELD", 2) || w.Can("EPM", 1) {
		t.Fatalf("worker: %+v %v", w, err)
	}
	for name, tc := range map[string]struct {
		name, model string
		caps        []Capability
		want        error
	}{
		"no name":         {"", "m", []Capability{{ParadigmKey: "EPM", ParadigmVersion: 1}}, ErrInvalidWorkerName},
		"no model":        {"w", " ", []Capability{{ParadigmKey: "EPM", ParadigmVersion: 1}}, ErrInvalidModelVersion},
		"no capabilities": {"w", "m", nil, ErrInvalidCapability},
		"duplicate":       {"w", "m", []Capability{{ParadigmKey: "EPM", ParadigmVersion: 1}, {ParadigmKey: "EPM", ParadigmVersion: 1}}, ErrInvalidCapability},
		"version zero":    {"w", "m", []Capability{{ParadigmKey: "EPM", ParadigmVersion: 0}}, ErrInvalidCapability},
	} {
		if _, err := NewWorker(tc.name, tc.model, tc.caps); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}
}
