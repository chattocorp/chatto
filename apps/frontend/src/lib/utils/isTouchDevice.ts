/**
 * Returns true if the device's primary pointer is coarse (touch).
 * Uses the `(pointer: coarse)` media query, which reliably detects
 * touch-primary devices without relying on viewport width.
 */
export function isTouchDevice(): boolean {
  if (typeof window === 'undefined') return false;
  return window.matchMedia('(pointer: coarse)').matches;
}
