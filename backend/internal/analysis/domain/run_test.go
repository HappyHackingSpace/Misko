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
