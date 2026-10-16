package domain

import (
	"slices"
	"testing"
)

func TestObserverScoredParadigms(t *testing.T) {
	var scored []string
	for _, p := range All() {
		if p.ObserverScored() {
			scored = append(scored, p.Key)
		}
	}
	if want := []string{"NOVEL_OBJECT", "POLE", "TREADMILL"}; !slices.Equal(scored, want) {
		t.Fatalf("observer scored paradigms: %v want %v", scored, want)
	}
}
