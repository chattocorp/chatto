package events_test

import (
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
				if strings.Contains(importPath, ".") &&
					importPath != "github.com/nats-io/nats.go" &&
					!strings.HasPrefix(importPath, "github.com/nats-io/nats.go/") {
					t.Errorf("%s imports non-framework dependency %q", sourceName, importPath)
				}
			}
		}
	}
}
