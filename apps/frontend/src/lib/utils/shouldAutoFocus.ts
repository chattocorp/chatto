import { prefersTouchActions } from './inputCapabilities';

/**
 * Returns true if autofocus should be enabled.
 * Disabled on touch devices where the keyboard popup would be jarring.
 */
export function shouldAutoFocus(): boolean {
  if (typeof window === 'undefined') return false;
  return !prefersTouchActions();
}
