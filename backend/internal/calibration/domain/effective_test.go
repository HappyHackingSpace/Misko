package domain

import (
	"errors"
	"testing"
)

func chainOf(scope Scope, statuses ...Status) []Calibration {
	var out []Calibration
	for i, s := range statuses {
		c := Calibration{ID: string(scope) + string(rune('a'+i)), Status: s, CameraID: "cam", FrameWidth: 1920, FrameHeight: 1080}
		if scope == ScopeOverride {
			c.TestSubjectID, c.RecordingID = "subject", "rec"
		}
		if i > 0 {
			c.SupersedesID = out[i-1].ID
		}
		out = append(out, c)
	}
	return out
}

func TestScopeFollowsTheTestSubject(t *testing.T) {
	if (Calibration{}).Scope() != ScopeEnvironment || (Calibration{TestSubjectID: "s", RecordingID: "r"}).Scope() != ScopeOverride {
		t.Fatal("scope must follow TestSubjectID")
	}
}

func TestResolveUsesTheLatestValidCalibration(t *testing.T) {
	for _, tc := range []struct {
		name   string
		chain  []Calibration
		source Source
		usesID string
	}{
		{"nothing calibrated", nil, "", ""},
		{"a valid default", chainOf(ScopeEnvironment, Valid), SourceEnvironment, "ENVIRONMENTa"},
		{"a rejected default waits", chainOf(ScopeEnvironment, Rejected), SourceEnvironment, ""},
		{"a corrected default uses the correction", chainOf(ScopeEnvironment, Rejected, Valid), SourceEnvironment, "ENVIRONMENTb"},
		{"a later rejected correction waits", chainOf(ScopeEnvironment, Valid, Rejected), SourceEnvironment, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Resolve(tc.chain, SourceEnvironment)
			if got.Source != tc.source {
				t.Fatalf("source %q, want %q", got.Source, tc.source)
			}
			switch {
			case tc.usesID == "" && got.Calibration != nil:
				t.Fatalf("should wait, got %s", got.Calibration.ID)
			case tc.usesID != "" && (got.Calibration == nil || got.Calibration.ID != tc.usesID):
				t.Fatalf("calibration %+v, want %s", got.Calibration, tc.usesID)
			}
		})
	}
}

func TestCorrectionsKeepTheCameraOnlyWithinOneVideo(t *testing.T) {
	chain := chainOf(ScopeOverride, Valid)
	otherCamera := Input{CameraID: "cam-2", FrameWidth: 1280, FrameHeight: 720, SupersedesID: "OVERRIDEa"}
	if err := CheckCorrection(chain, otherCamera, "rec"); !errors.Is(err, ErrCameraMismatch) {
		t.Fatalf("another camera on the same video: %v", err)
	}
	if err := CheckCorrection(chain, otherCamera, "replacement"); err != nil {
		t.Fatalf("a replacement video from another camera: %v", err)
	}
	if err := CheckCorrection(chain, Input{CameraID: "cam", FrameWidth: 1920, FrameHeight: 1080}, "replacement"); !errors.Is(err, ErrStaleCorrection) {
		t.Fatalf("a new video still continues the chain: %v", err)
	}
	if err := CheckCorrection(nil, otherCamera, "rec"); !errors.Is(err, ErrStaleCorrection) {
		t.Fatalf("the first calibration supersedes nothing: %v", err)
	}
	if err := CheckCorrection(chainOf(ScopeEnvironment, Valid), Input{CameraID: "cam", FrameWidth: 1920, FrameHeight: 1080, SupersedesID: "ENVIRONMENTa"}, ""); err != nil {
		t.Fatalf("an environment correction with the same camera: %v", err)
	}
}
