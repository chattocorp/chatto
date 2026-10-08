package intern_test

import (
	"strings"
	"testing"

	"hmans.de/chatto/pkg/events/intern"
)

func TestArenaRetainsLocationsAcrossGrowth(t *testing.T) {
	t.Parallel()
	var arena intern.Arena
	if arena.Add("") != 0 || arena.String(0) != "" || intern.Location(0).Len() != 0 {
		t.Fatal("empty string did not use the zero location")
	}
	first := arena.Add("body-42")
	retained := arena.String(first)
	if again := arena.Add("body-42"); again == first {
		t.Fatal("arena unexpectedly deduplicated an ID")
	}
	for _, size := range []int{1_024, 16_385, 65_536, 100_000} {
		id := strings.Repeat("x", size)
		location := arena.Add(id)
		if location.Len() != size || arena.String(location) != id {
			t.Fatalf("ID with %d bytes did not survive storage", size)
		}
	}
	if retained != "body-42" || arena.String(first) != retained || first.Len() != len(retained) {
		t.Fatal("arena growth changed an earlier ID")
	}
	if arena.EstimatedBytes() < 100_000 {
		t.Fatal("memory estimate omitted stored bytes")
	}
}
