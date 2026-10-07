package core

import "math"

// Position constants for role order. Role order is the administrative rank
// (see rbac_hierarchy.go); it does not affect permission resolution.
const (
	// A higher position ranks higher. Everyone always has position 0.
	// Fresh servers seed moderator at 100, admin at 900, and owner at 1000,
	// and older servers placed custom roles in the gaps. The first role move
	// or role creation renumbers every role except owner and everyone upward
	// from PositionCustomFirst and places owner directly above them.
	PositionEveryone    int32 = 0
	PositionCustomFirst int32 = 1
	PositionModerator   int32 = 100
	PositionAdmin       int32 = 900
	PositionOwner       int32 = 1000
)

// rankOwner is the rank of an effective owner: above every role position.
const rankOwner int32 = math.MaxInt32

// isSystemPosition reports whether a legacy custom-role reorder skips the
// position, because a system role held it on servers before 0.5.
func isSystemPosition(position int32) bool {
	return position == PositionModerator || position == PositionAdmin || position == PositionOwner
}

// roleIsOrderable reports whether role managers can move a role. Owner is
// always highest and everyone always lowest.
func roleIsOrderable(roleName string) bool {
	return roleName != RoleOwner && roleName != RoleEveryone
}
