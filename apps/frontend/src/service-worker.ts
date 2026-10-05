/// <reference lib="webworker" />
/// <reference types="@sveltejs/kit" />

/**
 * Service worker for push notifications and a versioned offline application shell.
 * The worker caches no private content. See ADR-107.
 */

import { APP_BADGE_REFRESH_MESSAGE_TYPE, updateAppBadge } from '$lib/notifications/appBadge';
import { build, version } from '$service-worker';
import { routeNotificationClick } from '$lib/pwa/notificationClick.worker';

declare const self: ServiceWorkerGlobalScope;

/**
 * Identifies one build. Compiled file names carry content hashes, so the key
 * changes with each build even when local builds repeat the version name.
 */
function buildKey(paths: readonly string[]): string {
  const text = paths.join('\n');
  let hash = 0x811c9dc5;
  for (let index = 0; index < text.length; index += 1) {
    hash = Math.imul(hash ^ text.charCodeAt(index), 0x01000193);
  }
  return (hash >>> 0).toString(36);
}

const SHELL_CACHE_PREFIX = 'chatto-shell-';
// Each build installs into its own cache. A shared cache would keep the files
// of earlier builds and exceed the storage budget.
const SHELL_CACHE = `${SHELL_CACHE_PREFIX}${version}-${buildKey(build)}`;
const MAX_SHELL_BYTES = 12_000_000;
const ownsAppShell = new URL(self.registration.scope).pathname === '/';
const shellAssets = new Set(build.map((path) => new URL(path, self.location.origin).pathname));
const OFFLINE_DOCUMENT = '/login';
const RETIRED_BADGE_CACHE_NAMES = new Set(['chatto-badge-state-v1', 'chatto-badge-state-v2']);

/**
 * Install only compiled application code and a public, unauthenticated HTML
 * document. API calls, authentication, realtime, and uploaded assets are never
 * added to Cache Storage.
 */
self.addEventListener('install', (event) => {
  event.waitUntil(
    (async () => {
      if (!ownsAppShell) {
        await self.skipWaiting();
        return;
      }
      const cache = await caches.open(SHELL_CACHE);
      await cache.addAll([...build, OFFLINE_DOCUMENT]);
      let bytes = 0;
      for (const request of await cache.keys()) {
        const response = await cache.match(request);
        bytes += (await response?.arrayBuffer())?.byteLength ?? 0;
        if (bytes > MAX_SHELL_BYTES) {
          await caches.delete(SHELL_CACHE);
          throw new Error('Offline application shell exceeds its storage budget');
        }
      }
      await self.skipWaiting();
    })()
  );
});

/**
 * Delete shell versions only after the current shell has installed.
 */
self.addEventListener('activate', (event) => {
  event.waitUntil(
    (async () => {
      if (!ownsAppShell) {
        await self.clients.claim();
        return;
      }
      const cacheNames = await caches.keys();
      await Promise.all(
        cacheNames
          .filter(
            (cacheName) =>
              (cacheName.startsWith(SHELL_CACHE_PREFIX) && cacheName !== SHELL_CACHE) ||
              RETIRED_BADGE_CACHE_NAMES.has(cacheName)
          )
          .map((cacheName) => caches.delete(cacheName))
      );
      await self.clients.claim();
    })()
  );
});

/**
 * Serve compiled assets from the versioned shell; they are immutable. App
 * navigations use the network first, so a reload after a deploy loads the new
 * frontend. The cached shell document is the fallback when the server is
 * unreachable or fails.
 */
self.addEventListener('fetch', (event) => {
  if (!ownsAppShell) return;
  const request = event.request;
  if (request.method !== 'GET') return;
  const url = new URL(request.url);
  if (url.origin !== self.location.origin) return;

  if (shellAssets.has(url.pathname)) {
    event.respondWith(
      (async () => {
        const cached = await (await caches.open(SHELL_CACHE)).match(request);
        return cached ?? fetch(request);
      })()
    );
    return;
  }
  const appNavigation =
    url.pathname === '/' ||
    url.pathname === '/login' ||
    url.pathname === '/chat' ||
    url.pathname.startsWith('/chat/');
  if (request.mode === 'navigate' && appNavigation) {
    const cachedShell = async () => (await caches.open(SHELL_CACHE)).match(OFFLINE_DOCUMENT);
    event.respondWith(
      (async () => {
        try {
          const response = await fetch(request);
          if (response.status < 500) return response;
          return (await cachedShell()) ?? response;
        } catch (error) {
          const cached = await cachedShell();
          if (cached) return cached;
          throw error;
        }
      })()
    );
  }
});

// Type for push notification payload from server
interface PushPayload {
  title?: string;
  body?: string;
  icon?: string;
  badge?: string;
  tag?: string;
  notificationId?: string;
  serverOrigin?: string;
  recipientId?: string;
  url?: string;
  app_badge?: string | number;
  attentionLevel?: string;
}

interface DeclarativePushPayload extends PushPayload {
  web_push?: number;
  mutable?: boolean;
  notification?: DeclarativeNotificationPayload;
}

