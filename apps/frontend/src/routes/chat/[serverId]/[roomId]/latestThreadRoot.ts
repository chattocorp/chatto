import {
  isMessagePostedEvent,
  type TimelineEventView
} from '@chatto/client/timeline/timelineEvents';
import type { LatestThreadRootCandidate } from '$lib/components/composer/messageComposerState.svelte';

/** The destination prompt applies for 30 minutes after the root was posted. */
const THREAD_PROMPT_MAX_AGE_MS = 30 * 60 * 1000;

/**
 * Offers the latest room root only when it has a thread and is at most 30 minutes
 * old. Call at submission time so an idle draft cannot keep an expired candidate.
 * A newer flat or deleted root prevents an older thread from qualifying.
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
    const age = Date.now() - Date.parse(event.createdAt);
    if (!Number.isFinite(age) || age < 0 || age > THREAD_PROMPT_MAX_AGE_MS) return null;
    return { threadRootEventId: event.id };
  }

  return null;
}
