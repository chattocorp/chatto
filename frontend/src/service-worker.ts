/// <reference lib="webworker" />
/// <reference types="@sveltejs/kit" />

/**
 * Service Worker for Chatto's PWA shell and push notifications.
 *
 * Keeps the app shell available during offline launches while leaving live
 * Chatto data on the network. It also handles Web Push notifications and
 * notification-click navigation.
 */

import { build, files, version } from '$service-worker';
import {
  OFFLINE_SHELL_PATH,
  classifyServiceWorkerRequest,
  normalizeSameOriginUrl
} from '$lib/pwa/serviceWorkerPolicy';
import {
  handleAssetProxyFetch,
  handleAssetProxyMessage,
  parseAssetProxyRequest
} from '$lib/pwa/assetProxy.worker';

declare const self: ServiceWorkerGlobalScope;

const CACHE_PREFIX = 'chatto-shell';
const CACHE_NAME = `${CACHE_PREFIX}-${version}`;
const SHELL_ASSETS = new Set([...build, ...files, OFFLINE_SHELL_PATH]);
const PRECACHE_ASSETS = Array.from(new Set([...build, ...files, OFFLINE_SHELL_PATH, '/']));

type BadgeCapableNavigator = Navigator & {
  setAppBadge?: (contents?: number) => Promise<void>;
  clearAppBadge?: () => Promise<void>;
};

/**
 * Immediately activate new service worker versions.
 * Without this, users must close all tabs before updates take effect.
 */
self.addEventListener('install', (event) => {
  self.skipWaiting();
  event.waitUntil(
    caches
      .open(CACHE_NAME)
      .then((cache) => Promise.all(PRECACHE_ASSETS.map((path) => cacheShellAsset(cache, path))))
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    (async () => {
      const cacheNames = await caches.keys();
      await Promise.all(
        cacheNames
          .filter(
            (cacheName) => cacheName.startsWith(`${CACHE_PREFIX}-`) && cacheName !== CACHE_NAME
          )
          .map((cacheName) => caches.delete(cacheName))
      );
      await self.clients.claim();
    })()
  );
});

self.addEventListener('message', (event) => {
  handleAssetProxyMessage(event);
});

/**
 * Serve known app-shell assets from the versioned cache. For navigations, try
 * the network first and fall back to the cached SPA shell only when offline.
 *
 * Chat data, API responses, auth endpoints, uploaded assets, and cross-origin
 * requests stay network-only so stale data never masquerades as live state.
 */
self.addEventListener('fetch', (event) => {
  const assetProxyRequest = parseAssetProxyRequest(event.request.url, self.location.origin);
  if (assetProxyRequest) {
    event.respondWith(handleAssetProxyFetch(event.request, assetProxyRequest));
    return;
  }

  const policy = classifyServiceWorkerRequest(
    event.request,
    event.request.url,
    SHELL_ASSETS,
    self.location.origin
  );

  if (policy.networkOnly) return;

  if (policy.cacheableShellAsset) {
    event.respondWith(
      (async () => {
        const cache = await caches.open(CACHE_NAME);
        const url = new URL(event.request.url);
        const cached = await cache.match(url.pathname);
        if (cached) return cached;

        const response = await fetch(event.request);
        if (response.ok) {
          await cache.put(url.pathname, response.clone());
        }
        return response;
      })()
    );
    return;
  }

  if (policy.navigationRequest) {
    event.respondWith(
      (async () => {
        try {
          return await fetch(event.request);
        } catch (err) {
          const cache = await caches.open(CACHE_NAME);
          const shell = await getCachedOfflineShell(cache);
          if (shell) return shell;
          throw err;
        }
      })()
    );
  }
});

async function cacheShellAsset(cache: Cache, path: string): Promise<void> {
  try {
    const response = await fetch(path, { cache: 'reload' });
    if (!response.ok) return;
    await cache.put(path, response);
  } catch {
    // A missing static fallback in local preview must not invalidate the whole
    // service worker. Production nginx serves the same shell through /200.html.
  }
}

async function getCachedOfflineShell(cache: Cache): Promise<Response | undefined> {
  return (await cache.match(OFFLINE_SHELL_PATH)) ?? cache.match('/');
}

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

function setFlagBadge(): Promise<void> {
  const badgeNavigator = navigator as BadgeCapableNavigator;
  return badgeNavigator.setAppBadge?.().catch(() => {}) ?? Promise.resolve();
}

async function clearBadgeIfNoNotificationsRemain(): Promise<void> {
  const notifications = await self.registration.getNotifications();
  if (notifications.length > 0) return;

  const badgeNavigator = navigator as BadgeCapableNavigator;
  await (badgeNavigator.clearAppBadge?.().catch(() => {}) ?? Promise.resolve());
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
      (async () => {
        const notifications = await self.registration.getNotifications({ tag: payload.tag });
        notifications.forEach((n) => n.close());
        await clearBadgeIfNoNotificationsRemain();
      })()
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

  event.waitUntil(
    Promise.all([
      self.registration.showNotification(payload.title ?? 'New notification', options),
      setFlagBadge()
    ])
  );
});

/**
 * Handle notification clicks.
 * Prefer postMessage to an already-open client so the SPA can route via
 * `goto()` (no full reload). Fall back to `WindowClient.navigate()` or
 * `openWindow()` when no client is open or messaging fails.
 */
self.addEventListener('notificationclick', (event) => {
  event.notification.close();

  const rawUrl =
    typeof event.notification.data?.url === 'string' ? event.notification.data.url : undefined;
  const url = normalizeSameOriginUrl(rawUrl, self.location.origin);
  if (!url) return;

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
