//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"github.com/HappyHackingSpace/Misko/backend/internal/catalog/adapters/postgres"
	"github.com/HappyHackingSpace/Misko/backend/internal/catalog/application"
	"github.com/HappyHackingSpace/Misko/backend/internal/catalog/domain"
	"github.com/HappyHackingSpace/Misko/backend/internal/platform/pgtest"
	"slices"
	"testing"
)

func names[T any](items []T, name func(T) string) []string {
	var out []string
	for _, item := range items {
		out = append(out, name(item))
	}
	return out
}

func TestSubstancesAreUniqueFilteredAndProtectedWhileUsed(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.Database(t)
	store := postgres.NewStore(pool)
	create := func(name string, kind domain.SubstanceKind, description string) domain.Substance {
		t.Helper()
		s, err := store.CreateSubstance(ctx, domain.Substance{Name: name, Kind: kind, Description: description})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	saline := create("Saline 0.9%", domain.Vehicle, "")
	mptp := create("MPTP", domain.Inducer, "neurotoxin")
	create("L-DOPA", domain.ReferenceDrug, "")
	create("Compound_7", domain.Compound, "")
	if saline.ID == "" || saline.Description != "" || mptp.Description != "neurotoxin" || saline.CreatedAt.IsZero() {
		t.Fatalf("created: %+v %+v", saline, mptp)
	}
	if _, err := store.CreateSubstance(ctx, domain.Substance{Name: "mptp", Kind: domain.Compound}); !errors.Is(err, application.ErrNameTaken) {
		t.Fatalf("name taken ignoring case: %v", err)
	}

	for _, tc := range []struct {
		filter application.SubstanceFilter
		want   []string
	}{
		{application.SubstanceFilter{}, []string{"Compound_7", "L-DOPA", "MPTP", "Saline 0.9%"}},
		{application.SubstanceFilter{Search: "%"}, []string{"Saline 0.9%"}},
		{application.SubstanceFilter{Search: "_"}, []string{"Compound_7"}},
		{application.SubstanceFilter{Search: "dopa"}, []string{"L-DOPA"}},
		{application.SubstanceFilter{Kind: domain.Inducer}, []string{"MPTP"}},
		{application.SubstanceFilter{Search: "a", Kind: domain.Vehicle}, []string{"Saline 0.9%"}},
	} {
		list, err := store.Substances(ctx, tc.filter)
		if got := names(list, func(s domain.Substance) string { return s.Name }); err != nil || !slices.Equal(got, tc.want) {
			t.Errorf("%+v: %v %v want %v", tc.filter, got, err, tc.want)
		}
	}

	name, kind, empty := "MPTP-HCl", domain.Compound, ""
	updated, err := store.UpdateSubstance(ctx, mptp.ID, application.SubstanceChanges{Name: &name, Kind: &kind, Description: &empty})
	if err != nil || updated.Name != "MPTP-HCl" || updated.Kind != domain.Compound || updated.Description != "" || !updated.UpdatedAt.After(mptp.UpdatedAt) {
		t.Fatalf("update: %+v %v", updated, err)
	}
	taken := "saline 0.9%"
	if _, err := store.UpdateSubstance(ctx, mptp.ID, application.SubstanceChanges{Name: &taken}); !errors.Is(err, application.ErrNameTaken) {
		t.Fatalf("rename to a taken name: %v", err)
	}
	for _, id := range []string{"", "not-a-uuid", "00000000-0000-0000-0000-000000000000"} {
		if _, err := store.Substance(ctx, id); !errors.Is(err, application.ErrSubstanceNotFound) {
			t.Errorf("Substance(%q): %v", id, err)
		}
		if _, err := store.UpdateSubstance(ctx, id, application.SubstanceChanges{Name: &name}); !errors.Is(err, application.ErrSubstanceNotFound) {
			t.Errorf("UpdateSubstance(%q): %v", id, err)
		}
		if err := store.DeleteSubstance(ctx, id); !errors.Is(err, application.ErrSubstanceNotFound) {
			t.Errorf("DeleteSubstance(%q): %v", id, err)
		}
	}
	if _, err := pool.Exec(ctx, "INSERT INTO misko.substances (name, kind) VALUES ('X', 'SOLVENT')"); err == nil {
		t.Fatal("database accepted an unknown substance kind")
	}

	if _, err := pool.Exec(ctx, `
		WITH e AS (INSERT INTO misko.experiments (code, title) VALUES ('E1', 'Study') RETURNING id),
		     g AS (INSERT INTO misko.experiment_groups (experiment_id, code, name, role) SELECT e.id, 'G1', 'Healthy', 'HEALTHY_CONTROL' FROM e RETURNING id, experiment_id)
		INSERT INTO misko.regimen_items (experiment_id, group_id, purpose, substance_id, dose_micro, unit, route, frequency, start_day, end_day)
		SELECT g.experiment_id, g.id, 'VEHICLE', $1, 10000000, 'mL/kg', 'ORAL', 'DAILY', 0, 14 FROM g`, saline.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteSubstance(ctx, saline.ID); !errors.Is(err, application.ErrSubstanceInUse) {
		t.Fatalf("substance used by a regimen deleted: %v", err)
	}
	if err := store.DeleteSubstance(ctx, mptp.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Substance(ctx, mptp.ID); !errors.Is(err, application.ErrSubstanceNotFound) {
		t.Fatalf("deleted substance readable: %v", err)
	}
}

func TestDiseaseModelsAreUniqueFilteredAndProtectedWhileUsed(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.Database(t)
	store := postgres.NewStore(pool)
	mptp, err := store.CreateDiseaseModel(ctx, domain.DiseaseModel{Name: "MPTP parkinsonism", InductionType: domain.Chemical, ConfirmationCriterion: "pole descent time > 150% of baseline"})
	if err != nil || mptp.ConfirmationCriterion != "pole descent time > 150% of baseline" || mptp.Description != "" {
		t.Fatalf("create: %+v %v", mptp, err)
	}
	mcao, err := store.CreateDiseaseModel(ctx, domain.DiseaseModel{Name: "MCAO stroke", InductionType: domain.Surgical})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateDiseaseModel(ctx, domain.DiseaseModel{Name: "mcao STROKE", InductionType: domain.Other}); !errors.Is(err, application.ErrNameTaken) {
		t.Fatalf("name taken ignoring case: %v", err)
	}
	for _, tc := range []struct {
		filter application.DiseaseModelFilter
		want   []string
	}{
		{application.DiseaseModelFilter{}, []string{"MCAO stroke", "MPTP parkinsonism"}},
		{application.DiseaseModelFilter{Search: "PARK"}, []string{"MPTP parkinsonism"}},
		{application.DiseaseModelFilter{InductionType: domain.Surgical}, []string{"MCAO stroke"}},
		{application.DiseaseModelFilter{InductionType: domain.Diet}, nil},
	} {
		list, err := store.DiseaseModels(ctx, tc.filter)
		if got := names(list, func(m domain.DiseaseModel) string { return m.Name }); err != nil || !slices.Equal(got, tc.want) {
			t.Errorf("%+v: %v %v want %v", tc.filter, got, err, tc.want)
		}
	}
	typ, empty, description := domain.Other, "", "toxin model"
	updated, err := store.UpdateDiseaseModel(ctx, mptp.ID, application.DiseaseModelChanges{InductionType: &typ, ConfirmationCriterion: &empty, Description: &description})
	if err != nil || updated.Name != "MPTP parkinsonism" || updated.InductionType != domain.Other || updated.ConfirmationCriterion != "" || updated.Description != "toxin model" {
		t.Fatalf("update: %+v %v", updated, err)
	}
	if _, err := store.UpdateDiseaseModel(ctx, "00000000-0000-0000-0000-000000000000", application.DiseaseModelChanges{InductionType: &typ}); !errors.Is(err, application.ErrDiseaseModelNotFound) {
		t.Fatalf("update unknown: %v", err)
	}
	if _, err := store.DiseaseModel(ctx, "x"); !errors.Is(err, application.ErrDiseaseModelNotFound) {
		t.Fatalf("get malformed: %v", err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO misko.disease_models (name, induction_type) VALUES ('Tg', 'GENETIC')"); err == nil {
		t.Fatal("database accepted a genetic model")
	}

	if _, err := pool.Exec(ctx, `
		WITH e AS (INSERT INTO misko.experiments (code, title) VALUES ('E1', 'Study') RETURNING id)
		INSERT INTO misko.experiment_groups (experiment_id, code, name, role, disease_model_id)
		SELECT e.id, 'G2', 'Stroke', 'DISEASE_CONTROL', $1 FROM e`, mcao.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteDiseaseModel(ctx, mcao.ID); !errors.Is(err, application.ErrDiseaseModelInUse) {
		t.Fatalf("disease model used by a group deleted: %v", err)
	}
	if err := store.DeleteDiseaseModel(ctx, mptp.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteDiseaseModel(ctx, mptp.ID); !errors.Is(err, application.ErrDiseaseModelNotFound) {
		t.Fatalf("delete twice: %v", err)
	}
}
