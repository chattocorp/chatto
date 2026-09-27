import { isMessagePostedEvent, type TimelineEventView } from '$lib/render/timelineEvents';

/**
 * Collect the IDs of the other users who take part in a thread.
 *
 * The result contains the authors of the loaded thread messages and the
 * root's participant list, which also covers replies outside the loaded
 * window. It never contains the viewer. The composer ranks these users first
 * in @mention autocomplete.
 */
export function threadParticipantIds(
  threadEvents: readonly TimelineEventView[],
  threadRootEventId: string,
  viewerId: string | null
): ReadonlySet<string> {
  const ids = threadEvents.flatMap((event) => {
    if (!isMessagePostedEvent(event.event)) return [];
    const rootParticipantIds =
      event.id === threadRootEventId
        ? event.event.threadParticipants.map((participant) => participant.id)
        : [];
    return event.actorId ? [event.actorId, ...rootParticipantIds] : rootParticipantIds;
  });
  return new Set(ids.filter((id) => id !== viewerId));
}
