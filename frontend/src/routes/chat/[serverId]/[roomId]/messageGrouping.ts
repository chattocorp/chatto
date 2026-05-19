import type { RoomEventViewFragment } from '$lib/gql/graphql';
import type { UserSettingsState } from '$lib/state/userSettings.svelte';
import { isSameDay, formatDayLabel } from '$lib/utils/formatTime';

const TEN_MINUTES_MS = 10 * 60 * 1000;

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
    const prevEvent: RoomEventViewFragment | null = i > 0 ? events[i - 1] : null;

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
