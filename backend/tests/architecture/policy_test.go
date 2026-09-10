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
	"bytes": true, "cmp": true, "encoding/json": true, "errors": true, "fmt": true,
	"maps": true, "math": true, "math/bits": true, "regexp": true, "slices": true,
	"sort": true, "strconv": true, "strings": true, "time": true, "unicode": true, "unicode/utf8": true,
}

func allowed(file, dependency string) bool {
	parts := strings.Split(filepath.ToSlash(file), "/")
	if len(parts) < 3 || parts[0] != "internal" {
		return true
	}
	owner := parts[1]
	if owner == "bootstrap" {
		return true
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
		if !strings.HasPrefix(dependency, module+"/") {
			return true
		}
		if strings.HasPrefix(dependency, module+"/internal/platform/") {
			return true
		}
		target := strings.Split(strings.TrimPrefix(dependency, module+"/"), "/")
		if len(target) < 3 || target[0] != "internal" {
			return false
		}
		if target[2] == "domain" || target[2] == "application" {
			return true
		}
		return target[1] == owner && target[2] == "adapters"
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
