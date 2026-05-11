package rbac

import (
	"fmt"
	"strings"
)

// KV key patterns for RBAC data.
//
// Key structure:
//   - role.{name}                                    - Role definition
//   - member.{name}.{userId}                         - Role assignment
//   - allow.{subject}.{verb}.{objectType}.{objectId} - Permission grant
//   - deny.{subject}.{verb}.{objectType}.{objectId}  - Permission denial
//
// Subject disambiguation:
//   - Role: lowercase letters only (a-z), e.g., "admin", "moderator"
//   - User ID: starts with "U" (e.g., "U9mP2qR5tYz3wK")
//
// Verb examples:
//   - "create", "join", "leave", "manage", "delete"
//   - "post", "react"
//   - "edit-own", "edit-any", "delete-own", "delete-any" (with qualifiers)
//   - "view-users", "manage-users" (compound for admin subcategories)
//
// ObjectType values:
//   - "space", "room", "message", "member", "role", "admin", "dm", "user"
//
// ObjectId values:
//   - "any" - applies to all objects of that type at this scope
//   - Specific ID - applies to that specific object (e.g., roomId, spaceId)

// ============================================================================
// Role Definitions
// ============================================================================

// RoleKey returns the KV key for a role definition.
// Format: role.{roleName}
func RoleKey(roleName string) string {
	return fmt.Sprintf("role.%s", roleName)
}

// RoleKeyPattern matches all role definitions.
const RoleKeyPattern = "role.*"

// ============================================================================
// Role Assignments (Member)
// ============================================================================

// MemberKey returns the KV key for a role assignment.
// Format: member.{roleName}.{userID}
func MemberKey(roleName, userID string) string {
	return fmt.Sprintf("member.%s.%s", roleName, userID)
}

// MemberPatternForRole returns a pattern matching all assignments for a role.
// Format: member.{roleName}.*
func MemberPatternForRole(roleName string) string {
	return fmt.Sprintf("member.%s.*", roleName)
}

// MemberPatternForUser returns a pattern matching all role assignments for a user.
// Format: member.*.{userID}
func MemberPatternForUser(userID string) string {
	return fmt.Sprintf("member.*.%s", userID)
}

// MemberKeyPattern matches all role assignments.
const MemberKeyPattern = "member.>"

// ============================================================================
// Permission Grants (Allow)
// ============================================================================

// AllowKey returns the KV key for a permission grant.
// Format: allow.{subject}.{verb}.{objectType}.{objectId}
//
// Subject is a role name or user ID:
//   - Role: "owner", "admin", "moderator", "everyone", custom roles
//   - User ID: "U9mP2qR5tYz3wK"
//
// Verb is the action (may include qualifier):
//   - "create", "join", "manage", "delete-own", "delete-any", etc.
//
// ObjectType is what the permission applies to:
//   - "space", "room", "message", "member", "role", "admin", "dm", "user"
//
// ObjectId is which specific object (or "any" for wildcard):
//   - "any" for all objects of that type
//   - Specific ID for a single object (e.g., roomId)
func AllowKey(subject, verb, objectType, objectId string) string {
	return fmt.Sprintf("allow.%s.%s.%s.%s", subject, verb, objectType, objectId)
}

// AllowPatternForSubject returns a pattern matching all grants for a subject.
// Format: allow.{subject}.>
func AllowPatternForSubject(subject string) string {
	return fmt.Sprintf("allow.%s.>", subject)
}

// AllowPatternForSubjectVerb returns a pattern matching all grants for a subject with a specific verb.
// Format: allow.{subject}.{verb}.>
func AllowPatternForSubjectVerb(subject, verb string) string {
	return fmt.Sprintf("allow.%s.%s.>", subject, verb)
}

// AllowPatternForSubjectVerbType returns a pattern matching grants for a subject/verb/objectType combo.
// Format: allow.{subject}.{verb}.{objectType}.*
func AllowPatternForSubjectVerbType(subject, verb, objectType string) string {
	return fmt.Sprintf("allow.%s.%s.%s.*", subject, verb, objectType)
}

// AllowPatternForObjectType returns a pattern matching all grants for an object type.
// Format: allow.*.*.{objectType}.*
func AllowPatternForObjectType(objectType string) string {
	return fmt.Sprintf("allow.*.*.%s.*", objectType)
}

