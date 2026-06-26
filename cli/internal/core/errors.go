package core

import (
	"errors"
	"fmt"
	"time"
)

// Sentinel errors for common error conditions in ChattoCore.
var (
	// ErrNotFound is returned when a requested resource does not exist.
	ErrNotFound = errors.New("not found")

	// ErrNotAuthenticated is returned when an operation requires authentication
	// but no valid user context is available.
	ErrNotAuthenticated = errors.New("authentication required")

	// ErrPermissionDenied is returned when a user lacks the required permission
	// to perform an operation.
	ErrPermissionDenied = errors.New("permission denied")

	// ErrInvalidArgument is returned when a caller provides invalid request
	// input. Wrap this with an InvalidArgumentError when the caller should see a
	// specific validation message.
	ErrInvalidArgument = errors.New("invalid argument")

	// ErrNotSpaceMember is returned when a user attempts to access a space
	// they are not a member of.
	ErrNotSpaceMember = errors.New("not a member of this space")

	// ErrNotRoomMember is returned when a user attempts to access a room
	// they are not a member of.
	ErrNotRoomMember = errors.New("not a member of this room")

	// ErrRoleNotFound is returned when attempting to access a role that doesn't exist.
	ErrRoleNotFound = errors.New("role not found")

	// ErrImplicitRole is returned when attempting to assign or revoke an implicit role
	// (like member or verified) which cannot be explicitly managed.
	ErrImplicitRole = errors.New("implicit role cannot be explicitly assigned or revoked")

	// ErrCannotDeleteSystemRole is returned when attempting to delete a system role
	// (owner, moderator, everyone, etc.) which are protected.
	ErrCannotDeleteSystemRole = errors.New("cannot delete system role")

	// ErrInvalidRoleName is returned when a role name doesn't match the required format
	// (lowercase letters, numbers, and dashes; must start with a letter; 1-32 chars).
	ErrInvalidRoleName = errors.New("invalid role name: must be lowercase letters, numbers, and dashes, starting with a letter, 1-32 chars")

	// ErrRoleAlreadyExists is returned when attempting to create a role that already exists.
	ErrRoleAlreadyExists = errors.New("role already exists")

	// ErrInvalidPermission is returned when an unrecognized permission value is used.
	ErrInvalidPermission = errors.New("invalid permission")

	// ErrNotMessageAuthor is returned when a user attempts to edit/delete a message
	// they did not author.
	ErrNotMessageAuthor = errors.New("not the message author")

	// ErrMessageNotFound is returned when a message body doesn't exist (already deleted).
	ErrMessageNotFound = errors.New("message not found")

	// ErrMessageAttachmentNotFound is returned when a message does not contain
	// the requested attachment.
	ErrMessageAttachmentNotFound = errors.New("message attachment not found")

	// ErrMessageLinkPreviewNotFound is returned when a message does not contain
	// the requested link preview.
	ErrMessageLinkPreviewNotFound = errors.New("message link preview not found")

	// ErrEditWindowExpired is returned when attempting to edit a message
	// after the edit window has closed.
	ErrEditWindowExpired = errors.New("edit window has expired")

	// ErrMessageTooLong is returned when a message body exceeds the maximum length.
	ErrMessageTooLong = errors.New("message body exceeds maximum length")

	// ErrDisplayNameTooLong is returned when a display name exceeds the maximum length.
	ErrDisplayNameTooLong = errors.New("display name exceeds maximum length")

	// ErrDisplayNameInvalidCharacter is returned when a display name contains
	// disallowed characters (control chars, zero-width chars, consecutive spaces).
	ErrDisplayNameInvalidCharacter = errors.New("display name contains invalid characters")

	// ErrDisplayNameInvalidStart is returned when a display name does not start
	// with a letter or digit. Required so the auto-generated avatar placeholder
	// (which uses the first character) renders sensibly.
	ErrDisplayNameInvalidStart = errors.New("display name must start with a letter or digit")

	// ErrDescriptionTooLong is returned when a description exceeds the maximum length.
	ErrDescriptionTooLong = errors.New("description exceeds maximum length")

	// ErrSpaceNameTooLong is returned when a space name exceeds the maximum length.
	ErrSpaceNameTooLong = errors.New("space name exceeds maximum length")

	// ErrAdminCannotLeaveSpace is returned when a space admin tries to leave the space.
	// Admins must transfer or remove their admin role before leaving.
	ErrAdminCannotLeaveSpace = errors.New("space admin cannot leave: transfer or remove admin role first")

	// ErrCannotLeaveDMConversation is returned when a user tries to leave a DM room.
	// DM conversations are permanent and cannot be left.
	ErrCannotLeaveDMConversation = errors.New("cannot leave DM conversations")

	// ErrCannotLeaveUniversalRoom is returned when a user tries to leave a
	// universal channel room. Universal rooms grant effective membership through
	// server policy; users can mute them instead.
	ErrCannotLeaveUniversalRoom = errors.New("cannot leave universal rooms")

	// ErrCannotBanDMRoomMember is returned when a moderator tries to ban
	// someone from a DM room. DM membership is the privacy boundary and
	// cannot be moderated like a channel room.
	ErrCannotBanDMRoomMember = errors.New("cannot ban members from DM conversations")

	// ErrLoginTooShort is returned when a login is shorter than MinLoginLength.
	ErrLoginTooShort = errors.New("username must be at least 2 characters")

	// ErrLoginTooLong is returned when a login exceeds MaxLoginLength.
	ErrLoginTooLong = errors.New("username cannot exceed 32 characters")

	// ErrLoginInvalidCharacter is returned when a login contains characters
	// outside the allowed set (letters, digits, periods, underscores, hyphens).
	ErrLoginInvalidCharacter = errors.New("username can only contain letters, numbers, periods, underscores, and hyphens")

	// ErrLoginChangeCooldown is returned when a user tries to change their login
	// before the cooldown period has elapsed.
	ErrLoginChangeCooldown = errors.New("you can only change your username once every 30 days")

	// ErrInvalidEvent is returned when an event publish helper receives an invalid
	// event payload (e.g. nil pointer or missing protobuf oneof payload).
	ErrInvalidEvent = errors.New("invalid event")

	// ErrLimitExceeded is returned when an operation would exceed an instance-wide
	// resource limit configured via [limits] (e.g. max_users).
	ErrLimitExceeded = errors.New("instance limit reached")

	// ErrServerNotBootstrapped is returned by API-layer helpers that need
	// the deployment's primary space ID before its bootstrap has run.
	ErrServerNotBootstrapped = errors.New("instance not bootstrapped")

	// ErrRoomArchived is returned when an operation is rejected because the
	// room is archived. Archived rooms are read-only: no posting, editing,
	// reactions, or joining.
	ErrRoomArchived = errors.New("room is archived")

	// ErrPasswordTooShort is returned when a password is shorter than MinPasswordLength.
	ErrPasswordTooShort = fmt.Errorf("password must be at least %d characters long", MinPasswordLength)

	// ErrPasswordTooLong is returned when a password exceeds MaxPasswordLength.
	// bcrypt silently truncates input at 72 bytes, so we cap above that to ensure
	// the entire user-provided password contributes to the hash and to bound work.
	ErrPasswordTooLong = fmt.Errorf("password cannot exceed %d bytes", MaxPasswordLength)
)

