import { getContext, setContext } from 'svelte';
import { SvelteSet } from 'svelte/reactivity';

/**
 * Timeline item keys that the virtualizer must keep mounted.
 *
 * A message row renders its own action overlays, such as the emoji picker sheet. When
 * the row leaves the rendered range, the overlay unmounts with it. This happens, for
 * example, when the virtual keyboard opens for the picker search field and the timeline
 * gets shorter. A row pins its key while it owns an open overlay.
 */
export class TimelineRowPins {
  readonly keys = new SvelteSet<string>();

  /** Keeps the item with `key` mounted until the returned function is called. */
  pin(key: string): () => void {
    this.keys.add(key);
    return () => this.keys.delete(key);
  }
}

const contextKey = Symbol('timeline-row-pins');

/** Provides row pins to the rows of one timeline. */
export function setTimelineRowPins(pins: TimelineRowPins): void {
  setContext(contextKey, pins);
}

/** Returns the pins of the enclosing timeline, or undefined outside a timeline. */
export function getTimelineRowPins(): TimelineRowPins | undefined {
  return getContext<TimelineRowPins | undefined>(contextKey);
}
