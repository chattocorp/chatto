package core

// Position constants for role order. Role order is the administrative rank
// (see rbac_hierarchy.go); it does not affect permission resolution.
const (
	// A higher position ranks higher. Owner and everyone never move. Fresh
	// servers seed moderator at 100 and admin at 900, and older servers
	// placed custom roles in the gaps. The first complete reorder, including
	// the one that places each new role lowest, assigns positions upward from
	// PositionCustomFirst to every role except owner and everyone.
	PositionEveryone    int32 = 0
	PositionCustomFirst int32 = 1
	PositionModerator   int32 = 100
	PositionAdmin       int32 = 900
	PositionOwner       int32 = 1000
)

func isSystemPosition(position int32) bool {
	return position == PositionModerator || position == PositionAdmin || position == PositionOwner
}

// MaxOrderableRoles is the number of roles that fit between everyone and
// owner.
const MaxOrderableRoles = int(PositionOwner - PositionCustomFirst)

// roleIsOrderable reports whether a reorder event places roleName. Owner and
// everyone never move. A legacy order also keeps admin and moderator fixed.
func roleIsOrderable(roleName string, completeOrder bool) bool {
	if completeOrder {
		return roleName != RoleOwner && roleName != RoleEveryone
	}
	return !IsSystemRole(roleName)
}
