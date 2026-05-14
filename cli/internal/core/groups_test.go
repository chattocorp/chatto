package core

import "testing"

func TestIsReservedGroupName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"here", true},
		{"all", true},
		{"team", false},
		{"frontend", false},
		{"", false},
		{"Here", false}, // case-sensitive
		{"ALL", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsReservedGroupName(tt.name); got != tt.want {
				t.Errorf("IsReservedGroupName(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
