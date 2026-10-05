import type { VirtualItem } from './virtualItems';

/**
 * The attribute that marks the element of a rendered timeline item. Its value is the
 * item's `VirtualItem.key`.
 */
export const TIMELINE_ITEM_KEY_ATTRIBUTE = 'data-timeline-key';

/** The keys of the timeline items that contain the two ends of the document selection. */
export type TimelineSelectionKeys = { anchor: string; focus: string };

/**
 * Finds the timeline items at the ends of the document selection.
 *
 * The virtualizer unmounts items outside the viewport, and a copy contains only mounted
 * DOM. The caller keeps every item between the returned keys mounted, so a selection
 * across several screens copies completely. A collapsed caret also counts: it keeps the
 * clicked item mounted, so a later Shift+click after a scroll extends from that item.
 *
 * Returns null when an end is not in a timeline item inside `container`, for example for
 * a caret in the composer or for Select All. Such a selection is not kept mounted: an end
 * in another pane would otherwise keep all loaded history between it and the list edge
 * mounted.
 */
export function selectionEndpointKeys(
  selection: Selection | null,
  container: Element
): TimelineSelectionKeys | null {
  if (!selection?.anchorNode || !selection.focusNode) return null;
  const anchor = itemKey(selection.anchorNode, container);
  const focus = itemKey(selection.focusNode, container);
  return anchor && focus ? { anchor, focus } : null;
}

function itemKey(node: Node, container: Element) {
  if (!container.contains(node)) return null;
  const element = node instanceof Element ? node : node.parentElement;
  return element
    ?.closest(`[${TIMELINE_ITEM_KEY_ATTRIBUTE}]`)
    ?.getAttribute(TIMELINE_ITEM_KEY_ATTRIBUTE);
}

/**
 * Returns the indexes of all items from one selection key to the other, inclusive, in
 * ascending order. Returns an empty array when `items` no longer contains a key.
 */
export function keptIndexes(items: readonly VirtualItem[], keys: TimelineSelectionKeys): number[] {
  const anchor = items.findIndex((item) => item.key === keys.anchor);
  const focus = items.findIndex((item) => item.key === keys.focus);
  if (anchor === -1 || focus === -1) return [];

  const start = Math.min(anchor, focus);
  return Array.from({ length: Math.abs(anchor - focus) + 1 }, (_, i) => start + i);
}
