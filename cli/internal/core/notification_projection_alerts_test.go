package core

import (
	"fmt"
	"slices"
	"testing"
	"time"

	notificationv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/notification/v1"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// dueAlertsByFullCopy is the selection that reconcileExpired made before
// dueAlertOccurrences: a copy of every occurrence, filtered on the copies.
func dueAlertsByFullCopy(p *NotificationProjection, now time.Time) []*notificationv1.NotificationOccurrence {
	var result []*notificationv1.NotificationOccurrence
	for _, occurrence := range p.allOccurrences(now) {
		if !NotificationAlertPending(occurrence) {
			continue
		}
		deadline := NotificationAlertDeadline(occurrence)
		if !deadline.IsZero() && now.Before(deadline) {
			continue
		}
		result = append(result, occurrence)
	}
	return result
}

func sortedNotificationIDs(occurrences []*notificationv1.NotificationOccurrence) []string {
	ids := make([]string, 0, len(occurrences))
	for _, occurrence := range occurrences {
		ids = append(ids, occurrence.GetId())
	}
	slices.Sort(ids)
	return ids
}

func TestNotificationProjectionDueAlertOccurrences(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 500, time.UTC)
	projection := NewNotificationProjection(nil)
	projection.now = func() time.Time { return now }
	sequence := uint64(0)
	apply := func(event *notificationv1.NotificationEvent) {
		t.Helper()
		sequence++
		event.Id = fmt.Sprintf("nte_%d", sequence)
		event.RecipientId = "U1"
		event.OccurredAt = timestamppb.New(now)
		event.ExpiresAt = timestamppb.New(now.Add(24 * time.Hour))
		if err := projection.Apply(event, sequence); err != nil {
			t.Fatal(err)
		}
	}
	signal := func(id string, alertExpiresAt *timestamppb.Timestamp, kind *notificationv1.NotificationSignal) {
		apply(&notificationv1.NotificationEvent{NotificationId: id, Event: &notificationv1.NotificationEvent_Signalled{
			Signalled: &notificationv1.NotificationSignalled{
				SourceEventId: "E" + id, SourceCreatedAt: timestamppb.New(now), Signal: kind, AlertExpiresAt: alertExpiresAt,
			},
		}})
	}
	message := func(id string) *notificationv1.NotificationSignal {
		return testNotificationSignal(notificationTestSignalDirectMessage, "R1", "M"+id)
	}

	signal("past", timestamppb.New(now.Add(-time.Second)), message("past"))
	signal("at_deadline", timestamppb.New(now), message("at_deadline"))
	signal("one_ns_before_deadline", timestamppb.New(now.Add(time.Nanosecond)), message("one_ns_before_deadline"))
	signal("future", timestamppb.New(now.Add(time.Minute)), message("future"))
	signal("no_alert", nil, message("no_alert"))
	signal("read", timestamppb.New(now.Add(-time.Second)), message("read"))
	apply(&notificationv1.NotificationEvent{NotificationId: "read", Event: &notificationv1.NotificationEvent_Read{Read: &notificationv1.NotificationRead{}}})
	signal("resolved", timestamppb.New(now.Add(-time.Second)), message("resolved"))
	apply(&notificationv1.NotificationEvent{NotificationId: "resolved", Event: &notificationv1.NotificationEvent_AlertResolved{
		AlertResolved: &notificationv1.NotificationAlertResolved{},
	}})
	// An unsupported signal kind is stored as a full row.
	signal("full_past", timestamppb.New(now.Add(-time.Second)), testUnsupportedNotificationSignal())
	signal("full_future", timestamppb.New(now.Add(time.Minute)), testUnsupportedNotificationSignal())
	// An invalid deadline gives a zero NotificationAlertDeadline, which is due.
	signal("full_invalid_deadline", &timestamppb.Timestamp{Seconds: 1, Nanos: -1}, message("full_invalid_deadline"))
	if row := projection.rows.byID["full_past"]; row == nil || row.full == nil {
		t.Fatal("unsupported signal is not a full row")
	}
	if row := projection.rows.byID["full_invalid_deadline"]; row == nil || row.full == nil {
		t.Fatal("invalid alert deadline is not a full row")
	}

	got := projection.dueAlertOccurrences(now)
	want := []string{"at_deadline", "full_invalid_deadline", "full_past", "past"}
	if ids := sortedNotificationIDs(got); !slices.Equal(ids, want) {
		t.Fatalf("due alerts = %v, want %v", ids, want)
	}
	for _, occurrence := range got {
		if row := projection.rows.byID[occurrence.GetId()]; !proto.Equal(occurrence, projection.rows.occurrence(row)) {
			t.Fatalf("due alert %s is not the stored occurrence: %v", occurrence.GetId(), occurrence)
		}
	}
	if ids := sortedNotificationIDs(dueAlertsByFullCopy(projection, now)); !slices.Equal(ids, want) {
		t.Fatalf("full-copy selection = %v, want %v", ids, want)
	}

	later := now.Add(time.Minute)
	want = []string{"at_deadline", "full_future", "full_invalid_deadline", "full_past", "future", "one_ns_before_deadline", "past"}
	if ids := sortedNotificationIDs(projection.dueAlertOccurrences(later)); !slices.Equal(ids, want) {
		t.Fatalf("due alerts a minute later = %v, want %v", ids, want)
	}
}

