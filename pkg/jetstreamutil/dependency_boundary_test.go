package jetstreamutil_test

import (
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

// Production code depends only on the standard library and nats.go. Tests can
// also use nats-server and this module.
func TestPackageDependenciesAreApplicationNeutral(t *testing.T) {
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	packages, err := parser.ParseDir(token.NewFileSet(), directory, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}

	for _, pkg := range packages {
		for sourceName, source := range pkg.Files {
			isTest := strings.HasSuffix(sourceName, "_test.go")
			for _, spec := range source.Imports {
				importPath, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					t.Fatalf("%s: decode import path: %v", sourceName, err)
				}
				if isStandardLibraryImport(importPath, directory) ||
					strings.HasPrefix(importPath, "github.com/nats-io/nats.go") ||
					isTest && (strings.HasPrefix(importPath, "github.com/nats-io/nats-server/v2") ||
						importPath == "hmans.de/chatto/pkg/jetstreamutil") {
					continue
				}
				t.Errorf("%s imports non-portable dependency %q", sourceName, importPath)
			}
		}
	}
}

func isStandardLibraryImport(importPath, sourceDirectory string) bool {
	pkg, err := build.Default.Import(importPath, sourceDirectory, build.FindOnly)
	return err == nil && pkg.Goroot
}