interface DeclarativeNotificationPayload {
  title?: string;
  body?: string;
  icon?: string;
  badge?: string;
  app_badge?: string | number;
  tag?: string;
  navigate?: string;
  data?: {
    notificationId?: string;
    attentionLevel?: string;
    serverOrigin?: string;
    recipientId?: string;
    url?: string;
  };
}

type NormalizedPushNotification = {
  important: boolean;
  title: string;
  options: NotificationOptions;
};

type DeclarativePushEventNotification = Pick<
  Notification,
  'title' | 'body' | 'icon' | 'tag' | 'data'
> & {
  badge?: string;
};

type PushEventWithDeclarativeNotification = PushEvent & {
  notification?: DeclarativePushEventNotification | null;
};

function normalizePushNotification(payload: DeclarativePushPayload): NormalizedPushNotification {
  const notification = payload.notification;
  const notificationId = payload.notificationId ?? notification?.data?.notificationId;
  const url = payload.url ?? notification?.data?.url ?? notification?.navigate;

  return {
    important: (payload.attentionLevel ?? notification?.data?.attentionLevel) === 'important',
    title: payload.title ?? notification?.title ?? 'New notification',
    options: {
      body: payload.body ?? notification?.body,
      icon: payload.icon ?? notification?.icon ?? '/icons/icon-192.png',
      badge: payload.badge ?? notification?.badge ?? '/icons/icon-192.png',
      tag: payload.tag ?? notification?.tag,
      data: {
        notificationId,
        serverOrigin: payload.serverOrigin ?? notification?.data?.serverOrigin,
        recipientId: payload.recipientId ?? notification?.data?.recipientId,
        url
      }
    }
  };
}

function declarativePayloadFromEventNotification(
  notification: DeclarativePushEventNotification
): DeclarativePushPayload {
  return {
    notification: {
      title: notification.title,
      body: notification.body,
      icon: notification.icon,
      badge: notification.badge,
      tag: notification.tag,
      data: notificationData(notification.data)
    }
  };
}

function notificationData(data: unknown): DeclarativeNotificationPayload['data'] {
  if (typeof data !== 'object' || data === null) return undefined;
  return {
    notificationId: stringProperty(data, 'notificationId'),
    attentionLevel: stringProperty(data, 'attentionLevel'),
    serverOrigin: stringProperty(data, 'serverOrigin'),
    recipientId: stringProperty(data, 'recipientId'),
    url: stringProperty(data, 'url')
  };
}

function stringProperty(record: object, key: string): string | undefined {
  const value = (record as Record<string, unknown>)[key];
  return typeof value === 'string' ? value : undefined;
}

/** Ask visible pages to restore their authoritative aggregate after a regular push. */
async function refreshVisibleAppBadges(): Promise<void> {
  let windowClients: readonly WindowClient[];
  try {
    windowClients = (await self.clients.matchAll({
      type: 'window',
      includeUncontrolled: true
    })) as WindowClient[];
  } catch {
    return;
  }

  for (const client of windowClients) {
    if (client.visibilityState === 'visible') {
      client.postMessage({ type: APP_BADGE_REFRESH_MESSAGE_TYPE });
    }
  }
}

/**
 * Handle incoming push events.
 * Parse the payload and display a native notification.
 */
self.addEventListener('push', (event) => {
  const declarativeNotification = (event as PushEventWithDeclarativeNotification).notification;
  // Subscriptions promise to show a notification for every push. An unreadable
  // push still shows a generic notification instead of the browser's own
  // "updated in the background" fallback.
  let payload: DeclarativePushPayload = {};
  if (event.data) {
    try {
      payload = event.data.json() as DeclarativePushPayload;
    } catch {
      console.error('Failed to parse push payload');
    }
  } else if (declarativeNotification) {
    payload = declarativePayloadFromEventNotification(declarativeNotification);
  } else {
    console.warn('Push event received with no data or declarative notification');
  }

  const notification = normalizePushNotification(payload);

  event.waitUntil(
    (async () => {
      await self.registration.showNotification(notification.title, notification.options);
      // Ambient pushes and older payloads without a classification must not
      // create a badge. Visible apps reconcile against current state afterward.
      if (notification.important) await updateAppBadge({ kind: 'flag' });
      await refreshVisibleAppBadges();
    })()
  );
});

/**
 * Handle notification clicks.
 * Send the click to an already-open client so the SPA can route via `goto()`
 * (no full reload). See `routeNotificationClick` for the window order and the
 * fallback to a new window.
 */
self.addEventListener('notificationclick', (event) => {
  event.notification.close();

  const rawUrl =
    typeof event.notification.data?.url === 'string' ? event.notification.data.url : undefined;
  event.waitUntil(
    routeNotificationClick(rawUrl, self.location.origin, self.clients).catch((err) => {
      console.error('[SW] Error handling notification click:', err);
    })
  );
});

// Export empty object for SvelteKit to recognize this as a module
export {};
