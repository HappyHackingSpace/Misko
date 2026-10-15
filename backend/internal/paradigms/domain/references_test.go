package domain

import (
	"strings"
	"testing"
)

func TestReferencesNameKnownParadigmsWithWikipediaLinks(t *testing.T) {
	known := map[string]bool{}
	for _, p := range All() {
		known[p.Key] = true
	}
	for key, ref := range references {
		if !known[key] {
			t.Errorf("reference for unknown paradigm %s", key)
		}
		if !strings.HasPrefix(ref.EN, "https://en.wikipedia.org/wiki/") {
			t.Errorf("%s: English link %q", key, ref.EN)
		}
		if ref.TR != "" && !strings.HasPrefix(ref.TR, "https://tr.wikipedia.org/wiki/") {
			t.Errorf("%s: Turkish link %q", key, ref.TR)
		}
	}
	if Reference("MWM").In("tr") != Reference("MWM").In("en") {
		t.Error("the English article is used when no Turkish one exists")
	}
	if Reference("POLE").In("en") != "" {
		t.Error("a paradigm without an article has no link")
	}
}
