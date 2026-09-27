import type { VirtualizerHandle } from 'virtua/svelte';
import type { GroupedListItem } from './groupedListItems';

/**
 * A visible entry and its distance from the scroll position. The group ID lets
 * the restore step skip a row that moved to another group, because that row is
 * no longer where the reader was looking.
 */
export interface ScrollAnchor {
  key: string;
  groupId: string;
  delta: number;
}

type AnchorHandle = Pick<
  VirtualizerHandle,
  'getScrollOffset' | 'getViewportSize' | 'findItemIndex' | 'getItemOffset' | 'scrollTo'
>;

const MAX_ANCHORS = 8;

function groupIdOf<T extends { id: string }>(entry: GroupedListItem<T>): string {
  return entry.type === 'row' ? entry.groupId : entry.group.id;
}

/**
 * Records the entries visible before a data change. The virtualizer opts out of
 * browser scroll anchoring and places entries by index, so without this an
 * entry inserted above the viewport moves everything the reader sees.
 *
 * Returns no anchors at the top of the list, so new entries at the top stay
 * visible, as with browser scroll anchoring.
 */
export function captureScrollAnchors<T extends { id: string }>(
  handle: AnchorHandle,
  entries: readonly GroupedListItem<T>[]
): ScrollAnchor[] {
  const offset = handle.getScrollOffset();
  if (entries.length === 0 || offset <= 0) return [];

  const viewportEnd = offset + handle.getViewportSize();
  const anchors: ScrollAnchor[] = [];
  for (
    let index = handle.findItemIndex(offset);
    index < entries.length && anchors.length < MAX_ANCHORS;
    index++
  ) {
    const itemOffset = handle.getItemOffset(index);
    if (itemOffset >= viewportEnd) break;
    const entry = entries[index];
    anchors.push({ key: entry.key, groupId: groupIdOf(entry), delta: offset - itemOffset });
  }
  return anchors;
}

/**
 * Scrolls so the first anchor that still exists in the same group keeps its
 * distance from the scroll position. Does nothing when no anchor survives, for
 * example after a search replaces the list.
 */
export function restoreScrollAnchor<T extends { id: string }>(
  handle: AnchorHandle,
  entries: readonly GroupedListItem<T>[],
  anchors: readonly ScrollAnchor[]
): void {
  if (anchors.length === 0) return;

  const indexByKey = new Map(entries.map((entry, index) => [entry.key, index]));
  for (const anchor of anchors) {
    const index = indexByKey.get(anchor.key);
    if (index === undefined || groupIdOf(entries[index]) !== anchor.groupId) continue;

    const target = handle.getItemOffset(index) + anchor.delta;
    if (Math.abs(target - handle.getScrollOffset()) >= 1) handle.scrollTo(target);
    return;
  }
}
