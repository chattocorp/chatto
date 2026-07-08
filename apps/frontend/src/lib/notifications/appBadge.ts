/**
 * App Badge API helper for PWA dock badges.
 *
 * Shows notification attention on the app icon when installed as PWA.
 * Safari requires notification permission; Chrome/Edge work without it.
 *
 * @see https://developer.mozilla.org/en-US/docs/Web/API/Badging_API
 */

export type AppBadgeIntent =
  | { kind: 'clear' }
  | { kind: 'flag' }
  | { kind: 'count'; count: number };

/**
 * Check if the Badging API is supported in this browser context.
 */
export function isSupported(): boolean {
  return typeof navigator !== 'undefined' && 'setAppBadge' in navigator;
}

function isInstalledAppContext(): boolean {
  if (typeof window === 'undefined') return false;

  const standaloneDisplayModes = [
    'standalone',
    'fullscreen',
    'minimal-ui',
    'window-controls-overlay'
  ];
  if (
    standaloneDisplayModes.some((mode) => window.matchMedia?.(`(display-mode: ${mode})`).matches)
  ) {
    return true;
  }

  return (navigator as Navigator & { standalone?: boolean }).standalone === true;
}

export function normalizeBadgeCount(count: number): number {
  if (!Number.isFinite(count)) return 0;
  return Math.max(0, Math.floor(count));
}

export function normalizeBadgeIntent(intent: AppBadgeIntent): AppBadgeIntent {
  if (intent.kind !== 'count') return intent;
  const count = normalizeBadgeCount(intent.count);
  return count > 0 ? { kind: 'count', count } : { kind: 'clear' };
}

function legacyNotificationCount(intent: AppBadgeIntent): number {
  switch (intent.kind) {
    case 'count':
      return normalizeBadgeCount(intent.count);
    case 'flag':
      return 1;
    case 'clear':
      return 0;
  }
}

/**
 * Share the foreground badge intent with the service worker so stale
 * push/native notification badge state can be reconciled against the app's
 * authoritative pending-notification state.
 */
export function syncServiceWorkerNotificationBadgeState(intent: AppBadgeIntent): void {
  if (typeof navigator === 'undefined' || !('serviceWorker' in navigator)) return;

  const normalized = normalizeBadgeIntent(intent);
  navigator.serviceWorker.controller?.postMessage({
    type: 'chatto-badge-state',
    badgeIntent: normalized,
    // Kept as a best-effort fallback for older active service workers.
    notificationCount: legacyNotificationCount(normalized),
    serviceWorkerAppBadgeEnabled: isSupported() && isInstalledAppContext()
  });
}

/**
 * Update the app badge for the given intent.
 * Sets a numeric badge for DMs, a flag/dot for channel notifications, and
 * clears it when notifications are handled.
 *
 * Silently fails if:
 * - Badging API not supported
 * - App not installed as PWA
 * - Safari without notification permission
 */
export async function updateBadge(intent: AppBadgeIntent): Promise<void> {
  if (!isSupported()) return;

  try {
    const normalized = normalizeBadgeIntent(intent);
    switch (normalized.kind) {
      case 'count':
        await navigator.setAppBadge(normalized.count);
        break;
      case 'flag':
        await navigator.setAppBadge();
        break;
      case 'clear':
        await navigator.clearAppBadge();
        break;
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
