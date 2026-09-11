package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func submission(run Run) Submission {
	prefix := run.OutputPrefix()
	return Submission{
		WorkerID: run.WorkerID, Attempt: run.Attempt, ModelVersion: "open-field-tracker 1.4.0",
		Artifacts: []ArtifactInput{
			{Kind: AnalyzedVideo, ObjectName: prefix + "overlay.mp4"},
			{Kind: Trajectory, ObjectName: prefix + "trajectory.json"},
			{Kind: Thumbnail, ObjectName: prefix + "thumb.png"},
		},
		Pair: PairInput{AnalyzedObjectName: prefix + "overlay.mp4", SourceOffsetUs: run.ClipStartUs, OutputOffsetUs: 0, TimeMappingVersion: "identity-v1"},
	}
}

func TestSubmissionsNameTheirOutputsAndPair(t *testing.T) {
	run, _ := queued().Claim("worker-a", now, time.Minute)
	result, err := Validate(run, submission(run))
	if err != nil {
		t.Fatal(err)
	}
	if result.DurationUs != 8_000_000 || result.TrajectoryObject != run.OutputPrefix()+"trajectory.json" || result.ModelVersion != "open-field-tracker 1.4.0" || len(result.Artifacts) != 3 {
		t.Fatalf("result: %+v", result)
	}

	with := func(mutate func(*Submission)) Submission { s := submission(run); mutate(&s); return s }
	for name, tc := range map[string]struct {
		s    Submission
		want error
	}{
		"no analyzed video": {with(func(s *Submission) { s.Artifacts = s.Artifacts[1:] }), ErrMissingAnalyzedVideo},
		"no trajectory":     {with(func(s *Submission) { s.Artifacts = append(s.Artifacts[:1], s.Artifacts[2]) }), ErrMissingTrajectory},
		"two trajectories": {with(func(s *Submission) {
			s.Artifacts[2] = ArtifactInput{Kind: Trajectory, ObjectName: run.OutputPrefix() + "b.json"}
		}), ErrMissingTrajectory},
		"unknown kind": {with(func(s *Submission) { s.Artifacts[2].Kind = "LOG" }), ErrInvalidArtifact},
		"same object twice": {with(func(s *Submission) {
			s.Artifacts[2] = ArtifactInput{Kind: Thumbnail, ObjectName: s.Artifacts[1].ObjectName}
		}), ErrInvalidArtifact},
		"artifact of another run": {with(func(s *Submission) { s.Artifacts[1].ObjectName = "runs/run-2/attempts/1/trajectory.json" }), ErrInvalidArtifact},
		"prefix only":             {with(func(s *Submission) { s.Artifacts[2].ObjectName = run.OutputPrefix() }), ErrInvalidArtifact},
		"artifact of an old attempt": {with(func(s *Submission) {
			s.Artifacts[1].ObjectName = strings.Replace(s.Artifacts[1].ObjectName, "/attempts/1/", "/attempts/0/", 1)
		}), ErrInvalidArtifact},
		"parent path":                      {with(func(s *Submission) { s.Artifacts[2].ObjectName = run.OutputPrefix() + "../x.png" }), ErrInvalidArtifact},
		"pair names another object":        {with(func(s *Submission) { s.Pair.AnalyzedObjectName = run.OutputPrefix() + "trajectory.json" }), ErrInvalidPair},
		"pair ignores the clip start":      {with(func(s *Submission) { s.Pair.SourceOffsetUs = 0 }), ErrInvalidPair},
		"negative output offset":           {with(func(s *Submission) { s.Pair.OutputOffsetUs = -1 }), ErrInvalidPair},
		"no mapping version":               {with(func(s *Submission) { s.Pair.TimeMappingVersion = " " }), ErrInvalidPair},
		"duration disagrees with the clip": {with(func(s *Submission) { s.RecordingDurationUs = 9_000_000 }), ErrInvalidDuration},
		"no model version":                 {with(func(s *Submission) { s.ModelVersion = "" }), ErrInvalidModelVersion},
	} {
		if _, err := Validate(run, tc.s); !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v want %v", name, err, tc.want)
		}
	}

	// A clip without an end takes the duration the worker measured.
	open := queued()
	open.ClipEndUs = nil
	openRun, _ := open.Claim("worker-a", now, time.Minute)
	s := submission(openRun)
	if _, err := Validate(openRun, s); !errors.Is(err, ErrInvalidDuration) {
		t.Fatalf("open clip without a measured duration: %v", err)
	}
	s.RecordingDurationUs = 7_000_000
	if r, err := Validate(openRun, s); err != nil || r.DurationUs != 7_000_000 {
		t.Fatalf("open clip: %+v %v", r, err)
	}
}
