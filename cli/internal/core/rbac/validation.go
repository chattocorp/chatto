package rbac

import (
	"errors"
	"regexp"
)

// Validation errors.
var (
	// ErrInvalidRoleName is returned when a role name doesn't match the required format.
	ErrInvalidRoleName = errors.New("invalid role name: must be lowercase letters, numbers, and dashes, starting with a letter, 1-32 chars")

	// ErrRoleNotFound is returned when a role doesn't exist.
	ErrRoleNotFound = errors.New("role not found")

	// ErrRoleAlreadyExists is returned when attempting to create a role that already exists.
	ErrRoleAlreadyExists = errors.New("role already exists")

	// ErrCannotDeleteSystemRole is returned when attempting to delete a system role.
	ErrCannotDeleteSystemRole = errors.New("cannot delete system role")

	// ErrInvalidPermission is returned when a permission value is not recognized.
	ErrInvalidPermission = errors.New("invalid permission")

	// ErrPermissionDenied is returned when a user lacks the required permission.
	ErrPermissionDenied = errors.New("permission denied")

	// ErrCannotAssignHigherRole is returned when a user tries to assign a role equal to or higher than their own.
	ErrCannotAssignHigherRole = errors.New("cannot assign role equal to or higher than your own")

	// ErrCannotRevokeHigherRole is returned when a user tries to revoke a role equal to or higher than their own.
	ErrCannotRevokeHigherRole = errors.New("cannot revoke role equal to or higher than your own")

	// ErrCannotManageHigherUser is returned when a user tries to manage another user with equal or higher role.
	ErrCannotManageHigherUser = errors.New("cannot manage user with equal or higher role")

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

// ValidateRoleName checks if a role name is valid.
// Valid names: lowercase letters / digits / dashes, starting with a letter,
// 1-32 characters, no leading or trailing dash.
func ValidateRoleName(name string) error {
	if !roleNameRegex.MatchString(name) {
		return ErrInvalidRoleName
	}
	return nil
}
