import { prefersTouchActions } from './inputCapabilities';

/**
 * Returns true if the device's primary pointer is coarse. Kept as a
 * compatibility wrapper; new interaction decisions should prefer the explicit
 * helpers in `inputCapabilities.ts`.
 */
export function isTouchDevice(): boolean {
  return prefersTouchActions();
}
