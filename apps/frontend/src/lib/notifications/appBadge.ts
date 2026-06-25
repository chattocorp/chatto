/**
 * App Badge API helper for PWA dock badges.
 *
 * Shows notification count on the app icon when installed as PWA.
 * Safari requires notification permission; Chrome/Edge work without it.
 *
 * @see https://developer.mozilla.org/en-US/docs/Web/API/Badging_API
 */

/**
 * Check if the Badging API is supported in this browser context.
 */
export function isSupported(): boolean {
  return typeof navigator !== 'undefined' && 'setAppBadge' in navigator;
}

/**
 * Share foreground unread badge state with the service worker so its native
 * notification cleanup does not erase an unread-only dock flag.
 */
export function syncServiceWorkerUnreadBadgeState(hasAnyUnread: boolean): void {
  if (typeof navigator === 'undefined' || !('serviceWorker' in navigator)) return;

  navigator.serviceWorker.controller?.postMessage({
    type: 'chatto-badge-state',
    hasAnyUnread
  });
}

/**
 * Update the app badge with the given count.
 * Sets a numeric badge if count > 0, clears it otherwise.
 *
 * Silently fails if:
 * - Badging API not supported
 * - App not installed as PWA
 * - Safari without notification permission
 */
export async function updateBadge(count: number): Promise<void> {
  if (!isSupported()) return;

  try {
    if (count > 0) {
      await navigator.setAppBadge(count);
    } else {
      await navigator.clearAppBadge();
    }
  } catch (e) {
    // Silently fail - badge API may not work in all contexts
    // (e.g., not installed as PWA, permission denied on Safari)
    console.debug('Badge update failed:', e);
  }
}

/**
 * Clear the app badge.
 */
export async function clearBadge(): Promise<void> {
  if (!isSupported()) return;

  try {
    await navigator.clearAppBadge();
  } catch {
    // Silently fail
  }
}

/**
 * Set a flag badge (dot without number).
 * Indicates activity without a specific count.
 */
export async function setFlagBadge(): Promise<void> {
  if (!isSupported()) return;

  try {
    await navigator.setAppBadge(); // No argument = flag mode
  } catch (e) {
    console.debug('Badge update failed:', e);
  }
}
