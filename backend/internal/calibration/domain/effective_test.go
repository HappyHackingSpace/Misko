package domain

import "testing"

func chainOf(scope Scope, statuses ...Status) []Calibration {
	var out []Calibration
	for i, s := range statuses {
		c := Calibration{ID: string(scope) + string(rune('a'+i)), Status: s}
		if scope == ScopeRecording {
			c.RecordingID = "rec"
		}
		if i > 0 {
			c.SupersedesID = out[i-1].ID
		}
		out = append(out, c)
	}
	return out
}

func TestScopeFollowsTheRecording(t *testing.T) {
	if (Calibration{}).Scope() != ScopeEnvironment || (Calibration{RecordingID: "r"}).Scope() != ScopeRecording {
		t.Fatal("scope must follow RecordingID")
	}
}

func TestEffectiveResolution(t *testing.T) {
	tests := []struct {
		name        string
		recording   []Calibration
		environment []Calibration
		source      Source
		usesID      string
	}{
		{"nothing calibrated", nil, nil, "", ""},
		{"environment default applies to every recording", nil, chainOf(ScopeEnvironment, Valid), SourceEnvironment, "ENVIRONMENTa"},
		{"rejected environment default waits", nil, chainOf(ScopeEnvironment, Rejected), SourceEnvironment, ""},
		{"corrected environment default uses the correction", nil, chainOf(ScopeEnvironment, Rejected, Valid), SourceEnvironment, "ENVIRONMENTb"},
		{"a valid override beats the environment", chainOf(ScopeRecording, Valid), chainOf(ScopeEnvironment, Valid), SourceRecording, "RECORDINGa"},
		{"an override works without an environment default", chainOf(ScopeRecording, Valid), nil, SourceRecording, "RECORDINGa"},
		{"a rejected override never falls back to the environment", chainOf(ScopeRecording, Rejected), chainOf(ScopeEnvironment, Valid), SourceRecording, ""},
		{"a corrected override is used", chainOf(ScopeRecording, Rejected, Valid), chainOf(ScopeEnvironment, Valid), SourceRecording, "RECORDINGb"},
		{"a later rejected correction of a valid override waits", chainOf(ScopeRecording, Valid, Rejected), chainOf(ScopeEnvironment, Valid), SourceRecording, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Effective(tc.recording, tc.environment)
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
