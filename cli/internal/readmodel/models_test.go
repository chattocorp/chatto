// SPDX-FileCopyrightText: 2024-present Chatto contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package readmodel

import (
	"testing"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestRoomCloneDetachesMutableFields(t *testing.T) {
	t.Parallel()
	original := &Room{
		CreatedAt:             &timestamppb.Timestamp{Seconds: 1},
		ThreadSlowModeSeconds: new(uint32(0)),
		Notice:                &RoomNotice{Version: "write", PublicationId: "announcement", Body: "notice", PublishedAt: &timestamppb.Timestamp{Seconds: 2}},
	}
	clone := original.Clone()
	clone.CreatedAt.Seconds = 3
	*clone.ThreadSlowModeSeconds = 4
	clone.Notice.Body = "changed"
	clone.Notice.PublishedAt.Seconds = 5
	require.Equal(t, int64(1), original.CreatedAt.Seconds)
	require.Equal(t, uint32(0), *original.ThreadSlowModeSeconds)
	require.Equal(t, "notice", original.Notice.Body)
	require.Equal(t, int64(2), original.Notice.PublishedAt.Seconds)
	require.Equal(t, "write", clone.Notice.Version)
	require.Equal(t, "announcement", clone.Notice.PublicationId)
	require.Nil(t, (&Room{}).Clone().ThreadSlowModeSeconds)
	require.Nil(t, (*Room)(nil).Clone())
}

func TestUserCloneDetachesEventValues(t *testing.T) {
	t.Parallel()
	original := &User{
		CreatedAt:    &timestamppb.Timestamp{Seconds: 1},
		CustomStatus: &evtv1.CustomUserStatus{Text: "status", ExpiresAt: &timestamppb.Timestamp{Seconds: 2}},
	}
	clone := original.Clone()
	clone.CreatedAt.Seconds = 3
	clone.CustomStatus.Text = "changed"
	clone.CustomStatus.ExpiresAt.Seconds = 4
	require.Equal(t, int64(1), original.CreatedAt.Seconds)
	require.Equal(t, "status", original.CustomStatus.Text)
	require.Equal(t, int64(2), original.CustomStatus.ExpiresAt.Seconds)
	require.Nil(t, (*User)(nil).Clone())
}