// TestNotificationProjectionDueAlertOccurrencesMatchFullCopy compares the
// selection with the full-copy selection on the benchmark fixture at times
// before, during, and after the alert deadlines, and after expiry.
func TestNotificationProjectionDueAlertOccurrencesMatchFullCopy(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	const count = 2_000
	records := newNotificationBenchmarkFixture(t, count, now)
	for _, at := range []time.Time{
		now.Add(-count * time.Second), now.Add(-30 * time.Minute), now, now.Add(time.Hour), now.Add(91 * 24 * time.Hour),
	} {
		t.Run(at.Format(time.RFC3339), func(t *testing.T) {
			left := replayNotificationBenchmarkFixture(t, records, now)
			right := replayNotificationBenchmarkFixture(t, records, now)
			got := left.dueAlertOccurrences(at)
			want := dueAlertsByFullCopy(right, at)
			if !slices.Equal(sortedNotificationIDs(got), sortedNotificationIDs(want)) {
				t.Fatalf("due alerts differ: got %d, want %d", len(got), len(want))
			}
			if len(left.rows.byID) != len(right.rows.byID) || len(left.tombstones) != len(right.tombstones) {
				t.Fatalf("pruning differs: rows %d/%d, tombstones %d/%d",
					len(left.rows.byID), len(right.rows.byID), len(left.tombstones), len(right.tombstones))
			}
		})
	}
}

// TestNotificationProjectionDueAlertOccurrencesDoesNotCopyEachNotification
// checks that a pass with no due alert does not allocate, however many
// notifications the projection holds.
func TestNotificationProjectionDueAlertOccurrencesDoesNotCopyEachNotification(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	const count = 2_000
	projection := replayNotificationBenchmarkFixture(t, newNotificationBenchmarkFixture(t, count, now), now)
	// Before the first fixture alert deadline, no alert is due.
	before := now.Add(-count * time.Second)
	if due := projection.dueAlertOccurrences(before); len(due) != 0 {
		t.Fatalf("due alerts = %d, want 0", len(due))
	}
	if allocs := testing.AllocsPerRun(10, func() { projection.dueAlertOccurrences(before) }); allocs != 0 {
		t.Fatalf("allocations per pass = %v, want 0", allocs)
	}
}

// BenchmarkNotificationAlertReconcileScan compares one reconcileExpired
// selection with no due alert against the full copy that it replaced.
func BenchmarkNotificationAlertReconcileScan(b *testing.B) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, count := range []int{10_000, 50_000} {
		projection := replayNotificationBenchmarkFixture(b, newNotificationBenchmarkFixture(b, count, now), now)
		before := now.Add(-time.Duration(count) * time.Second)
		b.Run(fmt.Sprintf("due_alerts/occurrences_%d", count), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				projection.dueAlertOccurrences(before)
			}
		})
		b.Run(fmt.Sprintf("full_copy/occurrences_%d", count), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				dueAlertsByFullCopy(projection, before)
			}
		})
	}
}