// InvalidArgumentError carries a caller-safe validation message while still
// matching ErrInvalidArgument through errors.Is.
type InvalidArgumentError struct {
	Message string
}

func (e *InvalidArgumentError) Error() string {
	return e.Message
}

func (e *InvalidArgumentError) Is(target error) bool {
	return target == ErrInvalidArgument
}

func invalidArgument(message string) error {
	return &InvalidArgumentError{Message: message}
}

// Input validation limits.
// Note: These limits are enforced using len() which counts bytes, not Unicode characters.
// This is intentional for consistent storage cost control - a 10KB message costs the same
// regardless of whether it contains ASCII or multi-byte UTF-8 characters.
const (
	// MessageEditWindow is the duration after posting during which a user can edit
	// their own message. Moderators with message.manage can edit at any time.
	MessageEditWindow = 3 * time.Hour

	// MaxMessageBodyLength is the maximum length of a message body in bytes.
	MaxMessageBodyLength = 10000

	// MaxDisplayNameLength is the maximum length of a user's display name in characters.
	MaxDisplayNameLength = 32

	// MaxDescriptionLength is the maximum length of space/room descriptions in bytes.
	MaxDescriptionLength = 2000

	// MaxSpaceNameLength is the maximum length of a space name in bytes.
	MaxSpaceNameLength = 42

	// MaxLoginLength is the maximum length of a user login in characters.
	MaxLoginLength = 32

	// MinLoginLength is the minimum length of a user login in characters.
	MinLoginLength = 2

	// LoginChangeCooldown is the minimum duration between login changes.
	LoginChangeCooldown = 30 * 24 * time.Hour

	// MinPasswordLength is the minimum length of a password in bytes.
	MinPasswordLength = 8

	// MaxPasswordLength is the maximum length of a password in bytes.
	// bcrypt silently truncates input at 72 bytes; capping above that prevents
	// surprising hash collisions on long passwords sharing the same prefix while
	// still leaving room for passphrases.
	MaxPasswordLength = 128

	// MaxServerNameLength is the maximum length of a runtime-editable server name in bytes.
	MaxServerNameLength = 80

	// MaxServerDescriptionLength is the maximum length of a server description in bytes.
	MaxServerDescriptionLength = 500

	// MaxServerMOTDLength is the maximum length of a server message of the day in bytes.
	MaxServerMOTDLength = 1000

	// MaxServerWelcomeMessageLength is the maximum length of a server welcome message in bytes.
	MaxServerWelcomeMessageLength = 10000

	// MaxServerBlockedUsernamesLength is the maximum length of the blocked-username list in bytes.
	MaxServerBlockedUsernamesLength = 10000

	// MaxRoleDisplayNameLength is the maximum length of a server role display name in bytes.
	MaxRoleDisplayNameLength = 80

	// MaxRoleDescriptionLength is the maximum length of a server role description in bytes.
	MaxRoleDescriptionLength = 500

	// MaxRoomGroupNameLength is the maximum length of a room group display name in bytes.
	MaxRoomGroupNameLength = 80

	// MaxRoomGroupDescriptionLength is the maximum length of a room group description in bytes.
	MaxRoomGroupDescriptionLength = 500

	// MaxSidebarLinkLabelLength is the maximum length of a sidebar link label in bytes.
	MaxSidebarLinkLabelLength = 80

	// MaxSidebarLinkURLLength is the maximum length of a sidebar link URL in bytes.
	MaxSidebarLinkURLLength = 2048

	// MaxPushEndpointLength is the maximum length of a Push API endpoint URL in bytes.
	MaxPushEndpointLength = 4096

	// MaxPushKeyLength is the maximum length of a Push API p256dh public key in bytes.
	MaxPushKeyLength = 256

	// MaxPushAuthLength is the maximum length of a Push API auth secret in bytes.
	MaxPushAuthLength = 128

	// MaxPushUserAgentLength is the maximum length of a stored push user-agent string in bytes.
	MaxPushUserAgentLength = 512

	// MaxLinkPreviewURLLength is the maximum length of a client-provided link preview URL in bytes.
	MaxLinkPreviewURLLength = 2048

	// MaxLinkPreviewTitleLength is the maximum length of a client-provided link preview title in bytes.
	MaxLinkPreviewTitleLength = 300

	// MaxLinkPreviewDescriptionLength is the maximum length of a client-provided link preview description in bytes.
	MaxLinkPreviewDescriptionLength = 1000

	// MaxLinkPreviewSiteNameLength is the maximum length of a client-provided link preview site name in bytes.
	MaxLinkPreviewSiteNameLength = 200

	// MaxLinkPreviewEmbedTypeLength is the maximum length of a client-provided link preview embed type in bytes.
	MaxLinkPreviewEmbedTypeLength = 64

	// MaxLinkPreviewEmbedIDLength is the maximum length of a client-provided link preview embed ID in bytes.
	MaxLinkPreviewEmbedIDLength = 256

	// MaxLinkPreviewImageAssetIDLength is the maximum length of a client-provided link preview image asset ID in bytes.
	MaxLinkPreviewImageAssetIDLength = 15
)
