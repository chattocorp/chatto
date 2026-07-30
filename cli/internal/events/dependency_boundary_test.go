package events_test

import (
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

const frameworkImportPath = "hmans.de/chatto/internal/events"

func TestPackageDependenciesArePortable(t *testing.T) {
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
					isNATSImport(importPath) ||
					isTest && (isNATSServerImport(importPath) ||
						importPath == frameworkImportPath) {
					continue
				}
				t.Errorf("%s imports non-portable dependency %q", sourceName, importPath)
			}
		}
	}
}

func TestFrameworkConsumerUsesOnlyPublicFrameworkPackage(t *testing.T) {
	source, err := parser.ParseFile(
		token.NewFileSet(),
		"framework_consumer_test.go",
		nil,
		parser.ImportsOnly,
	)
	if err != nil {
		t.Fatal(err)
	}
	if source.Name.Name != "events_test" {
		t.Fatalf(
			"framework consumer package = %q, want external package %q",
			source.Name.Name,
			"events_test",
		)
	}

	importsFramework := false
	for _, spec := range source.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatalf("decode import path: %v", err)
		}
		if importPath == frameworkImportPath {
			importsFramework = true
		}
		if strings.HasPrefix(importPath, "hmans.de/chatto/") &&
			importPath != frameworkImportPath {
			t.Errorf("external consumer imports Chatto package %q", importPath)
		}
	}
	if !importsFramework {
		t.Error("external consumer does not import the public events package")
	}
}

func isStandardLibraryImport(importPath, sourceDirectory string) bool {
	pkg, err := build.Default.Import(importPath, sourceDirectory, build.FindOnly)
	return err == nil && pkg.Goroot
}

func isNATSImport(importPath string) bool {
	return importPath == "github.com/nats-io/nats.go" ||
		strings.HasPrefix(importPath, "github.com/nats-io/nats.go/")
}

func isNATSServerImport(importPath string) bool {
	return importPath == "github.com/nats-io/nats-server/v2" ||
		strings.HasPrefix(importPath, "github.com/nats-io/nats-server/v2/")
}
