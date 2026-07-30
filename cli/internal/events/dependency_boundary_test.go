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
