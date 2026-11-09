/** Window-wide device appearance. Values are logical sizes, independent of Retina density. */
export const appearance = $state({ scale: 1 });
// Imperative subscriptions are not UI state. Reading them must not track or
// rerun a Svelte attachment when another view subscribes.
const listeners = new Set<(next: number) => void>();
/** Scale an immutable base length; reading this in a template tracks changes. */
export function scaled(base: number): number {
  return base * appearance.scale;
}
/** Capture native scroll anchors before changing geometry. */
export function beforeScaleChange(listener: (next: number) => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}
/** Apply a finite scale between 50% and 200%, without rounding to fixed steps. */
export function changeScale(value: number): void {
  const next = normalizeScale(value);
  if (next === appearance.scale) return;
  for (const listener of listeners) listener(next);
  appearance.scale = next;
}
/** Reject non-finite values and keep zoom within the supported layout range. */
export function normalizeScale(value: number): number {
  return Number.isFinite(value) ? Math.min(2, Math.max(0.5, value)) : 1;
}
