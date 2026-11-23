package core

import (
	"testing"
	"time"

	notificationv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/notification/v1"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestNotificationRowsReproduceOccurrences checks that a stored occurrence
// reads back unchanged, in the compact form where one exists and in the
// complete form otherwise.
func TestNotificationRowsReproduceOccurrences(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 123, time.UTC)
	base := func(signal *notificationv1.NotificationSignal) *notificationv1.NotificationOccurrence {
		return &notificationv1.NotificationOccurrence{
			Id: "N1", RecipientId: "U1", SourceEventId: "E1", ActorId: "U2",
			SourceCreatedAt: timestamppb.New(now), ExpiresAt: timestamppb.New(now.Add(time.Hour)),
			Signal: signal, SourceStreamSequence: 7, NotificationStreamSequence: 9,
			AttentionLevel: notificationv1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT,
		}
	}
	type testCase struct {
		name       string
		occurrence *notificationv1.NotificationOccurrence
		compact    bool
	}
	var tests []testCase
	for _, kind := range []notificationTestSignalKind{
		notificationTestSignalDirectMessage, notificationTestSignalDirectMention, notificationTestSignalReply,
		notificationTestSignalRoleMention, notificationTestSignalHere, notificationTestSignalAll,
		notificationTestSignalFollowedThread, notificationTestSignalFollowedRoom, notificationTestSignalReaction,
		notificationTestSignalRoomMessage,
	} {
		tests = append(tests, testCase{name: string(kind), occurrence: base(testNotificationSignal(kind, "R1", "M1")), compact: true})
	}
	change := func(name string, compact bool, edit func(*notificationv1.NotificationOccurrence)) {
		occurrence := base(testNotificationSignal(notificationTestSignalReply, "R1", "M1"))
		edit(occurrence)
		tests = append(tests, testCase{name: name, occurrence: occurrence, compact: compact})
	}
	change("thread and alert state", true, func(o *notificationv1.NotificationOccurrence) {
		notificationSignalMessage(o.GetSignal()).ThreadRootEventId = new("T1")
		o.AlertExpiresAt = timestamppb.New(now.Add(time.Minute))
		o.AlertDelivered = new(true)
		o.Read = true
	})
	change("suppressed alert", true, func(o *notificationv1.NotificationOccurrence) {
		o.AlertExpiresAt = timestamppb.New(now.Add(time.Minute))
		o.AlertDelivered = new(false)
	})
	change("reaction emoji", true, func(o *notificationv1.NotificationOccurrence) {
		o.Signal = testNotificationSignal(notificationTestSignalReaction, "R1", "M1")
		o.GetSignal().GetReactionReceived().Emoji = "👍"
	})
	change("source is the message", true, func(o *notificationv1.NotificationOccurrence) { o.SourceEventId = "M1" })
	change("no actor", true, func(o *notificationv1.NotificationOccurrence) { o.ActorId = "" })
	change("role names", false, func(o *notificationv1.NotificationOccurrence) {
		o.Signal = testNotificationSignal(notificationTestSignalRoleMention, "R1", "M1")
		o.GetSignal().GetRoleMentionReceived().RoleNames = []string{"admins"}
	})
	change("room invitation", false, func(o *notificationv1.NotificationOccurrence) {
		o.Signal = &notificationv1.NotificationSignal{Kind: &notificationv1.NotificationSignal_RoomInvitationReceived{
			RoomInvitationReceived: &notificationv1.RoomInvitationReceived{RoomId: "R1", InvitationId: "I1", ExpiresAt: timestamppb.New(now)},
		}}
	})
	change("unsupported signal", false, func(o *notificationv1.NotificationOccurrence) { o.Signal = testUnsupportedNotificationSignal() })
	change("empty thread root", true, func(o *notificationv1.NotificationOccurrence) {
		notificationSignalMessage(o.GetSignal()).ThreadRootEventId = new("")
	})
	change("no source time", false, func(o *notificationv1.NotificationOccurrence) { o.SourceCreatedAt = nil })
	change("expiry after the int64 nanosecond range", false, func(o *notificationv1.NotificationOccurrence) {
		o.ExpiresAt = timestamppb.New(time.Date(3000, 1, 1, 0, 0, 0, 0, time.UTC))
	})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rows := newNotificationRows(nil)
			want := proto.Clone(test.occurrence).(*notificationv1.NotificationOccurrence)
			row := rows.add(test.occurrence)
			if got := row.full == nil; got != test.compact {
				t.Fatalf("compact = %v, want %v", got, test.compact)
			}
			if got := rows.occurrence(row); !proto.Equal(got, want) {
				t.Fatalf("occurrence = %v, want %v", got, want)
			}
			// The store keeps no reference to its input.
			test.occurrence.Reset()
			if got := rows.occurrence(row); !proto.Equal(got, want) {
				t.Fatalf("occurrence after the input changed = %v, want %v", got, want)
			}
			_, scoped := notificationOccurrenceReadScope(want)
			scope := notificationReadBoundaryScope{userID: "U1", roomID: "R1", threadRootEventID: notificationSignalMessage(want.GetSignal()).GetThreadRootEventId()}
			if got := len(rows.scopeRows(scope)) == 1; got != scoped {
				t.Fatalf("row in scope index = %v, want %v", got, scoped)
			}
			// Only a message ID can enter the append-only event ID table.
			if _, interned := rows.eventIDs.Lookup(want.GetSourceEventId()); interned != (want.GetSourceEventId() == "M1") {
				t.Fatalf("source event ID %q interned = %v", want.GetSourceEventId(), interned)
			}
			rows.remove(row)
			if len(rows.byID)+len(rows.byUser)+len(rows.byScope) != 0 {
				t.Fatalf("remove left index entries: %+v", rows)
			}
		})
	}
}

