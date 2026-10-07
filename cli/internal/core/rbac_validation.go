package core

import (
	"errors"
	"regexp"
)

// RBAC-engine-specific validation errors. Errors common with the rest of core
// (ErrInvalidRoleName, ErrRoleNotFound, ErrRoleAlreadyExists,
// ErrPermissionDenied, ErrCannotDeleteSystemRole, ErrInvalidPermission) live in
// core/errors.go and core/rbac.go.
var (
	// ErrCannotReorderSystemRole is returned when attempting to reorder a system role.
	ErrCannotReorderSystemRole = errors.New("cannot reorder system roles")
)

// roleNameRegex matches valid role names: must start with a lowercase letter,
// may contain lowercase letters / digits / dashes in the middle, must end
// with a lowercase letter or digit. 1-32 characters.
//
// Single-character names are explicitly allowed (e.g. "a"). The end-anchor
// rules out leading/trailing dashes ("-admin", "admin-") and the regex
// disallows underscores, dots, uppercase, and unicode.
var roleNameRegex = regexp.MustCompile(`^[a-z]([a-z0-9-]{0,30}[a-z0-9])?$`)

// reservedRoleNames are path segments that role management pages use, such
// as /roles/new. A role with such a name could not be opened. Existing roles
// with these names stay valid.
var reservedRoleNames = map[string]bool{"new": true}

// ValidateRoleName checks if a name is valid for a new role.
// Valid names: lowercase letters / digits / dashes, starting with a letter,
// 1-32 characters, no leading or trailing dash, and not reserved.
func ValidateRoleName(name string) error {
	if !roleNameRegex.MatchString(name) || reservedRoleNames[name] {
		return ErrInvalidRoleName
	}
	return nil
}

func validateRoleMetadata(displayName, description string) error {
	if err := validateStringMaxLength("role display name", displayName, MaxRoleDisplayNameLength); err != nil {
		return err
	}
	if err := validateStringMaxLength("role description", description, MaxRoleDescriptionLength); err != nil {
		return err
	}
	return nil
}
