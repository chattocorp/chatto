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

function isInstalledAppContext(): boolean {
  if (typeof window === 'undefined') return false;

  const standaloneDisplayModes = ['standalone', 'fullscreen', 'minimal-ui', 'window-controls-overlay'];
  if (standaloneDisplayModes.some((mode) => window.matchMedia?.(`(display-mode: ${mode})`).matches)) {
    return true;
  }

  return (navigator as Navigator & { standalone?: boolean }).standalone === true;
}

function normalizeBadgeCount(notificationCount: number): number {
  if (!Number.isFinite(notificationCount)) return 0;
  return Math.max(0, Math.floor(notificationCount));
}

/**
 * Share the foreground notification count with the service worker so stale
 * push/native notification badge state can be reconciled against the app's
 * authoritative pending-notification state.
 */
export function syncServiceWorkerNotificationBadgeState(notificationCount: number): void {
  if (typeof navigator === 'undefined' || !('serviceWorker' in navigator)) return;

  navigator.serviceWorker.controller?.postMessage({
    type: 'chatto-badge-state',
    notificationCount: normalizeBadgeCount(notificationCount),
    serviceWorkerAppBadgeEnabled: isSupported() && isInstalledAppContext()
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
