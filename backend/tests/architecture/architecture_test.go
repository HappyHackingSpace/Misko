package architecture

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLayerPolicy(t *testing.T) {
	for _, tc := range []struct {
		path, dependency string
		allowed          bool
	}{
		{"internal/subjects/domain/subject.go", "time", true},
		{"internal/subjects/domain/subject.go", "net/http", false},
		{"internal/subjects/domain/subject.go", "database/sql", false},
		{"internal/subjects/domain/subject.go", module + "/internal/subjects/application", false},
		{"internal/subjects/domain/subject.go", module + "/internal/subjects/adapters/postgres", false},
		{"internal/subjects/domain/subject.go", module + "/internal/identity/domain", false},
		{"internal/subjects/application/create.go", module + "/internal/subjects/domain", true},
		{"internal/subjects/application/create.go", "github.com/jackc/pgx/v5", false},
		{"internal/subjects/application/create.go", module + "/internal/platform/config", false},
		{"internal/subjects/application/create.go", module + "/internal/subjects/adapters/postgres", false},
		{"internal/subjects/adapters/http/handler.go", module + "/internal/subjects/application", true},
		{"internal/subjects/adapters/http/handler.go", module + "/internal/identity/application", true},
		{"internal/subjects/adapters/http/handler.go", module + "/internal/identity/adapters/postgres", false},
		{"internal/bootstrap/app.go", module + "/internal/subjects/adapters/postgres", true},
		{"internal/subjects/domain/subject.go", "os", false},
	} {
		t.Run(tc.path+"->"+tc.dependency, func(t *testing.T) {
			if got := allowed(tc.path, tc.dependency); got != tc.allowed {
				t.Errorf("allowed=%v want %v", got, tc.allowed)
			}
		})
	}
}

func TestScannerRejectsForbiddenImportsIncludingTaggedFiles(t *testing.T) {
	for _, tc := range []struct {
		dependency string
		want       int
	}{{"time", 0}, {"net/http", 1}} {
		root := t.TempDir()
		path := filepath.Join(root, "internal/subjects/domain")
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
		source := "//go:build future\n\npackage domain\nimport _ \"" + tc.dependency + "\"\n"
		if err := os.WriteFile(filepath.Join(path, "subject.go"), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		failures, err := scan(root)
		if err != nil {
			t.Fatal(err)
		}
		if len(failures) != tc.want {
			t.Fatalf("got %v want %d violations", failures, tc.want)
		}
	}
}

func TestRepositoryLayerBoundaries(t *testing.T) {
	failures, err := scan("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, failure := range failures {
		t.Error(failure)
	}
}
