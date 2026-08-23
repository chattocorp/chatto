import {
  isMessagePostedEvent,
  type TimelineEventView
} from '$lib/render/timelineEvents';
import type { RecentThreadRootCandidate } from '$lib/components/composer/messageComposerState.svelte';

export const RECENT_THREAD_ROOT_WINDOW_MS = 5 * 60 * 1000;

/**
 * Returns the current user's latest root when it is still recent and has a
 * thread. An older threaded root does not qualify when the user's newer root
 * is flat: the safeguard is specifically about continuing their last thought.
 */
export function recentThreadRootCandidate(
  events: readonly TimelineEventView[],
  roomId: string,
  currentUserId: string,
  now: number
): RecentThreadRootCandidate | null {
  if (!roomId || !currentUserId || !Number.isFinite(now)) return null;

  for (let index = events.length - 1; index >= 0; index -= 1) {
    const event = events[index];
    if (!event || event.actorId !== currentUserId || !isMessagePostedEvent(event.event)) continue;
    if (event.event.roomId !== roomId || event.event.threadRootEventId) continue;

    const createdAt = Date.parse(event.createdAt);
    const age = now - createdAt;
    if (
      !Number.isFinite(createdAt) ||
      age < 0 ||
      age > RECENT_THREAD_ROOT_WINDOW_MS ||
      event.event.deletedAt ||
      !event.event.threadExists
    ) {
      return null;
    }
    return { threadRootEventId: event.id };
  }

  return null;
}
