// SPDX-FileCopyrightText: 2024-present Chatto contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package readmodel owns internal read results shared by core and transports.
// These types are not serialization contracts. Snapshot and API adapters map
// fields explicitly. Accessors are nil-safe; Clone methods detach mutable values.
package readmodel

import (
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Room is a detached room read model. Notice content is member-only.
type Room struct {
	Id          string
	Name        string
	Description string
	Archived    bool
	// GroupId is filled by grouped room reads. Catalog-only reads leave it empty.
	GroupId string
	// Canonical kind discriminator.
	Kind evtv1.RoomKind
	// Universal channel rooms grant effective membership to every server member
	// who is currently eligible to join the room. Explicit memberships are
	// preserved separately so disabling universal restores the previous state.
	Universal bool
	// Minimum number of seconds a non-exempt member must wait between original
	// message posts. Channel rooms only; zero disables slow mode.
	SlowModeSeconds uint32
	// Thread policy for channel rooms. See evtv1.RoomThreadingMode for legacy and DM
	// normalization rules.
	ThreadingMode evtv1.RoomThreadingMode
	// ShowMembershipEvents controls client timeline display, not membership delivery.
	ShowMembershipEvents bool
	// Independent per-user thread interval. Absence in legacy records uses
	// SlowModeSeconds; explicit zero disables thread slow mode.
	ThreadSlowModeSeconds *uint32
	// Creation-event timestamp, retained by the room catalog and its snapshot.
	CreatedAt *timestamppb.Timestamp
	// Separate room purpose retained by history interchange.
	Purpose string
	// Member-only notice state. Never expose this through room discovery.
	Notice *RoomNotice
}

// GetId returns the field value, or its zero value for a nil receiver.
func (x *Room) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

// GetName returns the field value, or its zero value for a nil receiver.
func (x *Room) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

// GetDescription returns the field value, or its zero value for a nil receiver.
func (x *Room) GetDescription() string {
	if x != nil {
		return x.Description
	}
	return ""
}

// GetArchived returns the field value, or its zero value for a nil receiver.
func (x *Room) GetArchived() bool {
	if x != nil {
		return x.Archived
	}
	return false
}

// GetGroupId returns the field value, or its zero value for a nil receiver.
func (x *Room) GetGroupId() string {
	if x != nil {
		return x.GroupId
	}
	return ""
}

// GetKind returns the field value, or its zero value for a nil receiver.
func (x *Room) GetKind() evtv1.RoomKind {
	if x != nil {
		return x.Kind
	}
	return evtv1.RoomKind_ROOM_KIND_UNSPECIFIED
}

// GetUniversal returns the field value, or its zero value for a nil receiver.
func (x *Room) GetUniversal() bool {
	if x != nil {
		return x.Universal
	}
	return false
}

// GetSlowModeSeconds returns the field value, or its zero value for a nil receiver.
func (x *Room) GetSlowModeSeconds() uint32 {
	if x != nil {
		return x.SlowModeSeconds
	}
	return 0
}

// GetThreadingMode returns the field value, or its zero value for a nil receiver.
func (x *Room) GetThreadingMode() evtv1.RoomThreadingMode {
	if x != nil {
		return x.ThreadingMode
	}
	return evtv1.RoomThreadingMode_ROOM_THREADING_MODE_UNSPECIFIED
}

// GetThreadSlowModeSeconds returns the field value, or its zero value for a nil receiver.
func (x *Room) GetThreadSlowModeSeconds() uint32 {
	if x != nil && x.ThreadSlowModeSeconds != nil {
		return *x.ThreadSlowModeSeconds
	}
	return 0
}

// GetCreatedAt returns the field value, or its zero value for a nil receiver.
func (x *Room) GetCreatedAt() *timestamppb.Timestamp {
	if x != nil {
		return x.CreatedAt
	}
	return nil
}

// GetNotice returns the field value, or its zero value for a nil receiver.
func (x *Room) GetNotice() *RoomNotice {
	if x != nil {
		return x.Notice
	}
	return nil
}

// Clone returns a detached copy, including nested mutable values.
func (x *Room) Clone() *Room {
	if x == nil {
		return nil
	}
	result := *x
	if x.ThreadSlowModeSeconds != nil {
		result.ThreadSlowModeSeconds = new(*x.ThreadSlowModeSeconds)
	}
	if x.CreatedAt != nil {
		result.CreatedAt = proto.Clone(x.CreatedAt).(*timestamppb.Timestamp)
	}
	result.Notice = x.Notice.Clone()
	return &result
}

// RoomNotice keeps the latest write version, including clears. Corrections preserve publication identity and time.
type RoomNotice struct {
	// ID of the latest notice-change event, including clear operations.
	Version string
	// ID of the last announcement event. Empty after clear.
	PublicationId string
	// Current Markdown source. Empty after clear.
	Body string
	// Source event time of the current publication; corrections preserve it.
	PublishedAt *timestamppb.Timestamp
}

// GetVersion returns the field value, or its zero value for a nil receiver.
func (x *RoomNotice) GetVersion() string {
	if x != nil {
		return x.Version
	}
	return ""
}

// GetPublicationId returns the field value, or its zero value for a nil receiver.
func (x *RoomNotice) GetPublicationId() string {
	if x != nil {
		return x.PublicationId
	}
	return ""
}

// GetBody returns the field value, or its zero value for a nil receiver.
func (x *RoomNotice) GetBody() string {
	if x != nil {
		return x.Body
	}
	return ""
}

// GetPublishedAt returns the field value, or its zero value for a nil receiver.
func (x *RoomNotice) GetPublishedAt() *timestamppb.Timestamp {
	if x != nil {
		return x.PublishedAt
	}
	return nil
}

// Clone returns a detached copy, including nested mutable values.
func (x *RoomNotice) Clone() *RoomNotice {
	if x == nil {
		return nil
	}
	result := *x
	if x.PublishedAt != nil {
		result.PublishedAt = proto.Clone(x.PublishedAt).(*timestamppb.Timestamp)
	}
	return &result
}

// RoomMembership identifies one user's membership in a room.
type RoomMembership struct {
	UserId string
	RoomId string
}

// GetUserId returns the field value, or its zero value for a nil receiver.
func (x *RoomMembership) GetUserId() string {
	if x != nil {
		return x.UserId
	}
	return ""
}

// GetRoomId returns the field value, or its zero value for a nil receiver.
func (x *RoomMembership) GetRoomId() string {
	if x != nil {
		return x.RoomId
	}
	return ""
}

// User is a hydrated user read model. Login, DisplayName, and Bio must not enter snapshot storage.
type User struct {
	Id           string
	Login        string // Unique login identifier
	DisplayName  string
	CreatedAt    *timestamppb.Timestamp  // When the user was created (null for users created before this field was added)
	Deleted      bool                    // True for public tombstones representing deleted/unresolvable users.
	CustomStatus *evtv1.CustomUserStatus // Optional durable user-authored status, hidden after expires_at.
	// True for an automated bot identity. False for human accounts, including
	// historical accounts created before bot identities existed.
	IsBot bool
	// Human owner of this bot. Empty for human accounts.
	BotOwnerUserId string
	// Current public bio, hydrated at read boundaries from encrypted projected
	// state. Never persisted in plaintext here.
	Bio string
	// Deactivated accounts retain their data but cannot authenticate.
	Deactivated bool
}

// GetId returns the field value, or its zero value for a nil receiver.
func (x *User) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

// GetLogin returns the field value, or its zero value for a nil receiver.
func (x *User) GetLogin() string {
	if x != nil {
		return x.Login
	}
	return ""
}

// GetDisplayName returns the field value, or its zero value for a nil receiver.
func (x *User) GetDisplayName() string {
	if x != nil {
		return x.DisplayName
	}
	return ""
}

// GetCreatedAt returns the field value, or its zero value for a nil receiver.
func (x *User) GetCreatedAt() *timestamppb.Timestamp {
	if x != nil {
		return x.CreatedAt
	}
	return nil
}

// GetDeleted returns the field value, or its zero value for a nil receiver.
func (x *User) GetDeleted() bool {
	if x != nil {
		return x.Deleted
	}
	return false
}

// GetDeactivated reports whether account access is disabled.
func (x *User) GetDeactivated() bool {
	return x != nil && x.Deactivated
}

// GetCustomStatus returns the field value, or its zero value for a nil receiver.
func (x *User) GetCustomStatus() *evtv1.CustomUserStatus {
	if x != nil {
		return x.CustomStatus
	}
	return nil
}

// GetIsBot returns the field value, or its zero value for a nil receiver.
func (x *User) GetIsBot() bool {
	if x != nil {
		return x.IsBot
	}
	return false
}

// GetBotOwnerUserId returns the field value, or its zero value for a nil receiver.
func (x *User) GetBotOwnerUserId() string {
	if x != nil {
		return x.BotOwnerUserId
	}
	return ""
}

// GetBio returns the field value, or its zero value for a nil receiver.
func (x *User) GetBio() string {
	if x != nil {
		return x.Bio
	}
	return ""
}

// Clone returns a detached copy, including nested mutable values.
func (x *User) Clone() *User {
	if x == nil {
		return nil
	}
	result := *x
	if x.CreatedAt != nil {
		result.CreatedAt = proto.Clone(x.CreatedAt).(*timestamppb.Timestamp)
	}
	if x.CustomStatus != nil {
		result.CustomStatus = proto.Clone(x.CustomStatus).(*evtv1.CustomUserStatus)
	}
	return &result
}

// Role describes a named permission subject. Position controls display order, not authorization.
type Role struct {
	Name        string // Unique identifier, lowercase + dashes only, e.g., "owner", "moderator", "content-reviewer"
	DisplayName string // Human-readable name, e.g., "Owner", "Moderator", "Content Reviewer"
	Description string
	Position    int32 // Display/order position; not an authorization rank.
	Pingable    bool  // Whether messages can ping users assigned to this role.
}

// GetName returns the field value, or its zero value for a nil receiver.
func (x *Role) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

// GetDisplayName returns the field value, or its zero value for a nil receiver.
func (x *Role) GetDisplayName() string {
	if x != nil {
		return x.DisplayName
	}
	return ""
}

// GetDescription returns the field value, or its zero value for a nil receiver.
func (x *Role) GetDescription() string {
	if x != nil {
		return x.Description
	}
	return ""
}

// GetPosition returns the field value, or its zero value for a nil receiver.
func (x *Role) GetPosition() int32 {
	if x != nil {
		return x.Position
	}
	return 0
}

// GetPingable returns the field value, or its zero value for a nil receiver.
func (x *Role) GetPingable() bool {
	if x != nil {
		return x.Pingable
	}
	return false
}

// Clone returns a detached copy, including nested mutable values.
func (x *Role) Clone() *Role {
	if x == nil {
		return nil
	}
	result := *x
	return &result
}

// RoomGroup is a detached, ordered group of rooms and sidebar links.
type RoomGroup struct {
	Id          string   // NanoID for stable identity across renames
	Name        string   // Display name (e.g., "General", "Projects")
	RoomIds     []string // Ordered list of room IDs in this group
	Description string   // Optional public group description
	// Ordered mixed sidebar entries. ROOM entries reference Room records by ID;
	// SIDEBAR_LINK entries reference payloads in sidebar_links by ID.
	Entries []*evtv1.SidebarGroupEntry
	// Link payloads owned by this group. Kept separate from entries so reorders
	// can move stable IDs without duplicating mutable label/URL data.
	SidebarLinks []*SidebarLink
}

// GetId returns the field value, or its zero value for a nil receiver.
func (x *RoomGroup) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

// GetName returns the field value, or its zero value for a nil receiver.
func (x *RoomGroup) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

// GetRoomIds returns the field value, or its zero value for a nil receiver.
func (x *RoomGroup) GetRoomIds() []string {
	if x != nil {
		return x.RoomIds
	}
	return nil
}

// GetDescription returns the field value, or its zero value for a nil receiver.
func (x *RoomGroup) GetDescription() string {
	if x != nil {
		return x.Description
	}
	return ""
}

// GetEntries returns the field value, or its zero value for a nil receiver.
func (x *RoomGroup) GetEntries() []*evtv1.SidebarGroupEntry {
	if x != nil {
		return x.Entries
	}
	return nil
}

// GetSidebarLinks returns the field value, or its zero value for a nil receiver.
func (x *RoomGroup) GetSidebarLinks() []*SidebarLink {
	if x != nil {
		return x.SidebarLinks
	}
	return nil
}

// SidebarLink is a link in the current room layout.
type SidebarLink struct {
	Id    string // NanoID for stable identity across renames/moves
	Label string // Display label shown in the server sidebar
	Url   string // Absolute http(s) URL or server-local path starting with /
}

// GetId returns the field value, or its zero value for a nil receiver.
func (x *SidebarLink) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

// GetLabel returns the field value, or its zero value for a nil receiver.
func (x *SidebarLink) GetLabel() string {
	if x != nil {
		return x.Label
	}
	return ""
}

// GetUrl returns the field value, or its zero value for a nil receiver.
func (x *SidebarLink) GetUrl() string {
	if x != nil {
		return x.Url
	}
	return ""
}

// Clone returns a detached copy, including nested mutable values.
func (x *SidebarLink) Clone() *SidebarLink {
	if x == nil {
		return nil
	}
	result := *x
	return &result
}
