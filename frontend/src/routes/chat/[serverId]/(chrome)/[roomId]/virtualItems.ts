import type { RoomEventViewFragment } from '$lib/gql/graphql';
import type { EventWithMeta } from './messageGrouping';
import { isEventHidden } from './messageGrouping';

export type SystemGroupKind = 'join' | 'leave';

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
  | { type: 'event'; key: string; event: RoomEventViewFragment; isFirstInGroup: boolean }
  | {
      type: 'system-group';
      key: string;
      kind: SystemGroupKind;
      events: RoomEventViewFragment[];
    };

function getSystemGroupKind(event: RoomEventViewFragment): SystemGroupKind | null {
  switch (event.event?.__typename) {
    case 'UserJoinedRoomEvent':
      return 'join';
    case 'UserLeftRoomEvent':
      return 'leave';
    default:
      return null;
  }
}

/**
 * Transform grouped event metadata into a flat array for the virtualizer.
 * Inserts the start-of-conversation marker, day separators, and the unread
 * separator as their own items.
 *
 * Consecutive `UserJoinedRoomEvent` (or consecutive `UserLeftRoomEvent`)
 * are coalesced into a single `system-group` item. The grouping breaks
 * on: a different event kind, a day separator (timezone-aware via
 * `showDaySeparator`), or the unread separator.
 */
export function buildVirtualItems(
  eventsWithMeta: EventWithMeta[],
  firstUnreadEventId: string | null,
  hasReachedStart: boolean
): VirtualItem[] {
  const items: VirtualItem[] = [];

  let openGroup: {
    kind: SystemGroupKind;
    events: RoomEventViewFragment[];
  } | null = null;

  const flushGroup = () => {
    if (!openGroup) return;
    // Key by the newest event in the group. When pagination prepends older
    // events that merge into this group, the newest event stays the same —
    // virtua keeps the cached height measurement and uses its scroll-shift
    // mechanism instead of treating the merged group as a brand new item.
    const lastId = openGroup.events[openGroup.events.length - 1].id;
    items.push({
      type: 'system-group',
      key: `system-group-${lastId}`,
      kind: openGroup.kind,
      events: openGroup.events
    });
    openGroup = null;
  };

  if (hasReachedStart && eventsWithMeta.length > 0) {
    items.push({ type: 'start-marker', key: 'start-marker' });
  }

  for (const item of eventsWithMeta) {
    const { event, isFirstInGroup, showDaySeparator, dayLabel } = item;
    const hidden = isEventHidden(event);
    const systemKind = getSystemGroupKind(event);

    const hasDaySeparator = showDaySeparator && !hidden;
    const hasUnreadSeparator =
      firstUnreadEventId !== null && event.id === firstUnreadEventId && !hidden;

    // Any separator or a mismatched / non-system event breaks an open group.
    if (
      openGroup &&
      (systemKind !== openGroup.kind || hasDaySeparator || hasUnreadSeparator)
    ) {
      flushGroup();
    }

    if (hasDaySeparator) {
      items.push({
        type: 'day-separator',
        key: `day-${event.id}`,
        label: dayLabel
      });
    }

    if (hasUnreadSeparator) {
      items.push({
        type: 'unread-separator',
        key: 'unread-separator'
      });
    }

    if (systemKind) {
      if (!openGroup) {
        openGroup = { kind: systemKind, events: [] };
      }
      openGroup.events.push(event);
      continue;
    }

    items.push({
      type: 'event',
      key: event.id,
      event,
      isFirstInGroup
    });
  }

  flushGroup();

  return items;
}
