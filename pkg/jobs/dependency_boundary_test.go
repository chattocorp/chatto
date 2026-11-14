package jobs

import (
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

// Production code depends only on the standard library, nats.go, protobuf,
// errgroup, and the shared events and jetstreamutil modules. Tests can also
// use nats-server.
func TestPackageDependenciesAreApplicationNeutral(t *testing.T) {
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	packages, err := parser.ParseDir(token.NewFileSet(), directory, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	allowed := []string{
		"github.com/nats-io/nats.go",
		"google.golang.org/protobuf/",
		"golang.org/x/sync/errgroup",
		"github.com/chattocorp/chatto/pkg/events",
		"github.com/chattocorp/chatto/pkg/jetstreamutil",
		"github.com/chattocorp/chatto/pkg/jobs/",
	}
	for _, pkg := range packages {
		for sourceName, source := range pkg.Files {
			isTest := strings.HasSuffix(sourceName, "_test.go")
			for _, spec := range source.Imports {
				importPath, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					t.Fatalf("%s: decode import path: %v", sourceName, err)
				}
				if isStandardLibraryImport(importPath, directory) || hasAnyPrefix(importPath, allowed) ||
					isTest && strings.HasPrefix(importPath, "github.com/nats-io/nats-server/v2") {
					continue
				}
				t.Errorf("%s imports non-portable dependency %q", sourceName, importPath)
			}
		}
	}
}

func hasAnyPrefix(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func isStandardLibraryImport(importPath, sourceDirectory string) bool {
	pkg, err := build.Default.Import(importPath, sourceDirectory, build.FindOnly)
	return err == nil && pkg.Goroot
}
