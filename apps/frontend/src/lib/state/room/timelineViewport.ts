/**
 * Where the room view shows each timeline, in scroll coordinates.
 *
 * A `MessagesStore` keeps only the anchor event, so that a reset reloads its
 * window around the event that the user reads. The pixel offset of that event
 * stays here, next to the store, and returns with the store's recovery anchor.
 */

import { ReactiveSet } from '@chatto/client/reactivity';
import type { MessagesStore } from '@chatto/client/room/messages/MessagesStore';

/** The anchor event and its offset from the top of the viewport. */
export type TimelineViewport = { eventId: string; offset: number; hasNewer?: boolean };

const offsets = new WeakMap<MessagesStore, number>();

/**
 * Record the mounted viewport of a timeline. Null means it follows the latest
 * event. The store ignores the position while it loads or recovers.
 */
export function setTimelineViewport(
  store: MessagesStore,
  position: { eventId: string; offset: number } | null
): void {
  if (!store.setAnchor(position?.eventId ?? null)) return;
  if (position) offsets.set(store, position.offset);
  else offsets.delete(store);
}

/** Forget the viewport, for example when the timeline leaves the mounted view. */
export function clearTimelineViewport(store: MessagesStore): void {
  store.clearAnchor();
  offsets.delete(store);
}

/** The viewport to restore after a reset, or null. Reactive through the store. */
export function recoveryViewport(store: MessagesStore): TimelineViewport | null {
  const anchor = store.recoveryAnchor;
  return anchor ? { ...anchor, offset: offsets.get(store) ?? 0 } : null;
}

/** Wait for the next animation frame, or the next task outside a browser. */
function nextFrame(): Promise<void> {
  return new Promise((resolve) => {
    if (typeof requestAnimationFrame === 'function') requestAnimationFrame(() => resolve());
    else setTimeout(resolve, 0);
  });
}

/** Stores whose older page loaded and whose list settles for one frame. Reactive. */
const settling = new ReactiveSet<MessagesStore>();

/**
 * Whether the view loads an older page of `store`: while the store reads it,
 * and one frame after, while the virtualized list settles. A virtualized list
 * keeps its scroll position across prepended rows while this is true.
 * Reactive.
 */
export function isLoadingOlder(store: MessagesStore): boolean {
  return store.isLoadingMore || settling.has(store);
}

/**
 * Load the next older page, then wait one frame so a virtualized list can
 * settle before the next page. {@link isLoadingOlder} stays true until then.
 */
export async function loadOlder(store: MessagesStore): Promise<void> {
  if (settling.has(store) || store.isLoadingMore) return;
  // Mark before the read, so the flag stays true from the first prepended
  // row until the list settled.
  settling.add(store);
  try {
    await store.loadMore();
    await nextFrame();
  } finally {
    settling.delete(store);
  }
}
