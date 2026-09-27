import type { VirtualizerHandle } from 'virtua/svelte';
import type { GroupedListItem } from './groupedListItems';

/**
 * A visible entry and its distance from the top of the scroll viewport. The
 * group ID lets the restore step skip a row that moved to another group,
 * because that row is no longer where the reader was looking.
 */
export interface ScrollAnchor {
  key: string;
  groupId: string;
  top: number;
}

/** Attribute that carries each mounted entry's key, so anchors can be found in the DOM. */
export const ANCHOR_KEY_ATTRIBUTE = 'data-virtual-key';

/** Attribute that carries each mounted entry's group ID. */
export const ANCHOR_GROUP_ATTRIBUTE = 'data-room-group-id';

const MAX_ANCHORS = 8;

type AnchorHandle = Pick<VirtualizerHandle, 'getItemOffset'>;

function groupIdOf<T extends { id: string }>(entry: GroupedListItem<T>): string {
  return entry.type === 'row' ? entry.groupId : entry.group.id;
}

function viewportTop(scrollElement: HTMLElement): number {
  return scrollElement.getBoundingClientRect().top;
}

function mountedEntry(listElement: HTMLElement, key: string): HTMLElement | null {
  return listElement.querySelector<HTMLElement>(`[${ANCHOR_KEY_ATTRIBUTE}="${CSS.escape(key)}"]`);
}

/**
 * Records the mounted entries whose top edge is inside the viewport, from the
 * top down. The virtualizer mounts entries in list order, so the first matches
 * in the DOM are the topmost. The virtualizer opts out of browser scroll
 * anchoring, so without this an entry inserted above the viewport moves
 * everything the reader sees.
 *
 * Returns no anchors at the top of the list, so new entries at the top stay
 * visible, as with browser scroll anchoring.
 */
export function captureScrollAnchors(
  listElement: HTMLElement,
  scrollElement: HTMLElement
): ScrollAnchor[] {
  if (scrollElement.scrollTop <= 0) return [];

  const top = viewportTop(scrollElement);
  const height = scrollElement.clientHeight;
  const anchors: ScrollAnchor[] = [];
  for (const element of listElement.querySelectorAll<HTMLElement>(`[${ANCHOR_KEY_ATTRIBUTE}]`)) {
    const offset = element.getBoundingClientRect().top - top;
    if (offset >= height) break;
    const key = element.getAttribute(ANCHOR_KEY_ATTRIBUTE);
    const groupId = element.getAttribute(ANCHOR_GROUP_ATTRIBUTE);
    if (offset < 0 || !key || groupId === null) continue;
    anchors.push({ key, groupId, top: offset });
    if (anchors.length === MAX_ANCHORS) break;
  }
  return anchors;
}

/**
 * Picks the first anchor that still exists in the same group, or null when
 * none survives, for example after a search replaces the list.
 */
export function selectScrollAnchor<T extends { id: string }>(
  entries: readonly GroupedListItem<T>[],
  anchors: readonly ScrollAnchor[]
): { anchor: ScrollAnchor; index: number } | null {
  if (anchors.length === 0) return null;
  const indexByKey = new Map(entries.map((entry, index) => [entry.key, index]));
  for (const anchor of anchors) {
    const index = indexByKey.get(anchor.key);
    if (index !== undefined && groupIdOf(entries[index]) === anchor.groupId) {
      return { anchor, index };
    }
  }
  return null;
}

/**
 * Scrolls so the anchor is again `anchor.top` pixels below the top of the
 * viewport. Measures the mounted entry when it exists. Otherwise it estimates
 * the position from the virtualizer, which assumes that the list starts at the
 * top of the scroll content. Returns whether the anchor was mounted.
 *
 * Adjusts the scroll element directly. The virtualizer's `scrollTo` holds a
 * fixed target while entries are measured, which discards its own correction
 * when an entry above changes size.
 */
export function alignScrollAnchor(
  handle: AnchorHandle,
  listElement: HTMLElement,
  scrollElement: HTMLElement,
  anchor: ScrollAnchor,
  index: number
): boolean {
  const element = mountedEntry(listElement, anchor.key);
  const currentTop = element
    ? element.getBoundingClientRect().top - viewportTop(scrollElement)
    : handle.getItemOffset(index) - scrollElement.scrollTop;
  const difference = currentTop - anchor.top;
  if (Math.abs(difference) >= 1) scrollElement.scrollTop += difference;
  return element !== null;
}
