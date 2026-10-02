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
 * An end before or after `container` maps to the first or last item. Returns null when
 * both ends are outside `container`, for example for Select All, or when an end inside
 * `container` is not in an item.
 */
export function selectionEndpointKeys(
  selection: Selection | null,
  container: Element,
  items: readonly VirtualItem[]
): TimelineSelectionKeys | null {
  if (!selection?.anchorNode || !selection.focusNode || items.length === 0) return null;
  const anchorInside = container.contains(selection.anchorNode);
  const focusInside = container.contains(selection.focusNode);
  if (!anchorInside && !focusInside) return null;

  const anchor = endpointKey(selection.anchorNode, container, items);
  const focus = endpointKey(selection.focusNode, container, items);
  return anchor && focus ? { anchor, focus } : null;
}

function endpointKey(node: Node, container: Element, items: readonly VirtualItem[]) {
  if (!container.contains(node)) {
    const before = container.compareDocumentPosition(node) & Node.DOCUMENT_POSITION_PRECEDING;
    return (before ? items[0] : items[items.length - 1]).key;
  }
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
