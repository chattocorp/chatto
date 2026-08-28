import { isMessagePostedEvent, type TimelineEventView } from '$lib/render/timelineEvents';

/**
 * Return whether a confirmed tombstone has no visible context and should be
 * omitted from the timeline.
 */
export function shouldHideTombstone(event: TimelineEventView): boolean {
  const message = event.event;
  if (!isMessagePostedEvent(message) || message.body) return false;
  if ((message.attachments?.length ?? 0) > 0 || message.linkPreview) return false;
  if ((message.reactions?.length ?? 0) > 0 || message.replyCount > 0) return false;

  // Removing the final attachment from an attachment-only message is a
  // MessageEditedEvent rather than a retraction. The API preserves its empty
  // body and edit timestamp, which together identify the empty row. A null
  // body without deletedAt remains an unknown or corrupt body and is retained.
  return Boolean(message.deletedAt || (message.body === '' && message.updatedAt));
}

export function visibleTombstoneEvents(events: TimelineEventView[]): TimelineEventView[] {
  return events.filter((event) => !shouldHideTombstone(event));
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
