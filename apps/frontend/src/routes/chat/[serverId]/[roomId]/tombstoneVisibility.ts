import { isMessagePostedEvent, type TimelineEventView } from '$lib/render/timelineEvents';
export const MESSAGE_TOMBSTONE_GRACE_MS = 60 * 60 * 1000;

/**
 * Return the finite time when a context-free tombstone becomes hidden.
 * Null means the row is not an expiring tombstone or currently has persistent
 * visible context.
 */
export function tombstoneExpiry(event: TimelineEventView): number | null {
  const message = event.event;
  if (!isMessagePostedEvent(message) || message.body) return null;
  if ((message.attachments?.length ?? 0) > 0 || message.linkPreview) return null;
  if ((message.reactions?.length ?? 0) > 0 || message.replyCount > 0) return null;

  // Removing the final attachment from an attachment-only message is a
  // MessageEditedEvent rather than a retraction. The API preserves its empty
  // body and edit timestamp, which together mark when the row became a
  // context-free tombstone. A null body without deletedAt remains an unknown
  // or corrupt body and is deliberately retained.
  const tombstonedAt = message.deletedAt ?? (message.body === '' ? message.updatedAt : null);
  if (!tombstonedAt) return null;
  const tombstonedAtMs = Date.parse(tombstonedAt);
  if (!Number.isFinite(tombstonedAtMs)) return null;

  return tombstonedAtMs + MESSAGE_TOMBSTONE_GRACE_MS;
}

export function shouldHideTombstone(event: TimelineEventView, nowMs: number): boolean {
  const expiresAt = tombstoneExpiry(event);
  return expiresAt !== null && nowMs >= expiresAt;
}

export function visibleTombstoneEvents(
  events: TimelineEventView[],
  nowMs: number
): TimelineEventView[] {
  return events.filter((event) => !shouldHideTombstone(event, nowMs));
}

export function nextTombstoneExpiry(events: TimelineEventView[], nowMs: number): number | null {
  let next: number | null = null;
  for (const event of events) {
    const expiresAt = tombstoneExpiry(event);
    if (expiresAt === null || expiresAt <= nowMs) continue;
    if (next === null || expiresAt < next) next = expiresAt;
  }
  return next;
}

/**
 * Schedule the next finite tombstone expiry and return a cleanup function.
 * Keeping this lifecycle in a pure helper makes timer replacement and
 * component teardown independently testable.
 */
export function scheduleNextTombstoneExpiry(
  events: TimelineEventView[],
  nowMs: number,
  onExpire: (expiresAt: number) => void
): () => void {
  const expiresAt = nextTombstoneExpiry(events, nowMs);
  if (expiresAt === null) return () => {};

  const timer = setTimeout(() => onExpire(expiresAt), Math.max(0, expiresAt - nowMs));
  return () => clearTimeout(timer);
}

export function visibleUnreadMarkerEventId(
  timelineEvents: TimelineEventView[],
  visibleEvents: TimelineEventView[],
  unreadEventId: string | null
): string | null {
  if (!unreadEventId) return null;
  if (visibleEvents.some((event) => event.id === unreadEventId)) return unreadEventId;

  const markerIndex = timelineEvents.findIndex((event) => event.id === unreadEventId);
  if (markerIndex === -1) return null;
  const visibleIDs = new Set(visibleEvents.map((event) => event.id));
  for (let i = markerIndex + 1; i < timelineEvents.length; i++) {
    if (visibleIDs.has(timelineEvents[i].id)) return timelineEvents[i].id;
  }
  return null;
}
