import {
  isMessagePostedEvent,
  type TimelineEventView
} from '@chatto/client/timeline/timelineEvents';
import type { LatestThreadRootCandidate } from '$lib/components/composer/messageComposerState.svelte';

/**
 * Offers the latest room root only when it has a thread. Author and age do not
 * matter. A newer flat or deleted root prevents an older thread from qualifying.
 * Events must be in timeline order, oldest first.
 */
export function latestThreadRootCandidate(
  events: readonly TimelineEventView[],
  roomId: string
): LatestThreadRootCandidate | null {
  if (!roomId) return null;

  for (let index = events.length - 1; index >= 0; index -= 1) {
    const event = events[index];
    if (!event || !isMessagePostedEvent(event.event)) continue;
    if (event.event.roomId !== roomId || event.event.threadRootEventId) continue;

    if (event.event.deletedAt || !event.event.threadExists) return null;
    return { threadRootEventId: event.id };
  }

  return null;
}
