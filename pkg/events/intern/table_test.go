package intern_test

import (
	"slices"
	"testing"

	"hmans.de/chatto/pkg/events/intern"
)

// The same public contract applies to a local table and a shared table.
type testKind struct{}

type idTable interface {
	Intern(string) intern.ID[testKind]
	Lookup(string) (intern.ID[testKind], bool)
	Resolve(intern.ID[testKind]) string
	Len() int
	EstimatedBytes() int64
}

func TestTableContract(t *testing.T) {
	t.Parallel()
	for name, create := range map[string]func() idTable{
		"local":      func() idTable { return &intern.Table[testKind]{} },
		"concurrent": func() idTable { return intern.NewConcurrentTable[testKind]() },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			table := create()
			if table.Intern("") != 0 || table.Resolve(0) != "" || table.Len() != 0 {
				t.Fatal("empty ID allocated a handle")
			}
			for _, id := range []string{"", "missing"} {
				if h, ok := table.Lookup(id); ok || h != 0 {
					t.Fatal("unknown ID was found")
				}
			}
			// IDs are opaque bytes, with no UUID, text, or event-envelope rule.
			ids := []string{"resource-42", "actor-7", "\x00\xff", "世界"}
			for i, id := range ids {
				h := table.Intern(id)
				if h != intern.ID[testKind](i+1) || table.Intern(id) != h || table.Resolve(h) != id {
					t.Fatalf("ID %d did not retain a dense, stable handle", i)
				}
				if found, ok := table.Lookup(id); !ok || found != h {
					t.Fatalf("ID %d was not found by its handle", i)
				}
			}
			if table.Len() != len(ids) || table.EstimatedBytes() <= 0 {
				t.Fatal("table count or memory estimate is invalid")
			}
			// Restore strings into a different insertion order. Numeric handles
			// need not survive, but the associated application values must.
			restored := create()
			restored.Intern("earlier-resource")
			for i := range slices.Backward(ids) {
				id := table.Resolve(intern.ID[testKind](i + 1))
				if restored.Resolve(restored.Intern(id)) != ids[i] {
					t.Fatal("restore changed an ID")
				}
			}
			if table.Intern(ids[0]) == restored.Intern(ids[0]) {
				t.Fatal("fixture did not exercise different handle assignments")
			}
		})
	}
}