// AllowKeyPattern matches all permission grants.
const AllowKeyPattern = "allow.>"

// ============================================================================
// Permission Denials (Deny)
// ============================================================================

// DenyKey returns the KV key for a permission denial.
// Format: deny.{subject}.{verb}.{objectType}.{objectId}
func DenyKey(subject, verb, objectType, objectId string) string {
	return fmt.Sprintf("deny.%s.%s.%s.%s", subject, verb, objectType, objectId)
}

// DenyPatternForSubject returns a pattern matching all denials for a subject.
// Format: deny.{subject}.>
func DenyPatternForSubject(subject string) string {
	return fmt.Sprintf("deny.%s.>", subject)
}

// DenyPatternForSubjectVerb returns a pattern matching all denials for a subject with a specific verb.
// Format: deny.{subject}.{verb}.>
func DenyPatternForSubjectVerb(subject, verb string) string {
	return fmt.Sprintf("deny.%s.%s.>", subject, verb)
}

// DenyPatternForSubjectVerbType returns a pattern matching denials for a subject/verb/objectType combo.
// Format: deny.{subject}.{verb}.{objectType}.*
func DenyPatternForSubjectVerbType(subject, verb, objectType string) string {
	return fmt.Sprintf("deny.%s.%s.%s.*", subject, verb, objectType)
}

// DenyPatternForObjectType returns a pattern matching all denials for an object type.
// Format: deny.*.*.{objectType}.*
func DenyPatternForObjectType(objectType string) string {
	return fmt.Sprintf("deny.*.*.%s.*", objectType)
}

// DenyKeyPattern matches all permission denials.
const DenyKeyPattern = "deny.>"

// ============================================================================
// Key Prefixes for Parsing
// ============================================================================

const (
	RoleKeyPrefix   = "role."
	MemberKeyPrefix = "member."
	AllowKeyPrefix  = "allow."
	DenyKeyPrefix   = "deny."
)

// ============================================================================
// Special ObjectId Values
// ============================================================================

// ObjectIdAny is used when a permission applies to all objects of a type.
const ObjectIdAny = "any"

// ============================================================================
// Subject Type Helpers
// ============================================================================

// IsUserSubject returns true if the subject is a user ID.
// User IDs start with "U" prefix.
func IsUserSubject(subject string) bool {
	return len(subject) > 0 && subject[0] == 'U'
}

// IsRoleSubject returns true if the subject is a role (not a user ID).
func IsRoleSubject(subject string) bool {
	return !IsUserSubject(subject)
}

// ============================================================================
// Key Parsing
// ============================================================================

// PermissionKeyParts holds the parsed components of a permission key.
type PermissionKeyParts struct {
	Subject    string
	Verb       string
	ObjectType string
	ObjectId   string
}

// ParseAllowKey extracts components from an allow key.
// Returns empty struct if the key format is invalid.
// Expected format: allow.{subject}.{verb}.{objectType}.{objectId}
func ParseAllowKey(key string) PermissionKeyParts {
	return parsePermissionKey(key, AllowKeyPrefix)
}

// ParseDenyKey extracts components from a deny key.
// Returns empty struct if the key format is invalid.
// Expected format: deny.{subject}.{verb}.{objectType}.{objectId}
func ParseDenyKey(key string) PermissionKeyParts {
	return parsePermissionKey(key, DenyKeyPrefix)
}

func parsePermissionKey(key, prefix string) PermissionKeyParts {
	if !strings.HasPrefix(key, prefix) {
		return PermissionKeyParts{}
	}

	rest := key[len(prefix):]
	parts := strings.SplitN(rest, ".", 4)
	if len(parts) != 4 {
		return PermissionKeyParts{}
	}

	return PermissionKeyParts{
		Subject:    parts[0],
		Verb:       parts[1],
		ObjectType: parts[2],
		ObjectId:   parts[3],
	}
}

// ParseMemberKey extracts role name and user ID from a member key.
// Returns empty strings if the key format is invalid.
// Expected format: member.{roleName}.{userID}
func ParseMemberKey(key string) (roleName, userID string) {
	if !strings.HasPrefix(key, MemberKeyPrefix) {
		return "", ""
	}

	rest := key[len(MemberKeyPrefix):]
	parts := strings.SplitN(rest, ".", 2)
	if len(parts) != 2 {
		return "", ""
	}

	return parts[0], parts[1]
}
