package datacrypto_test

import (
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

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
			for _, spec := range source.Imports {
				importPath, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					t.Fatalf("%s: decode import path: %v", sourceName, err)
				}
				if isStandardLibraryImport(importPath, directory) ||
					strings.HasPrefix(importPath, "golang.org/x/crypto/") ||
					strings.HasSuffix(sourceName, "_test.go") && importPath == "hmans.de/chatto/pkg/datacrypto" {
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
