package application

import (
	"context"
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/catalog/domain"
	"strings"
	"testing"
)

func TestCatalogPermissions(t *testing.T) {
	ctx := context.Background()
	name := "Renamed"
	second := func(_ any, err error) error { return err }
	operations := map[string]struct {
		permission access.Permission
		run        func(*Service, access.Actor) error
	}{
		"list substances": {access.Read, func(s *Service, a access.Actor) error { return second(s.Substances(ctx, a, SubstanceQuery{})) }},
		"get substance":   {access.Read, func(s *Service, a access.Actor) error { return second(s.Substance(ctx, a, "id")) }},
		"create substance": {access.StudyWrite, func(s *Service, a access.Actor) error {
			return second(s.CreateSubstance(ctx, a, SubstanceInput{Name: "Saline", Kind: "VEHICLE"}))
		}},
		"update substance": {access.StudyWrite, func(s *Service, a access.Actor) error {
			return second(s.UpdateSubstance(ctx, a, "id", SubstancePatch{Name: &name}))
		}},
		"delete substance":    {access.StudyWrite, func(s *Service, a access.Actor) error { return s.DeleteSubstance(ctx, a, "id") }},
		"list disease models": {access.Read, func(s *Service, a access.Actor) error { return second(s.DiseaseModels(ctx, a, DiseaseModelQuery{})) }},
		"get disease model":   {access.Read, func(s *Service, a access.Actor) error { return second(s.DiseaseModel(ctx, a, "id")) }},
		"create disease model": {access.StudyWrite, func(s *Service, a access.Actor) error {
			return second(s.CreateDiseaseModel(ctx, a, DiseaseModelInput{Name: "MPTP", InductionType: "CHEMICAL"}))
		}},
		"update disease model": {access.StudyWrite, func(s *Service, a access.Actor) error {
			return second(s.UpdateDiseaseModel(ctx, a, "id", DiseaseModelPatch{Name: &name}))
		}},
		"delete disease model": {access.StudyWrite, func(s *Service, a access.Actor) error { return s.DeleteDiseaseModel(ctx, a, "id") }},
	}
	for _, role := range append(access.Roles(), "ROOT") {
		for name, op := range operations {
			store := &fakeStore{}
			err := op.run(New(store), access.Actor{UserID: "u", Role: role})
			allowed := role.Allows(access.Read) && role.Allows(op.permission)
			if allowed != (err == nil) || (!allowed && !errors.Is(err, access.ErrForbidden)) {
				t.Errorf("%s %s: err=%v allowed=%v", role, name, err, allowed)
			}
			if !allowed && store.calls != 0 {
				t.Errorf("%s %s: denied operation reached the store", role, name)
			}
		}
	}
	for name, op := range operations {
		if err := op.run(New(&fakeStore{}), access.Actor{}); !errors.Is(err, access.ErrUnauthenticated) {
			t.Errorf("anonymous %s: %v", name, err)
		}
	}
}

func TestListQueriesAreValidated(t *testing.T) {
	ctx := context.Background()
	viewer := access.Actor{UserID: "u", Role: access.Viewer}
	store := &fakeStore{}
	s := New(store)
	for _, q := range []SubstanceQuery{{Kind: "vehicle"}, {Kind: "DRUG"}, {Search: strings.Repeat("a", 101)}, {Search: "\xff"}} {
		if _, err := s.Substances(ctx, viewer, q); !errors.Is(err, ErrInvalidQuery) {
			t.Errorf("%+v: %v", q, err)
		}
	}
	for _, q := range []DiseaseModelQuery{{InductionType: "GENETIC"}, {Search: strings.Repeat("a", 101)}} {
		if _, err := s.DiseaseModels(ctx, viewer, q); !errors.Is(err, ErrInvalidQuery) {
			t.Errorf("%+v: %v", q, err)
		}
	}
	if store.calls != 0 {
		t.Fatalf("invalid queries reached the store %d times", store.calls)
	}
	if _, err := s.Substances(ctx, viewer, SubstanceQuery{Search: " sal ", Kind: "VEHICLE"}); err != nil || store.substanceFilter != (SubstanceFilter{Search: "sal", Kind: domain.Vehicle}) {
		t.Fatalf("substance filter %+v %v", store.substanceFilter, err)
	}
	if _, err := s.DiseaseModels(ctx, viewer, DiseaseModelQuery{Search: "mptp", InductionType: "CHEMICAL"}); err != nil || store.modelFilter != (DiseaseModelFilter{Search: "mptp", InductionType: domain.Chemical}) {
		t.Fatalf("disease model filter %+v %v", store.modelFilter, err)
	}
}

