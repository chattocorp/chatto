// SPDX-FileCopyrightText: 2026 Chatto contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package core

import "testing"

func TestProjectionStringTableInternsStableDenseHandles(t *testing.T) {
	table := newProjectionStringTable()

	if got := table.intern(""); got != 0 {
		t.Fatalf("empty handle = %d, want reserved zero", got)
	}
	first := table.intern("first")
	second := table.intern("second")
	if first != 1 || second != 2 {
		t.Fatalf("handles = (%d, %d), want (1, 2)", first, second)
	}
	if repeated := table.intern(string([]byte("first"))); repeated != first {
		t.Fatalf("repeated handle = %d, want %d", repeated, first)
	}
	if got := table.value(first); got != "first" {
		t.Fatalf("value(%d) = %q, want first", first, got)
	}
	if got, ok := table.lookup("missing"); ok || got != 0 {
		t.Fatalf("missing lookup = (%d, %v), want (0, false)", got, ok)
	}
	if len(table.values) != 3 || len(table.byValue) != 2 {
		t.Fatalf("table sizes = values %d map %d, want 3 and 2", len(table.values), len(table.byValue))
	}
}

func TestGrowProjectionSlicePreservesHandleIndex(t *testing.T) {
	values := []uint32{0}
	values = growProjectionSlice(values, 4)
	values[4] = 99
	values = growProjectionSlice(values, 2)
	if len(values) != 5 || values[4] != 99 {
		t.Fatalf("grown values = %v, want length 5 with handle 4 preserved", values)
	}
}
