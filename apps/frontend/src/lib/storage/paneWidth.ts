/**
 * Shared storage plumbing for resizable pane and sidebar widths.
 *
 * Each width is one globally-scoped `localStorage` slot with min/max bounds.
 * The clamp is owned here — the single place that turns an arbitrary incoming
 * width into a persisted, in-bounds value. The codec's bounds act only as a
 * corruption fallback for stored payloads written outside this path (for
 * example by hand-edited localStorage): those parse back to the default
 * instead of being trusted.
 */

import { Codecs, globalSlot } from './slot';

/** Persisted width slot with explicit visual bounds. */
export interface PaneWidthSlot {
  defaultValue: number;
  minWidth: number;
  maxWidth: number;
  /** Read the stored width, or `defaultValue` when missing / corrupt. */
  get(): number;
  /**
   * Clamp `width` into `[minWidth, maxWidth]`, persist it best-effort, and
   * return the clamped value so callers can mirror memory.
   */
  set(width: number): number;
}

/**
 * Build a pane-width storage slot at `chatto:{suffix}`.
 *
 * `set()` clamps before persisting; unavailable or full browser storage makes
 * persistence a silent no-op while still returning the clamped value.
 */
export function paneWidthSlot(
  suffix: string,
  options: { defaultValue: number; minWidth: number; maxWidth: number }
): PaneWidthSlot {
  const { defaultValue, minWidth, maxWidth } = options;
  const slot = globalSlot(suffix, defaultValue, Codecs.number({ min: minWidth, max: maxWidth }));
  return {
    defaultValue,
    minWidth,
    maxWidth,
    get: () => slot.get(),
    set: (width: number) => {
      const clamped = Math.min(maxWidth, Math.max(minWidth, width));
      slot.set(clamped);
      return clamped;
    }
  };
}
