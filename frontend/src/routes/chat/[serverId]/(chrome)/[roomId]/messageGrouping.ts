import type { RoomEventViewFragment } from '$lib/gql/graphql';
import type { UserSettingsState } from '$lib/state/userSettings.svelte';
import { isSameDay, formatDayLabel } from '$lib/utils/formatTime';

const TEN_MINUTES_MS = 10 * 60 * 1000;

/**
 * Mirrors the isHidden logic in MessageEvent.svelte: a deleted message
 * (no body, no attachments) with no reactions and no replies is hidden.
 */
export function isEventHidden(event: RoomEventViewFragment): boolean {
  const e = event.event;
  if (!e) return false;
  if (e.__typename !== 'MessagePostedEvent') return false;
  const hasBody = !!e.body;
  const hasAttachments = (e.attachments?.length ?? 0) > 0;
  if (hasBody || hasAttachments) return false;
  // Echoes don't have reactions/replyCount — if body+attachments are gone, always hidden.
  if (e.echoOfEventId != null) return true;
  const hasReactions = (e.reactions?.length ?? 0) > 0;
  const hasReplies = (e.replyCount ?? 0) > 0;
  return !hasReactions && !hasReplies;
}

export type EventWithMeta = {
  event: RoomEventViewFragment;
  isFirstInGroup: boolean;
  showDaySeparator: boolean;
  dayLabel: string;
};

export function computeEventMetadata(
  events: RoomEventViewFragment[],
  settings: UserSettingsState
): EventWithMeta[] {
  const result: EventWithMeta[] = [];

  for (let i = 0; i < events.length; i++) {
    const event = events[i];

    // Find the last visible previous event (skip hidden deleted messages)
    let prevEvent: RoomEventViewFragment | null = null;
    for (let j = i - 1; j >= 0; j--) {
      if (!isEventHidden(events[j])) {
        prevEvent = events[j];
        break;
      }
    }

    const eventDate = new Date(event.createdAt);
    const prevEventDate = prevEvent ? new Date(prevEvent.createdAt) : null;

    // Check if we need a day separator (timezone-aware)
    const showDaySeparator = !prevEventDate || !isSameDay(eventDate, prevEventDate, settings);
    const dayLabel = showDaySeparator ? formatDayLabel(eventDate, settings) : '';

    // Determine if this is the first message in a group
    let isFirstInGroup = true;

    if (prevEvent && !showDaySeparator) {
      const timeDiff = eventDate.getTime() - prevEventDate!.getTime();
      const sameActor = event.actorId === prevEvent.actorId;
      const withinTimeWindow = timeDiff <= TEN_MINUTES_MS;
      const isMessage = (t?: string) => t === 'MessagePostedEvent';
      const bothAreMessages =
        isMessage(event.event?.__typename) && isMessage(prevEvent.event?.__typename);
      const isReply =
        event.event?.__typename === 'MessagePostedEvent' && event.event?.inReplyTo != null;

      // Group if same actor, within 10 minutes, both are messages, and not a reply.
      // Replies always render full (with avatar/name) to show the attribution context.
      if (sameActor && withinTimeWindow && bothAreMessages && !isReply) {
        isFirstInGroup = false;
      }
    }

    result.push({ event, isFirstInGroup, showDaySeparator, dayLabel });
  }

  return result;
}
