import { isMessagePostedEvent } from '$lib/render/eventKinds';
import type { RoomEventView } from '$lib/render/types';

export const MESSAGE_TOMBSTONE_GRACE_MS = 60 * 60 * 1000;

/**
 * Return the finite time when a context-free tombstone becomes hidden.
 * Null means the row is not an expiring tombstone or currently has persistent
 * visible context.
 */
export function tombstoneExpiry(event: RoomEventView): number | null {
  const message = event.event;
  if (!isMessagePostedEvent(message) || !message.deletedAt) return null;
  if (message.body != null) return null;
  if ((message.attachments?.length ?? 0) > 0 || message.linkPreview) return null;
  if ((message.reactions?.length ?? 0) > 0 || message.replyCount > 0) return null;

  const deletedAt = Date.parse(message.deletedAt);
  if (!Number.isFinite(deletedAt)) return null;

  return deletedAt + MESSAGE_TOMBSTONE_GRACE_MS;
}

export function shouldHideTombstone(event: RoomEventView, nowMs: number): boolean {
  const expiresAt = tombstoneExpiry(event);
  return expiresAt !== null && nowMs >= expiresAt;
}

export function visibleTombstoneEvents(events: RoomEventView[], nowMs: number): RoomEventView[] {
  return events.filter((event) => !shouldHideTombstone(event, nowMs));
}

export function nextTombstoneExpiry(events: RoomEventView[], nowMs: number): number | null {
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
  events: RoomEventView[],
  nowMs: number,
  onExpire: (expiresAt: number) => void
): () => void {
  const expiresAt = nextTombstoneExpiry(events, nowMs);
  if (expiresAt === null) return () => {};

  const timer = setTimeout(() => onExpire(expiresAt), Math.max(0, expiresAt - nowMs));
  return () => clearTimeout(timer);
}

export function visibleUnreadMarkerEventId(
  timelineEvents: RoomEventView[],
  visibleEvents: RoomEventView[],
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
