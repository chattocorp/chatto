package core

import (
	"fmt"
	"testing"
	"time"

	notificationv1 "hmans.de/chatto/internal/pb/chatto/core/notification/v1"
)

func TestNotificationUnreadScopeIsolationAndExpiry(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Millisecond)
	p := NewNotificationProjection()
	p.now = func() time.Time { return now }
	for i, row := range []struct {
		id, user, room, thread string
		read                   bool
		expires                time.Duration
	}{
		{"wanted", "U1", "R1", "", false, time.Hour},
		{"read", "U1", "R1", "", true, time.Hour},
		{"expired", "U1", "R1", "", false, time.Second},
		{"boundary", "U1", "R1", "", false, time.Minute},
		{"room", "U1", "R2", "", false, time.Hour},
		{"user", "U2", "R1", "", false, time.Hour},
		{"thread", "U1", "R1", "T1", false, time.Hour},
	} {
		signal := testNotificationSignal(notificationTestSignalDirectMention, row.room, row.id)
		if row.thread != "" {
			notificationSignalMessage(signal).ThreadRootEventId = new(row.thread)
		}
		occurrence := &notificationv1.NotificationOccurrence{
			Id: row.id, RecipientId: row.user, SourceEventId: row.id,
			SourceCreatedAt: timestamp(now), Signal: signal, Read: row.read,
		}
		if err := p.Apply(notificationSignalledEvent("event-"+row.id, occurrence, now.Add(row.expires)), uint64(i+1)); err != nil {
			t.Fatal(err)
		}
	}
	queryTime := now.Add(time.Minute)
	for _, scope := range []notificationReadBoundaryScope{
		{userID: "U1", roomID: "R1"},
		{userID: "U1", roomID: "R1", threadRootEventID: "T1"},
	} {
		want := "wanted"
		if scope.threadRootEventID != "" {
			want = "thread"
		}
		got := p.unreadScopeOccurrences(scope, queryTime)
		if len(got) != 1 || got[0].GetId() != want {
			t.Fatalf("scope %+v returned %+v, want %s", scope, got, want)
		}
		got[0].Read = true
		notificationSignalMessage(got[0].Signal).RoomId = "changed"
		again := p.unreadScopeOccurrences(scope, queryTime)
		if len(again) != 1 || notificationSignalMessage(again[0].Signal).RoomId != "R1" {
			t.Fatal("caller changed projected notification state")
		}
	}
	if got := p.unreadScopeOccurrences(notificationReadBoundaryScope{userID: "missing", roomID: "R1"}, queryTime); len(got) != 0 {
		t.Fatalf("unknown recipient returned %+v", got)
	}
	// Scoped reads enforce expiry without taking over global maintenance.
	if _, exists := p.byID["expired"]; !exists {
		t.Fatal("scoped read performed global expiry cleanup")
	}
}

// BenchmarkNotificationReadScope compares the former send-path lookup with
// the scoped lookup while unrelated server history grows. Setup is isolated
// from a running core and bypasses replay to measure only request-path reads.
func BenchmarkNotificationReadScope(b *testing.B) {
	for _, count := range []int{1000, 10000, 100000} {
		b.Run(fmt.Sprintf("retained=%d", count), func(b *testing.B) {
			now := time.Now().UTC()
			p := NewNotificationProjection()
			for i := range count {
				id := fmt.Sprintf("N%d", i)
				user := fmt.Sprintf("U%d", i%2000)
				occurrence := &notificationv1.NotificationOccurrence{
					Id: id, RecipientId: user, ExpiresAt: timestamp(now.Add(time.Hour)),
					Signal: testNotificationSignal(notificationTestSignalDirectMention, "unrelated-room", id),
				}
				p.byID[id] = occurrence
				if p.idsByUser[user] == nil {
					p.idsByUser[user] = make(map[string]struct{})
				}
				p.idsByUser[user][id] = struct{}{}
				p.addScopeLocked(occurrence)
			}
			target := &notificationv1.NotificationOccurrence{
				Id: "target", RecipientId: "U0", SourceEventId: "target",
				SourceCreatedAt: timestamp(now),
				Signal:          testNotificationSignal(notificationTestSignalDirectMention, "target-room", "target"),
			}
			if err := p.Apply(notificationSignalledEvent("target-event", target, now.Add(time.Hour)), uint64(count+1)); err != nil {
				b.Fatal(err)
			}
			b.Run("user_lookup_before_sort", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					p.userOccurrences("U0", now)
				}
			})
			b.Run("unread_scope_lookup", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					p.unreadScopeOccurrences(notificationReadBoundaryScope{userID: "U0", roomID: "target-room"}, now)
				}
			})
		})
	}
}
