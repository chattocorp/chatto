package core

// ReservedGroupNames lists names that cannot be used for custom groups.
// These are reserved for built-in mention groups (e.g. @here, @all).
var ReservedGroupNames = []string{
	"here",
	"all",
}

// IsReservedGroupName reports whether the given name is reserved and cannot
// be used as a custom group name.
func IsReservedGroupName(name string) bool {
	for _, reserved := range ReservedGroupNames {
		if name == reserved {
			return true
		}
	}
	return false
}
