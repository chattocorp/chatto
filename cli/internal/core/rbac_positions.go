package core

// Position constants for role hierarchy.
// Higher position = higher rank (more power).
const (
	// Position numbering: higher = more power.
	//   everyone   = 0     (always; the implicit role every user holds)
	//   custom     = 1..99 (operator-defined roles slot in here)
	//   moderator  = 100
	//   admin      = 900
	//   owner      = 1000
	//
	// Wide gaps between system roles leave room for new system roles in the
	// future and let custom roles be positioned at any rank without
	// renumbering existing ones.
	PositionEveryone    int32 = 0
	PositionCustomFirst int32 = 1
	PositionModerator   int32 = 100
	PositionAdmin       int32 = 900
	PositionOwner       int32 = 1000
)

func isSystemPosition(position int32) bool {
	return position == PositionModerator || position == PositionAdmin || position == PositionOwner
}
