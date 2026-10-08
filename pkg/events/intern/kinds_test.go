package intern_test

import (
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"unsafe"

	"hmans.de/chatto/pkg/events/intern"
)

func TestIDKeepsCompactRepresentation(t *testing.T) {
	t.Parallel()
	// Even a kind containing pointers contributes no storage to an ID.
	type kind struct{ value *string }
	var id intern.ID[kind]
	if unsafe.Sizeof(id) != 4 || unsafe.Alignof(id) != unsafe.Alignof(uint32(0)) ||
		reflect.TypeOf(id).Kind() != reflect.Uint32 {
		t.Fatal("ID must remain a pointer-free four-byte integer")
	}
}

func TestKindsRejectAccidentalMixing(t *testing.T) {
	t.Parallel()
	// Compile an external consumer against the actual exported API. testdata
	// keeps this intentionally invalid package outside ordinary ./... builds.
	cmd := exec.Command("go", "test", "./testdata/kinds")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("Go accepted IDs from different kinds")
	}
	text := string(output)
	for _, expected := range []string{"as intern.ID[roomKind] value in variable declaration", "as intern.ID[roomKind] value in argument to rooms.Resolve"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected type rejection %q, got:\n%s", expected, text)
		}
	}
}
