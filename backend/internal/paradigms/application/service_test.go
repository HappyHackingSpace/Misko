package application

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/paradigms/domain"
	"testing"
)

func TestCatalogReadsFollowReadPermission(t *testing.T) {
	ctx := context.Background()
	s := New(nil)
	for _, role := range append(access.Roles(), "ROOT") {
		actor := access.Actor{UserID: "u", Role: role}
		_, listErr := s.List(ctx, actor)
		_, latestErr := s.Latest(ctx, actor, "OPEN_FIELD")
		_, versionErr := s.Version(ctx, actor, "OPEN_FIELD", 1)
		for name, err := range map[string]error{"list": listErr, "latest": latestErr, "version": versionErr} {
			if role.Allows(access.Read) != (err == nil) || (err != nil && !errors.Is(err, access.ErrForbidden)) {
				t.Errorf("%s %s: %v", role, name, err)
			}
		}
	}
	if _, err := s.List(ctx, access.Actor{}); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatalf("anonymous: %v", err)
	}
	viewer := access.Actor{UserID: "u", Role: access.Viewer}
	if _, err := s.Latest(ctx, viewer, "MAZE"); !errors.Is(err, domain.ErrUnknownParadigm) {
		t.Fatalf("unknown key: %v", err)
	}
	if _, err := s.Version(ctx, viewer, "OPEN_FIELD", 9); !errors.Is(err, domain.ErrUnknownVersion) {
		t.Fatalf("unknown version: %v", err)
	}
	list, err := s.List(ctx, viewer)
	if err != nil || len(list) != len(domain.Keys()) || len(list) != 11 {
		t.Fatalf("list: %+v %v", list, err)
	}
	for i, summary := range list {
		if summary.Key != domain.Keys()[i] || summary.AutomatedAnalysis {
			t.Fatalf("catalog presence must not imply automated analysis: %+v", summary)
		}
	}
	m, err := s.Version(ctx, viewer, "OPEN_FIELD", 1)
	if err != nil || m.MetricEngineVersion != domain.MetricEngineVersion || m.ResultSchemaVersion != domain.ResultSchemaVersion || m.Paradigm.Version != 1 {
		t.Fatalf("manifest versions: %+v %v", m, err)
	}
}

type fakeCapabilities []Capability

func (f fakeCapabilities) Active(context.Context) ([]Capability, error) { return f, nil }

func TestAutomatedAnalysisFollowsActiveWorkerCapabilities(t *testing.T) {
	ctx := context.Background()
	viewer := access.Actor{UserID: "u", Role: access.Viewer}
	s := New(fakeCapabilities{{Key: "OPEN_FIELD", Version: 1}})
	list, err := s.List(ctx, viewer)
	if err != nil {
		t.Fatal(err)
	}
	for _, summary := range list {
		if want := summary.Key == "OPEN_FIELD"; summary.AutomatedAnalysis != want {
			t.Errorf("%s automated = %v, want %v", summary.Key, summary.AutomatedAnalysis, want)
		}
	}
	m, err := s.Latest(ctx, viewer, "OPEN_FIELD")
	if err != nil || !m.AutomatedAnalysis {
		t.Fatalf("OPEN_FIELD latest: %+v %v", m, err)
	}
	m, err = s.Latest(ctx, viewer, "EPM")
	if err != nil || m.AutomatedAnalysis {
		t.Fatalf("EPM latest: %+v %v", m, err)
	}
}
