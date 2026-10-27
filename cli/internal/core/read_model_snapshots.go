// SPDX-FileCopyrightText: 2024-present Chatto contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package core

import (
	projectionv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/projection/v1"
	"github.com/chattocorp/chatto/cli/internal/readmodel"
)

// Snapshot adapters run under projection locks. Their nested values stay owned
// by the projection until marshal completes. Restore adapters receive values
// owned by a newly decoded snapshot, never caller-owned read results.

func snapshotUser(value *readmodel.User) *projectionv1.UserSnapshot {
	if value == nil {
		return nil
	}
	return &projectionv1.UserSnapshot{
		Id:             value.Id,
		CreatedAt:      value.CreatedAt,
		Deleted:        value.Deleted,
		Deactivated:    value.Deactivated,
		CustomStatus:   value.CustomStatus,
		IsBot:          value.IsBot,
		BotOwnerUserId: value.BotOwnerUserId,
	}
}

func restoreUser(value *projectionv1.UserSnapshot) *readmodel.User {
	if value == nil {
		return nil
	}
	return &readmodel.User{
		Id:             value.Id,
		CreatedAt:      value.CreatedAt,
		Deleted:        value.Deleted,
		Deactivated:    value.Deactivated,
		CustomStatus:   value.CustomStatus,
		IsBot:          value.IsBot,
		BotOwnerUserId: value.BotOwnerUserId,
	}
}

func snapshotRole(value *readmodel.Role) *projectionv1.RoleSnapshot {
	if value == nil {
		return nil
	}
	return &projectionv1.RoleSnapshot{
		Name:        value.Name,
		DisplayName: value.DisplayName,
		Description: value.Description,
		Position:    value.Position,
		Pingable:    value.Pingable,
	}
}

func restoreRole(value *projectionv1.RoleSnapshot) *readmodel.Role {
	if value == nil {
		return nil
	}
	return &readmodel.Role{
		Name:        value.Name,
		DisplayName: value.DisplayName,
		Description: value.Description,
		Position:    value.Position,
		Pingable:    value.Pingable,
	}
}

func snapshotRoomNotice(value *readmodel.RoomNotice) *projectionv1.RoomNoticeSnapshot {
	if value == nil {
		return nil
	}
	return &projectionv1.RoomNoticeSnapshot{
		Version:       value.Version,
		PublicationId: value.PublicationId,
		Body:          value.Body,
		PublishedAt:   value.PublishedAt,
	}
}

func restoreRoomNotice(value *projectionv1.RoomNoticeSnapshot) *readmodel.RoomNotice {
	if value == nil {
		return nil
	}
	return &readmodel.RoomNotice{
		Version:       value.Version,
		PublicationId: value.PublicationId,
		Body:          value.Body,
		PublishedAt:   value.PublishedAt,
	}
}

func snapshotSidebarLink(value *readmodel.SidebarLink) *projectionv1.SidebarLinkSnapshot {
	if value == nil {
		return nil
	}
	return &projectionv1.SidebarLinkSnapshot{
		Id:    value.Id,
		Label: value.Label,
		Url:   value.Url,
	}
}

func restoreSidebarLink(value *projectionv1.SidebarLinkSnapshot) *readmodel.SidebarLink {
	if value == nil {
		return nil
	}
	return &readmodel.SidebarLink{
		Id:    value.Id,
		Label: value.Label,
		Url:   value.Url,
	}
}
