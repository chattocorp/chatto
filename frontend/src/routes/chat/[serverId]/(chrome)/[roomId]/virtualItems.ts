import type { RoomEventViewFragment } from '$lib/gql/graphql';
import type { EventWithMeta } from './messageGrouping';
import { isEventHidden } from './messageGrouping';

/**
 * A discriminated union representing items in the virtual list.
 * Day separators, unread separators, and the start-of-conversation marker
 * are their own items so that virtua can manage them as discrete entries
 * with measured heights.
 */
export type VirtualItem =
  | { type: 'start-marker'; key: string }
  | { type: 'day-separator'; key: string; label: string }
  | { type: 'unread-separator'; key: string }
  | { type: 'event'; key: string; event: RoomEventViewFragment; isFirstInGroup: boolean };

/**
 * Transform grouped event metadata into a flat array for the virtualizer.
 * Inserts the start-of-conversation marker, day separators, and the unread
 * separator as their own items.
 */
export function buildVirtualItems(
  eventsWithMeta: EventWithMeta[],
  firstUnreadEventId: string | null,
  hasReachedStart: boolean
): VirtualItem[] {
  const items: VirtualItem[] = [];

  if (hasReachedStart && eventsWithMeta.length > 0) {
    items.push({ type: 'start-marker', key: 'start-marker' });
  }

  for (const item of eventsWithMeta) {
    const { event, isFirstInGroup, showDaySeparator, dayLabel } = item;
    const hidden = isEventHidden(event);

    // Don't emit day/unread separators for hidden (deleted) events
    if (showDaySeparator && !hidden) {
      items.push({
        type: 'day-separator',
        key: `day-${event.id}`,
        label: dayLabel
      });
    }

    if (firstUnreadEventId !== null && event.id === firstUnreadEventId && !hidden) {
      items.push({
        type: 'unread-separator',
        key: 'unread-separator'
      });
    }

    items.push({
      type: 'event',
      key: event.id,
      event,
      isFirstInGroup
    });
  }

  return items;
}
