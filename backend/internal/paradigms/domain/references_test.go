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
	if Reference("Y_MAZE").In("tr") == Reference("Y_MAZE").In("en") || Reference("EPM").In("tr") != Reference("EPM").In("en") {
		t.Error("a Turkish article is used when one exists, English otherwise")
	}
	if Reference("POLE").In("en") != "" {
		t.Error("a paradigm without an article has no link")
	}
}
