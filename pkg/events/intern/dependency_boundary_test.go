package intern_test

import (
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The package must not depend on the parent events package, NATS, or any
// other module, so applications can use it without these dependencies.
func TestProductionCodeImportsOnlyStandardLibrary(t *testing.T) {
	t.Parallel()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	sourceNames, err := filepath.Glob(filepath.Join(directory, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, sourceName := range sourceNames {
		if strings.HasSuffix(sourceName, "_test.go") {
			continue
		}
		source, err := parser.ParseFile(token.NewFileSet(), sourceName, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range source.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("%s: decode import path: %v", sourceName, err)
			}
			imported, err := build.Default.Import(importPath, directory, build.FindOnly)
			if err != nil || !imported.Goroot {
				t.Errorf("%s imports non-standard-library package %q", sourceName, importPath)
			}
		}
	}
}
