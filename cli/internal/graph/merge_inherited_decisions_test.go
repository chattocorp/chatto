package graph

import (
	"slices"
	"testing"

	"hmans.de/chatto/internal/core"
)

// mergeInheritedDecisions is exercised end-to-end through the room-scope
// tierRoles tests, but those happy paths only cover the cases where the
// role of interest survives universal-role filtering. The pure merge
// logic is small but has a few edge cases (override wins per-permission,
// either side may be empty) that are easier to assert directly.
func TestMergeInheritedDecisions(t *testing.T) {
	cases := []struct {
		name           string
		overrideAllow  []core.Permission
		overrideDeny   []core.Permission
		parentAllow    []core.Permission
		parentDeny     []core.Permission
		wantAllow      []string
		wantDeny       []string
	}{
		{
			name: "both empty",
		},
		{
			name:        "parent only allow",
			parentAllow: []core.Permission{core.PermMessagePost},
			wantAllow:   []string{string(core.PermMessagePost)},
		},
		{
			name:       "parent only deny",
			parentDeny: []core.Permission{core.PermMessagePost},
			wantDeny:   []string{string(core.PermMessagePost)},
		},
		{
			name:          "override only allow",
			overrideAllow: []core.Permission{core.PermMessagePost},
			wantAllow:     []string{string(core.PermMessagePost)},
		},
		{
			name:         "override only deny",
			overrideDeny: []core.Permission{core.PermMessagePost},
			wantDeny:     []string{string(core.PermMessagePost)},
		},
		{
			name:          "override allow suppresses parent deny on same permission",
			overrideAllow: []core.Permission{core.PermMessagePost},
			parentDeny:    []core.Permission{core.PermMessagePost},
			wantAllow:     []string{string(core.PermMessagePost)},
		},
		{
			name:         "override deny suppresses parent allow on same permission",
			overrideDeny: []core.Permission{core.PermMessagePost},
			parentAllow:  []core.Permission{core.PermMessagePost},
			wantDeny:     []string{string(core.PermMessagePost)},
		},
		{
			name:          "non-overlapping override and parent decisions both surface",
			overrideAllow: []core.Permission{core.PermMessagePost},
			parentDeny:    []core.Permission{core.PermRoomCreate},
			wantAllow:     []string{string(core.PermMessagePost)},
			wantDeny:      []string{string(core.PermRoomCreate)},
		},
		{
			name:          "override allow + parent allow on same permission only emits once via override",
			overrideAllow: []core.Permission{core.PermMessagePost},
			parentAllow:   []core.Permission{core.PermMessagePost},
			wantAllow:     []string{string(core.PermMessagePost)},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotAllow, gotDeny := mergeInheritedDecisions(
				tc.overrideAllow, tc.overrideDeny, tc.parentAllow, tc.parentDeny,
			)
			if !slices.Equal(gotAllow, tc.wantAllow) {
				t.Errorf("allow: got %v, want %v", gotAllow, tc.wantAllow)
			}
			if !slices.Equal(gotDeny, tc.wantDeny) {
				t.Errorf("deny: got %v, want %v", gotDeny, tc.wantDeny)
			}
		})
	}
}
