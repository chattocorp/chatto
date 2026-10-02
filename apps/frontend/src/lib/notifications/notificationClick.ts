/**
 * Page side of push notification clicks. Kept apart from the push registration
 * module so the root layout can listen for clicks without loading Web Push
 * registration code into every route's initial bundle.
 */

import {
  NOTIFICATION_CLICK_ACK_MESSAGE_TYPE,
  NOTIFICATION_CLICK_MESSAGE_TYPE
} from '$lib/pwa/notificationClick.worker';

/**
 * Listen for notification-click messages from the service worker.
 * The SW posts these instead of calling `WindowClient.navigate()` so the
 * SPA can route via `goto()` (client-side navigation, no full reload).
 */
export function onNotificationClick(callback: (url: string) => void | Promise<void>): () => void {
  if (!('serviceWorker' in navigator)) {
    return () => {};
  }

  const handler = (event: MessageEvent) => {
    if (
      event.data?.type === NOTIFICATION_CLICK_MESSAGE_TYPE &&
      typeof event.data.url === 'string'
    ) {
      const responsePort = event.ports[0];
      void (async () => {
        try {
          await callback(event.data.url);
          responsePort?.postMessage({ type: NOTIFICATION_CLICK_ACK_MESSAGE_TYPE });
        } catch {
          // Leave the service worker unacknowledged so it can fall back to
          // WindowClient.navigate() after its timeout.
        }
      })();
    }
  };

  navigator.serviceWorker.addEventListener('message', handler);
  return () => navigator.serviceWorker.removeEventListener('message', handler);
}
