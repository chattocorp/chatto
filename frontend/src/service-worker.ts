/// <reference lib="webworker" />

/**
 * Service Worker for push notifications.
 *
 * Handles push events from the browser vendor's push service and displays
 * native OS notifications. Also handles notification clicks to navigate
 * to the relevant content.
 */

declare const self: ServiceWorkerGlobalScope;

/**
 * Immediately activate new service worker versions.
 * Without this, users must close all tabs before updates take effect.
 */
self.addEventListener('install', () => {
  self.skipWaiting();
});

self.addEventListener('activate', (event) => {
  event.waitUntil(self.clients.claim());
});

// Type for push notification payload from server
interface PushPayload {
  title?: string;
  body?: string;
  icon?: string;
  badge?: string;
  tag?: string;
  notificationId?: string;
  url?: string;
  // "dismiss" action is used to close notifications on other devices
  action?: 'dismiss';
}

/**
 * Handle incoming push events.
 * Parse the payload and display a native notification, or dismiss existing ones.
 */
self.addEventListener('push', (event) => {
  if (!event.data) {
    console.warn('Push event received with no data');
    return;
  }

  let payload: PushPayload;
  try {
    payload = event.data.json() as PushPayload;
  } catch {
    console.error('Failed to parse push payload');
    return;
  }

  // Handle dismiss action - close matching notifications on this device
  if (payload.action === 'dismiss' && payload.tag) {
    event.waitUntil(
      self.registration.getNotifications({ tag: payload.tag }).then((notifications) => {
        notifications.forEach((n) => n.close());
      })
    );
    return;
  }

  // Regular notification display
  const options: NotificationOptions = {
    body: payload.body,
    icon: payload.icon ?? '/icons/icon-192.png',
    badge: payload.badge ?? '/icons/icon-192.png',
    tag: payload.tag,
    // Pass notificationId and url in data for the click handler
    data: {
      notificationId: payload.notificationId,
      url: payload.url
    }
  };

  event.waitUntil(self.registration.showNotification(payload.title ?? 'New notification', options));
});

/**
 * Handle notification clicks.
 * Prefer postMessage to an already-open client so the SPA can route via
 * `goto()` (no full reload). Fall back to `WindowClient.navigate()` or
 * `openWindow()` when no client is open or messaging fails.
 */
self.addEventListener('notificationclick', (event) => {
  event.notification.close();

  const path = event.notification.data?.url ?? '/chat';
  // Build absolute URL for openWindow (required by some browsers).
  // Reject any URL that resolves to a different origin — `new URL(absUrl, origin)`
  // returns the absolute URL when its first arg is already absolute, so a push
  // payload with `data.url = "https://attacker.example/"` would otherwise
  // navigate the user there.
  const candidate = new URL(path, self.location.origin);
  if (candidate.origin !== self.location.origin) {
    return;
  }
  const url = candidate.href;

  event.waitUntil(
    (async () => {
      const clientList = await self.clients.matchAll({
        type: 'window',
        includeUncontrolled: true
      });

      // Prefer postMessage to an existing client — the SPA listener handles
      // navigation via goto(), avoiding a full document reload when the user
      // is already on the target URL (or anywhere in the SPA).
      for (const client of clientList) {
        if ('focus' in client) {
          try {
            const focusedClient = await client.focus();
            if (focusedClient) {
              focusedClient.postMessage({ type: 'notification-click', url });
              return;
            }
          } catch (err) {
            console.warn('[SW] Failed to focus existing window:', err);
          }
          // Focus didn't yield a client — fall back to navigate().
          try {
            if ('navigate' in client) {
              const navigatedClient = await (client as WindowClient).navigate(url);
              if (navigatedClient) {
                return;
              }
            }
          } catch (err) {
            console.warn('[SW] Failed to navigate existing window:', err);
          }
          break;
        }
      }

      // Fallback: open a new window
      await self.clients.openWindow(url);
    })().catch((err) => {
      console.error('[SW] Error handling notification click:', err);
    })
  );
});

/**
 * Handle push subscription changes.
 * This can happen when the browser's push subscription expires or is revoked.
 * We re-subscribe and update the server.
 */
self.addEventListener('pushsubscriptionchange', (event) => {
  // Send a message to any open clients to trigger re-subscription
  event.waitUntil(
    self.clients.matchAll({ type: 'window' }).then((clients) => {
      clients.forEach((client) => {
        client.postMessage({ type: 'push-subscription-changed' });
      });
    })
  );
});

// Export empty object for SvelteKit to recognize this as a module
export {};
