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

func TestPackageAndTestsDoNotImportChattoApplicationContracts(t *testing.T) {
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
				if strings.Contains(importPath, "/internal/pb/chatto/") ||
					importPath == "hmans.de/chatto/internal/evtstream" {
					t.Errorf("%s imports application contract %q", sourceName, importPath)
				}
			}
		}
	}
}

func TestProductionPackageDependsOnlyOnStandardLibraryAndNATS(t *testing.T) {
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	packages, err := parser.ParseDir(
		token.NewFileSet(),
		directory,
		func(info os.FileInfo) bool {
			return strings.HasSuffix(info.Name(), ".go") &&
				!strings.HasSuffix(info.Name(), "_test.go")
		},
		parser.ImportsOnly,
	)
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
				if !isStandardLibraryImport(importPath, directory) &&
					!isNATSImport(importPath) {
					t.Errorf("%s imports non-framework dependency %q", sourceName, importPath)
				}
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
		if importPath == "hmans.de/chatto/internal/events" {
			importsFramework = true
		}
		if strings.HasPrefix(importPath, "hmans.de/chatto/") &&
			importPath != "hmans.de/chatto/internal/events" {
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
