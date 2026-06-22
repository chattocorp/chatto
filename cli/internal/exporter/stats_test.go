package exporter

import (
	"testing"

	"github.com/stretchr/testify/require"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestEVTStatsProjection(t *testing.T) {
	stats := newEVTStats()

	stats.apply(&corev1.Event{
		Event: &corev1.Event_UserAccountCreated{
			UserAccountCreated: &corev1.UserAccountCreatedEvent{UserId: "U1"},
		},
	}, 1)
	stats.apply(&corev1.Event{
		Event: &corev1.Event_UserAccountCreated{
			UserAccountCreated: &corev1.UserAccountCreatedEvent{UserId: "U2"},
		},
	}, 2)
	stats.apply(&corev1.Event{
		Event: &corev1.Event_UserAccountCreated{
			UserAccountCreated: &corev1.UserAccountCreatedEvent{UserId: "U3"},
		},
	}, 3)
	stats.apply(&corev1.Event{
		Event: &corev1.Event_UserVerifiedEmailAdded{
			UserVerifiedEmailAdded: &corev1.UserVerifiedEmailAddedEvent{UserId: "U1"},
		},
	}, 4)
	stats.apply(&corev1.Event{
		Event: &corev1.Event_UserVerifiedEmailAdded{
			UserVerifiedEmailAdded: &corev1.UserVerifiedEmailAddedEvent{UserId: "U2"},
		},
	}, 5)
	stats.apply(&corev1.Event{
		Event: &corev1.Event_RoomCreated{
			RoomCreated: &corev1.RoomCreatedEvent{RoomId: "R1", Kind: corev1.RoomKind_ROOM_KIND_CHANNEL},
		},
	}, 6)
	stats.apply(&corev1.Event{
		Event: &corev1.Event_RoomCreated{
			RoomCreated: &corev1.RoomCreatedEvent{RoomId: "R2", Kind: corev1.RoomKind_ROOM_KIND_DM},
		},
	}, 7)
	stats.apply(&corev1.Event{
		Id: "E-root",
		Event: &corev1.Event_MessagePosted{
			MessagePosted: &corev1.MessagePostedEvent{RoomId: "R1"},
		},
	}, 8)
	stats.apply(&corev1.Event{
		Id: "E-thread",
		Event: &corev1.Event_MessagePosted{
			MessagePosted: &corev1.MessagePostedEvent{RoomId: "R1", InThread: "E-root", InReplyTo: "E-root"},
		},
	}, 9)
	stats.apply(&corev1.Event{
		Id: "E-echo",
		Event: &corev1.Event_MessagePosted{
			MessagePosted: &corev1.MessagePostedEvent{RoomId: "R1", EchoOfEventId: "E-thread"},
		},
	}, 10)
	stats.apply(&corev1.Event{
		Event: &corev1.Event_AssetCreated{
			AssetCreated: &corev1.AssetCreatedEvent{
				Asset: &corev1.AssetRecord{
					Id:      "A1",
					Storage: &corev1.AssetRecord_Nats{Nats: &corev1.NATSAsset{Key: "A1"}},
				},
			},
		},
	}, 11)
	stats.apply(&corev1.Event{
		Event: &corev1.Event_AssetCreated{
			AssetCreated: &corev1.AssetCreatedEvent{
				Asset: &corev1.AssetRecord{
					Id:      "A2",
					Storage: &corev1.AssetRecord_S3{S3: &corev1.S3Asset{Key: "A2"}},
				},
			},
		},
	}, 12)
	stats.apply(&corev1.Event{
		Event: &corev1.Event_AssetCreated{
			AssetCreated: &corev1.AssetCreatedEvent{
				Asset: &corev1.AssetRecord{
					Id:      "A3",
					Storage: &corev1.AssetRecord_Nats{Nats: &corev1.NATSAsset{Key: "A3"}},
				},
				ParentAssetId:  "A1",
				DerivativeRole: corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_THUMBNAIL,
			},
		},
	}, 13)
	stats.apply(&corev1.Event{
		Event: &corev1.Event_AssetCreated{
			AssetCreated: &corev1.AssetCreatedEvent{
				Asset: &corev1.AssetRecord{
					Id:      "A4",
					Storage: &corev1.AssetRecord_Nats{Nats: &corev1.NATSAsset{Key: "A4"}},
				},
				ParentAssetId:  "A1",
				DerivativeRole: corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_VIDEO_VARIANT,
			},
		},
	}, 14)
	stats.apply(&corev1.Event{
		Event: &corev1.Event_AssetDeleted{
			AssetDeleted: &corev1.AssetDeletedEvent{AssetId: "A2"},
		},
	}, 15)
	stats.apply(&corev1.Event{
		Event: &corev1.Event_UserAccountDeleted{
			UserAccountDeleted: &corev1.UserAccountDeletedEvent{UserId: "U2"},
		},
	}, 16)
	stats.markReplayComplete()

	snapshot := stats.snapshot(map[string]int{"online": 3})
	require.Equal(t, map[string]int{"verified": 1, "unverified": 1}, snapshot.Users)
	require.Equal(t, map[string]int{"channel": 1, "dm": 1}, snapshot.Rooms)
	require.Equal(t, map[string]int{"root": 1, "thread": 1, "echo": 1}, snapshot.Messages)
	require.Equal(t, 1, snapshot.Assets["nats|active|original"])
	require.Equal(t, 1, snapshot.Assets["nats|active|thumbnail"])
	require.Equal(t, 1, snapshot.Assets["nats|active|video_variant"])
	require.Equal(t, 1, snapshot.Assets["s3|deleted|original"])
	require.Equal(t, 16, int(snapshot.LastSeq))
	require.True(t, snapshot.ReplayComplete)
	require.Equal(t, 3, snapshot.Presence["online"])
}
