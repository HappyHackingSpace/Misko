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
		{"internal/media/domain/media.go", "encoding/base64", true},
		{"internal/media/domain/media.go", "hash/crc32", false},
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
		{"internal/subjects/adapters/http/handler.go", module + "/internal/identity/adapters/postgres", false},
		{"internal/bootstrap/app.go", module + "/internal/subjects/adapters/postgres", true},
		{"internal/subjects/domain/subject.go", "os", false},
		// access/domain is the shared RBAC kernel every inner layer may use.
		{"internal/subjects/domain/subject.go", module + "/internal/access/domain", true},
		{"internal/subjects/application/create.go", module + "/internal/access/domain", true},
		{"internal/subjects/application/create.go", module + "/internal/access/adapters/http", false},
		{"internal/access/domain/role.go", module + "/internal/identity/domain", false},
		{"internal/access/domain/role.go", "net/http", false},
		// Modules meet only in bootstrap, through interfaces the consumer declares.
		{"internal/subjects/adapters/http/handler.go", module + "/internal/identity/application", false},
		{"internal/trials/adapters/http/handler.go", module + "/internal/trials/application", true},
		{"internal/trials/adapters/http/handler.go", module + "/internal/access/domain", true},
		{"internal/media/adapters/http/handler.go", module + "/internal/analysis/domain", false},
		{"internal/analysis/adapters/postgres/store.go", module + "/internal/media/adapters/postgres", false},
		// The paradigm catalog is read only by a module's catalog adapter.
		{"internal/analysis/adapters/catalog/catalog.go", module + "/internal/paradigms/domain", true},
		{"internal/analysis/adapters/http/handler.go", module + "/internal/paradigms/domain", false},
		{"internal/analysis/application/service.go", module + "/internal/paradigms/domain", false},
		// The analysis service knows nothing about experiments or trials.
		{"internal/analysis/application/service.go", module + "/internal/tests/application", false},
		{"internal/analysis/adapters/postgres/store.go", module + "/internal/experiments/domain", false},
		{"internal/analysis/adapters/http/handler.go", module + "/internal/reports/application", false},
		{"internal/analysis/domain/job.go", module + "/internal/trials/domain", false},
		{"internal/analysis/adapters/catalog/catalog.go", module + "/internal/subjectanalysis/domain", false},
		// Paradigm trials never reach the experiment side.
		{"internal/trials/application/service.go", module + "/internal/tests/domain", false},
		{"internal/trials/adapters/postgres/store.go", module + "/internal/subjectanalysis/application", false},
		{"internal/trials/adapters/catalog/catalog.go", module + "/internal/experiments/adapters/catalog", false},
		{"internal/trials/adapters/http/handler.go", module + "/internal/reports/adapters/http", false},
		{"internal/trials/adapters/catalog/catalog.go", module + "/internal/paradigms/domain", true},
		{"internal/bootstrap/ports.go", module + "/internal/trials/application", true},
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