// TestNotificationProjectionPrunesAfterSkippedScans checks that the scan that
// expiry skips before the earliest deadline still runs at each deadline,
// including after a restore.
func TestNotificationProjectionPrunesAfterSkippedScans(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	p := NewNotificationProjection(nil)
	p.now = func() time.Time { return now }
	for i, row := range []struct {
		id      string
		expires time.Duration
	}{{"late", 3 * time.Hour}, {"early", time.Hour}, {"removed", 2 * time.Hour}} {
		occurrence := &notificationv1.NotificationOccurrence{
			Id: row.id, RecipientId: "U1", SourceEventId: row.id, SourceCreatedAt: timestamp(now),
			Signal: testNotificationSignal(notificationTestSignalReply, "R1", row.id),
		}
		if err := p.Apply(notificationSignalledEvent("event-"+row.id, occurrence, now.Add(row.expires)), uint64(i+1)); err != nil {
			t.Fatal(err)
		}
	}
	removed := &notificationv1.NotificationEvent{
		Id: "event-removal", RecipientId: "U1", NotificationId: "removed", ExpiresAt: timestamp(now.Add(2 * time.Hour)),
		Event: &notificationv1.NotificationEvent_Removed{Removed: &notificationv1.NotificationRemoved{}},
	}
	if err := p.Apply(removed, 4); err != nil {
		t.Fatal(err)
	}
	snapshot, err := p.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	restored := NewNotificationProjection(nil)
	restored.now = p.now
	if err := restored.Restore(snapshot); err != nil {
		t.Fatal(err)
	}

	for name, projection := range map[string]*NotificationProjection{"replayed": p, "restored": restored} {
		t.Run(name, func(t *testing.T) {
			check := func(after time.Duration, wantUsers, wantOccurrences, wantTombstones int) {
				t.Helper()
				users := projection.pruneExpired(now.Add(after))
				if len(users) != wantUsers || len(projection.rows.byID) != wantOccurrences || len(projection.tombstones) != wantTombstones {
					t.Fatalf("after %v: users=%v occurrences=%d tombstones=%d, want %d, %d, %d",
						after, users, len(projection.rows.byID), len(projection.tombstones), wantUsers, wantOccurrences, wantTombstones)
				}
			}
			check(time.Hour-time.Nanosecond, 0, 2, 1)
			check(time.Hour, 1, 1, 1)
			check(3*time.Hour, 1, 0, 1)
			check(2*time.Hour+notificationPhysicalCleanupGrace-time.Nanosecond, 0, 0, 1)
			check(2*time.Hour+notificationPhysicalCleanupGrace, 0, 0, 0)
		})
	}
}
