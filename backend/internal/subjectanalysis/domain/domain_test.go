package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var at = time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)

func run(jobID, subject string, status JobStatus, finished *time.Time) Run {
	return Run{Link: Link{JobID: jobID, TestSubjectID: subject}, Job: Job{ID: jobID, Status: status, FinishedAt: finished}}
}

func TestReviewDecisions(t *testing.T) {
	open := OpenResult("ts-1", "job-1")
	if open.Status != ResultPending || open.JobID != "job-1" || open.TestSubjectID != "ts-1" || open.ReviewedBy != "" {
		t.Fatalf("open: %+v", open)
	}
	accepted, err := open.Accept("reviewer", "  clean track ", at)
	if err != nil || accepted.Status != ResultAccepted || accepted.Note != "clean track" || accepted.ReviewedBy != "reviewer" || !accepted.ReviewedAt.Equal(at) {
		t.Fatalf("accept: %+v %v", accepted, err)
	}
	rejected, err := accepted.Reject("second", "TRACKING_LOST", "", at.Add(time.Hour))
	if err != nil || rejected.Status != ResultRejected || rejected.Reason != "TRACKING_LOST" || rejected.Note != "" || rejected.ReviewedBy != "second" {
		t.Fatalf("revise to reject: %+v %v", rejected, err)
	}
	again, err := rejected.Accept("third", "", at)
	if err != nil || again.Reason != "" || again.Status != ResultAccepted {
		t.Fatalf("accepting clears the reason: %+v %v", again, err)
	}
	for _, reason := range []string{"", "tracking_lost", "BORED"} {
		if _, err := open.Reject("r", reason, "", at); !errors.Is(err, ErrInvalidRejection) {
			t.Errorf("reason %q: %v", reason, err)
		}
	}
	if _, err := open.Accept("r", strings.Repeat("x", 2001), at); !errors.Is(err, ErrInvalidNote) {
		t.Errorf("long note: %v", err)
	}
	if _, err := open.Reject("r", "OTHER", "\xff", at); !errors.Is(err, ErrInvalidNote) {
		t.Errorf("invalid UTF-8 note: %v", err)
	}

	chosen, err := again.Choose(run("job-2", "ts-1", JobSucceeded, &at))
	if err != nil || chosen.JobID != "job-2" || chosen.Status != ResultPending || chosen.ReviewedBy != "" || chosen.ReviewedAt != nil || chosen.Note != "" {
		t.Fatalf("choose: %+v %v", chosen, err)
	}
	for name, other := range map[string]Run{
		"another animal": run("job-3", "ts-2", JobSucceeded, &at),
		"failed job":     run("job-4", "ts-1", JobFailed, &at),
		"queued job":     run("job-5", "ts-1", JobQueued, nil),
	} {
		if _, err := again.Choose(other); !errors.Is(err, ErrRunNotChosen) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestLatestAndPendingRuns(t *testing.T) {
	later := at.Add(time.Minute)
	runs := []Run{run("a", "ts", JobSucceeded, &at), run("b", "ts", JobSucceeded, &later), run("c", "ts", JobFailed, &later), run("d", "ts", JobQueued, nil)}
	if Latest(runs) != "b" || !AnyPending(runs) || AnyPending(runs[:3]) || Latest(runs[2:]) != "" {
		t.Fatalf("latest %q pending %v", Latest(runs), AnyPending(runs))
	}
}

func ready() Candidate {
	return Candidate{ExperimentID: "e", TestID: "t", TestSubjectID: "ts", RecordingID: "rec", SourceAssetID: "asset", CalibrationID: "cal",
		ZoneKeys: []string{"novel_object", "familiar_object"}, ParadigmKey: "NOVEL_OBJECT", ParadigmVersion: 1, VideoVerified: true, ExperimentStatus: "RUNNING"}
}

func TestCandidatesAreReadyWithWhatTheirParadigmRequires(t *testing.T) {
	zoned := Requirement{CalibrationRequired: true, RequiredZones: []string{"novel_object", "familiar_object"}}
	for name, tc := range map[string]struct {
		mutate func(*Candidate)
		req    Requirement
		want   bool
	}{
		"ready":                 {func(*Candidate) {}, zoned, true},
		"completed experiment":  {func(c *Candidate) { c.ExperimentStatus = "COMPLETED" }, zoned, true},
		"no video":              {func(c *Candidate) { c.RecordingID = "" }, zoned, false},
		"video not verified":    {func(c *Candidate) { c.VideoVerified = false }, zoned, false},
		"test cancelled":        {func(c *Candidate) { c.TestCancelled = true }, zoned, false},
		"excluded animal":       {func(c *Candidate) { c.Excluded = true }, zoned, false},
		"draft experiment":      {func(c *Candidate) { c.ExperimentStatus = "DRAFT" }, zoned, false},
		"unreadable inputs":     {func(c *Candidate) { c.Invalid = errors.New("bad parameters") }, zoned, false},
		"no calibration":        {func(c *Candidate) { c.CalibrationID = "" }, zoned, false},
		"a zone missing":        {func(c *Candidate) { c.ZoneKeys = c.ZoneKeys[:1] }, zoned, false},
		"calibration not asked": {func(c *Candidate) { c.CalibrationID = "" }, Requirement{}, true},
	} {
		c := ready()
		tc.mutate(&c)
		if got := c.Ready(tc.req); got != tc.want {
			t.Errorf("%s: ready=%v want %v", name, got, tc.want)
		}
	}
	c := ready()
	automatic := c.Request(zoned, Automatic, "")
	if automatic.IdempotencyKey != "test-subject:ts:recording:rec:calibration:cal" || automatic.CalibrationID != "cal" || automatic.SourceAssetID != "asset" {
		t.Fatalf("automatic request: %+v", automatic)
	}
	manual := c.Request(Requirement{}, Manual, "user")
	if manual.IdempotencyKey != "" || manual.CalibrationID != "" || manual.CreatedBy != "user" {
		t.Fatalf("manual request without calibration: %+v", manual)
	}
	if link := c.Link(zoned, "job-1", Automatic, ""); link.TestSubjectID != "ts" || link.RecordingID != "rec" || link.CalibrationID != "cal" || link.JobID != "job-1" {
		t.Fatalf("link: %+v", link)
	}
}
