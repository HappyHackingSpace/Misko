package architecture

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
)

const module = "github.com/HappyHackingSpace/Misko/backend"

// Explicit pure-library allowlist: importing net/http or database/sql is not
// acceptable merely because it belongs to the standard library.
var pure = map[string]bool{
	"bytes": true, "cmp": true, "encoding/base64": true, "encoding/binary": true, "encoding/json": true, "errors": true, "fmt": true,
	"maps": true, "math": true, "math/bits": true, "regexp": true, "slices": true,
	"sort": true, "strconv": true, "strings": true, "time": true, "unicode": true, "unicode/utf8": true,
}

// experimentSide are the modules of experiments: their design, tests,
// review and reports. Neither the analysis service nor paradigm trials may
// depend on them.
var experimentSide = []string{"experiments", "tests", "protocols", "dosing", "subjects", "subjectanalysis", "reports", "dashboard"}

// forbidden names module pairs that must never meet, whatever the layer. The
// general rule below already keeps modules apart; these pairs state the
// boundaries of the analysis service explicitly so a future exception to
// that rule cannot open them.
var forbidden = map[string][]string{
	"analysis": append([]string{"trials"}, experimentSide...),
	"trials":   experimentSide,
}

// sharedKernel is the published paradigm catalog. Only a module's
// adapters/catalog package may read it, so every module translates the
// catalog into its own domain in one place.
const sharedKernel = module + "/internal/paradigms/domain"

func allowed(file, dependency string) bool {
	parts := strings.Split(filepath.ToSlash(file), "/")
	if len(parts) < 3 || parts[0] != "internal" {
		return true
	}
	owner := parts[1]
	if owner == "bootstrap" {
		return true
	}
	for _, other := range forbidden[owner] {
		if dependency == module+"/internal/"+other || strings.HasPrefix(dependency, module+"/internal/"+other+"/") {
			return false
		}
	}
	if owner == "platform" {
		return !strings.HasPrefix(dependency, module+"/internal/") || strings.HasPrefix(dependency, module+"/internal/platform/")
	}
	layer := parts[2]
	switch layer {
	case "domain", "application":
		// The RBAC kernel is shared so every use case authorizes the same way.
		if pure[dependency] || dependency == module+"/internal/access/domain" {
			return true
		}
		if strings.HasSuffix(file, "_test.go") && (dependency == "testing" || dependency == "testing/quick") {
			return true
		}
		if layer == "application" && (dependency == "context" || dependency == "sync" || dependency == "sync/atomic") {
			return true
		}
		prefix := module + "/internal/" + owner + "/"
		if strings.HasPrefix(dependency, prefix+"domain") {
			return dependency == prefix+"domain" || strings.HasPrefix(dependency, prefix+"domain/")
		}
		return layer == "application" && (dependency == prefix+"application" || strings.HasPrefix(dependency, prefix+"application/"))
	case "adapters":
		// Cross-module calls go through interfaces the consumer declares and
		// bootstrap wires, so an adapter imports only its own module, the
		// platform, the RBAC kernel and, from adapters/catalog, the catalog.
		if !strings.HasPrefix(dependency, module+"/") {
			return true
		}
		if strings.HasPrefix(dependency, module+"/internal/platform/") || dependency == module+"/internal/access/domain" {
			return true
		}
		target := strings.Split(strings.TrimPrefix(dependency, module+"/"), "/")
		if len(target) >= 3 && target[0] == "internal" && target[1] == owner {
			return true
		}
		return dependency == sharedKernel && len(parts) >= 4 && parts[3] == "catalog"
	default:
		return false
	}
}

func scan(root string) ([]string, error) {
	var failures []string
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		// Parse all sources, including inactive build tags, so tags cannot hide a violation.
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			dependency, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			if !allowed(relative, dependency) {
				failures = append(failures, fmt.Sprintf("%s cannot import %s", relative, dependency))
			}
		}
		return nil
	})
	return failures, err
}
