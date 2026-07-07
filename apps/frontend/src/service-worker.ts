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
import { OFFLINE_SHELL_PATH, classifyServiceWorkerRequest } from '$lib/pwa/serviceWorkerPolicy';
import {
  routeNotificationClick,
  type NotificationClickClients
} from '$lib/pwa/notificationClick.worker';
import {
  ServiceWorkerBadgeCoordinator,
  createCacheForegroundNotificationCountStorage
} from '$lib/pwa/notificationBadge.worker';

declare const self: ServiceWorkerGlobalScope;

const CACHE_PREFIX = 'chatto-shell';
const CACHE_NAME = `${CACHE_PREFIX}-${version}`;
const BADGE_STATE_CACHE_NAME = 'chatto-badge-state-v1';
const SHELL_ASSETS = new Set([...build, ...files, OFFLINE_SHELL_PATH]);
const PRECACHE_ASSETS = Array.from(new Set([...build, OFFLINE_SHELL_PATH, '/']));

type ServiceWorkerAppBadgeNavigator = WorkerNavigator & {
  setAppBadge?: (contents?: number) => Promise<void>;
  clearAppBadge?: () => Promise<void>;
};

const badgeCoordinator = new ServiceWorkerBadgeCoordinator(
  self.registration,
  navigator as ServiceWorkerAppBadgeNavigator,
  createCacheForegroundNotificationCountStorage(caches, BADGE_STATE_CACHE_NAME)
);

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
  handleBadgeStateMessage(event);
});

/**
 * Serve known app-shell assets from the versioned cache. For navigations, try
 * the network first and fall back to the cached SPA shell only when offline.
 *
 * Chat data, API responses, auth endpoints, uploaded assets, and cross-origin
 * requests stay network-only so stale data never masquerades as live state.
 */
self.addEventListener('fetch', (event) => {
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
    url?: string;
  };
}

type NormalizedPushNotification = {
  title: string;
  options: NotificationOptions;
  appBadgeCount: number | null;
};

type DeclarativePushEventNotification = Pick<
  Notification,
  'title' | 'body' | 'icon' | 'tag' | 'data'
> & {
  badge?: string;
  app_badge?: string | number;
};

type PushEventWithDeclarativeNotification = PushEvent & {
  notification?: DeclarativePushEventNotification | null;
};

function handleBadgeStateMessage(event: ExtendableMessageEvent): boolean {
  const message = event.data as Record<string, unknown> | undefined;
  if (!message || message.type !== 'chatto-badge-state') return false;
  if (typeof message.notificationCount !== 'number') return false;

  event.waitUntil(
    badgeCoordinator.applyForegroundNotificationCount(message.notificationCount, {
      serviceWorkerAppBadgeEnabled: message.serviceWorkerAppBadgeEnabled === true
    })
  );
  return true;
}

function normalizePushNotification(payload: DeclarativePushPayload): NormalizedPushNotification {
  const notification = payload.notification;
  const notificationId = payload.notificationId ?? notification?.data?.notificationId;
  const url = payload.url ?? notification?.data?.url ?? notification?.navigate;

  return {
    title: payload.title ?? notification?.title ?? 'New notification',
    options: {
      body: payload.body ?? notification?.body,
      icon: payload.icon ?? notification?.icon ?? '/icons/icon-192.png',
      badge: payload.badge ?? notification?.badge ?? '/icons/icon-192.png',
      tag: payload.tag ?? notification?.tag,
      data: {
        notificationId,
        url
      }
    },
    appBadgeCount: declarativeAppBadgeCount(notification?.app_badge)
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
      app_badge: notification.app_badge,
      tag: notification.tag,
      data: notificationData(notification.data)
    }
  };
}

function declarativeAppBadgeCount(appBadge: unknown): number | null {
  if (typeof appBadge === 'number' && Number.isFinite(appBadge)) {
    return Math.max(0, Math.floor(appBadge));
  }
  if (typeof appBadge !== 'string' || appBadge.trim() === '') return null;

  const count = Number(appBadge);
  return Number.isFinite(count) ? Math.max(0, Math.floor(count)) : null;
}

function notificationData(data: unknown): DeclarativeNotificationPayload['data'] {
  if (typeof data !== 'object' || data === null) return undefined;
  return {
    notificationId: stringProperty(data, 'notificationId'),
    url: stringProperty(data, 'url')
  };
}

function stringProperty(record: object, key: string): string | undefined {
  const value = (record as Record<string, unknown>)[key];
  return typeof value === 'string' ? value : undefined;
}

/**
 * Handle incoming push events.
 * Parse the payload and display a native notification, or dismiss existing ones.
 */
self.addEventListener('push', (event) => {
  const declarativeNotification = (event as PushEventWithDeclarativeNotification).notification;
  let payload: DeclarativePushPayload;
  if (event.data) {
    try {
      payload = event.data.json() as DeclarativePushPayload;
    } catch {
      console.error('Failed to parse push payload');
      return;
    }
  } else if (declarativeNotification) {
    payload = declarativePayloadFromEventNotification(declarativeNotification);
  } else {
    console.warn('Push event received with no data or declarative notification');
    return;
  }

  // Handle dismiss action - close matching notifications on this device
  if (payload.action === 'dismiss' && payload.tag) {
    event.waitUntil(
      (async () => {
        const notifications = await self.registration.getNotifications({ tag: payload.tag });
        notifications.forEach((n) => n.close());
        await badgeCoordinator.reconcileAfterDismissPush();
      })()
    );
    return;
  }

  badgeCoordinator.recordRegularPush();
  const notification = normalizePushNotification(payload);

  event.waitUntil(
    Promise.all([
      self.registration.showNotification(notification.title, notification.options),
      notification.appBadgeCount !== null
        ? badgeCoordinator.setPushAppBadgeCount(notification.appBadgeCount)
        : badgeCoordinator.setProvisionalPushFlagBadge()
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
  event.waitUntil(
    (async () => {
      try {
        await routeNotificationClick(
          rawUrl,
          self.location.origin,
          self.clients as unknown as NotificationClickClients,
          { logger: console }
        );
      } finally {
        await badgeCoordinator.reconcileAfterNotificationClick().catch(() => {});
      }
    })().catch((err) => {
      console.error('[SW] Error handling notification click:', err);
    })
  );
});

// Export empty object for SvelteKit to recognize this as a module
export {};