func TestUpdatesValidateProvidedFieldsOnly(t *testing.T) {
	ctx := context.Background()
	researcher := access.Actor{UserID: "u", Role: access.Researcher}
	store := &fakeStore{}
	s := New(store)
	bad, kind, empty, name, typ := "SOLVENT", "INDUCER", "", " LPS ", "OTHER"
	if _, err := s.UpdateSubstance(ctx, researcher, "id", SubstancePatch{}); !errors.Is(err, ErrNoChanges) {
		t.Fatalf("empty substance patch: %v", err)
	}
	if _, err := s.UpdateSubstance(ctx, researcher, "id", SubstancePatch{Kind: &bad}); !errors.Is(err, domain.ErrInvalidSubstanceKind) || store.calls != 0 {
		t.Fatalf("invalid kind: %v calls=%d", err, store.calls)
	}
	if _, err := s.UpdateSubstance(ctx, researcher, "id", SubstancePatch{Name: &name, Kind: &kind, Description: &empty}); err != nil {
		t.Fatal(err)
	}
	if c := store.substanceChanges; *c.Name != "LPS" || *c.Kind != domain.Inducer || *c.Description != "" {
		t.Fatalf("substance changes %+v", c)
	}
	if _, err := s.UpdateDiseaseModel(ctx, researcher, "id", DiseaseModelPatch{}); !errors.Is(err, ErrNoChanges) {
		t.Fatalf("empty disease model patch: %v", err)
	}
	if _, err := s.UpdateDiseaseModel(ctx, researcher, "id", DiseaseModelPatch{InductionType: &bad}); !errors.Is(err, domain.ErrInvalidInductionType) {
		t.Fatalf("invalid induction type: %v", err)
	}
	if _, err := s.UpdateDiseaseModel(ctx, researcher, "id", DiseaseModelPatch{InductionType: &typ, ConfirmationCriterion: &empty}); err != nil {
		t.Fatal(err)
	}
	if c := store.modelChanges; c.Name != nil || *c.InductionType != domain.Other || *c.ConfirmationCriterion != "" || c.Description != nil {
		t.Fatalf("disease model changes %+v", c)
	}
}

func TestCreateRejectsInvalidInputBeforeStoring(t *testing.T) {
	ctx := context.Background()
	researcher := access.Actor{UserID: "u", Role: access.Researcher}
	store := &fakeStore{}
	s := New(store)
	if _, err := s.CreateSubstance(ctx, researcher, SubstanceInput{Name: "Saline"}); !errors.Is(err, domain.ErrInvalidSubstanceKind) {
		t.Fatalf("missing kind: %v", err)
	}
	if _, err := s.CreateDiseaseModel(ctx, researcher, DiseaseModelInput{Name: "MPTP"}); !errors.Is(err, domain.ErrInvalidInductionType) {
		t.Fatalf("missing induction type: %v", err)
	}
	if store.calls != 0 {
		t.Fatalf("invalid input reached the store %d times", store.calls)
	}
}

type fakeStore struct {
	calls            int
	substanceFilter  SubstanceFilter
	modelFilter      DiseaseModelFilter
	substanceChanges SubstanceChanges
	modelChanges     DiseaseModelChanges
}

func (f *fakeStore) Substances(_ context.Context, filter SubstanceFilter) ([]domain.Substance, error) {
	f.calls++
	f.substanceFilter = filter
	return nil, nil
}

func (f *fakeStore) Substance(context.Context, string) (domain.Substance, error) {
	f.calls++
	return domain.Substance{}, nil
}

func (f *fakeStore) CreateSubstance(_ context.Context, s domain.Substance) (domain.Substance, error) {
	f.calls++
	return s, nil
}

func (f *fakeStore) UpdateSubstance(_ context.Context, _ string, c SubstanceChanges) (domain.Substance, error) {
	f.calls++
	f.substanceChanges = c
	return domain.Substance{}, nil
}

func (f *fakeStore) DeleteSubstance(context.Context, string) error {
	f.calls++
	return nil
}

func (f *fakeStore) DiseaseModels(_ context.Context, filter DiseaseModelFilter) ([]domain.DiseaseModel, error) {
	f.calls++
	f.modelFilter = filter
	return nil, nil
}

func (f *fakeStore) DiseaseModel(context.Context, string) (domain.DiseaseModel, error) {
	f.calls++
	return domain.DiseaseModel{}, nil
}

func (f *fakeStore) CreateDiseaseModel(_ context.Context, m domain.DiseaseModel) (domain.DiseaseModel, error) {
	f.calls++
	return m, nil
}

func (f *fakeStore) UpdateDiseaseModel(_ context.Context, _ string, c DiseaseModelChanges) (domain.DiseaseModel, error) {
	f.calls++
	f.modelChanges = c
	return domain.DiseaseModel{}, nil
}

func (f *fakeStore) DeleteDiseaseModel(context.Context, string) error {
	f.calls++
	return nil
}
