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
 * top down. The virtualizer opts out of browser scroll anchoring, so without
 * this an entry inserted above the viewport moves everything the reader sees.
 *
 * Returns no anchors at the top of the list, so new entries at the top stay
 * visible, as with browser scroll anchoring.
 */
export function captureScrollAnchors<T extends { id: string }>(
  listElement: HTMLElement,
  scrollElement: HTMLElement,
  entries: readonly GroupedListItem<T>[]
): ScrollAnchor[] {
  if (entries.length === 0 || scrollElement.scrollTop <= 0) return [];

  const top = viewportTop(scrollElement);
  const bottom = top + scrollElement.clientHeight;
  const groupByKey = new Map(entries.map((entry) => [entry.key, groupIdOf(entry)]));
  return Array.from(listElement.querySelectorAll<HTMLElement>(`[${ANCHOR_KEY_ATTRIBUTE}]`))
    .map((element) => ({
      key: element.getAttribute(ANCHOR_KEY_ATTRIBUTE) ?? '',
      top: element.getBoundingClientRect().top - top
    }))
    .filter((anchor) => anchor.top >= 0 && anchor.top < bottom - top)
    .sort((left, right) => left.top - right.top)
    .slice(0, MAX_ANCHORS)
    .flatMap((anchor) => {
      const groupId = groupByKey.get(anchor.key);
      return groupId === undefined ? [] : [{ ...anchor, groupId }];
    });
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
