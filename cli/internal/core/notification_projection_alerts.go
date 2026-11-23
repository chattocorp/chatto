package core

import (
	"time"

	notificationv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/notification/v1"
)

// dueAlertOccurrences prunes expired state, then returns detached copies of
// the occurrences whose alert is pending (NotificationAlertPending) and whose
// deadline (NotificationAlertDeadline) is zero or not after now.
//
// The scan reads the stored rows and builds a protobuf message only for a
// returned occurrence. Thus, a pass with no due alerts does not allocate for
// each notification. The caller must not hold the projection lock across the
// I/O that it does with the result.
func (p *NotificationProjection) dueAlertOccurrences(now time.Time) []*notificationv1.NotificationOccurrence {
	p.Lock()
	defer p.Unlock()
	p.pruneExpiredLocked(now)
	var result []*notificationv1.NotificationOccurrence
	for _, row := range p.rows.byID {
		if row.alertDue(now) {
			result = append(result, p.rows.occurrence(row))
		}
	}
	return result
}

// alertDue reports, without an allocation, the same result as
// NotificationAlertPending plus an expired NotificationAlertDeadline on the
// occurrence that notificationRows.occurrence builds for the row.
func (r *notificationRow) alertDue(now time.Time) bool {
	// occurrence takes read and alert state from flags, also for full rows.
	if !r.has(notificationRowHasAlertExpiry) || r.has(notificationRowRead) || r.has(notificationRowAlertResolved) {
		return false
	}
	if r.full != nil {
		deadline := NotificationAlertDeadline(r.full)
		return deadline.IsZero() || !now.Before(deadline)
	}
	// A compact row round-trips alert_expires_at exactly through Unix
	// nanoseconds, so the timestamp is valid and this comparison is exact.
	return saturatedUnixNano(now) >= r.alertExpiresAt
}
