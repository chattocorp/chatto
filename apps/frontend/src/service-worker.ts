/// <reference lib="webworker" />
/// <reference types="@sveltejs/kit" />

/**
 * Service Worker for Chatto's push notifications.
 *
 * Frontend and uploaded-asset requests use the browser's normal HTTP caching
 * behavior without service-worker interception.
 */

import { APP_BADGE_REFRESH_MESSAGE_TYPE } from '$lib/notifications/appBadge';
import {
  routeNotificationClick,
  type NotificationClickClients
} from '$lib/pwa/notificationClick.worker';

declare const self: ServiceWorkerGlobalScope;

type ServiceWorkerAppBadgeNavigator = WorkerNavigator & {
  setAppBadge?: (contents?: number) => Promise<void>;
};

const badgeNavigator = navigator as ServiceWorkerAppBadgeNavigator;

const RETIRED_SHELL_CACHE_PREFIX = 'chatto-shell-';
const RETIRED_BADGE_CACHE_NAMES = new Set(['chatto-badge-state-v1', 'chatto-badge-state-v2']);

/**
 * Retire an existing request-intercepting worker promptly, even when Chatto
 * tabs remain open. Installation performs no network or cache work.
 */
self.addEventListener('install', (event) => {
  event.waitUntil(self.skipWaiting());
});

/**
 * Delete Cache Storage left by earlier worker versions. Current workers do not
 * intercept requests or populate caches.
 */
self.addEventListener('activate', (event) => {
  event.waitUntil(
    (async () => {
      const cacheNames = await caches.keys();
      await Promise.all(
        cacheNames
          .filter(
            (cacheName) =>
              cacheName.startsWith(RETIRED_SHELL_CACHE_PREFIX) ||
              RETIRED_BADGE_CACHE_NAMES.has(cacheName)
          )
          .map((cacheName) => caches.delete(cacheName))
      );
      await self.clients.claim();
    })()
  );
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
  app_badge?: string | number;
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
    url: stringProperty(data, 'url')
  };
}

function stringProperty(record: object, key: string): string | undefined {
  const value = (record as Record<string, unknown>)[key];
  return typeof value === 'string' ? value : undefined;
}

/**
 * Apply the server's remaining origin count unless a visible page can provide
 * the authoritative aggregate multi-server badge. Hidden clients may be
 * suspended and cannot safely claim foreground ownership.
 */
async function updateClosedAppBadgeAfterDismiss(appBadge: unknown): Promise<void> {
  const count = parseAppBadgeCount(appBadge);
  if (count === undefined || !badgeNavigator.setAppBadge) return;

  let windowClients: readonly WindowClient[];
  try {
    windowClients = (await self.clients.matchAll({
      type: 'window',
      includeUncontrolled: true
    })) as WindowClient[];
  } catch {
    return;
  }
  if (windowClients.some((client) => client.visibilityState === 'visible')) return;

  await badgeNavigator.setAppBadge(count).catch(() => {});
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

function parseAppBadgeCount(value: unknown): number | undefined {
  const count = typeof value === 'string' && /^\d+$/.test(value) ? Number(value) : value;
  return typeof count === 'number' && Number.isSafeInteger(count) && count >= 0 ? count : undefined;
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
        await updateClosedAppBadgeAfterDismiss(payload.app_badge);
      })()
    );
    return;
  }

  const notification = normalizePushNotification(payload);

  event.waitUntil(
    (async () => {
      await self.registration.showNotification(notification.title, notification.options);
      await refreshVisibleAppBadges();
    })()
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
    routeNotificationClick(
      rawUrl,
      self.location.origin,
      self.clients as unknown as NotificationClickClients,
      { logger: console }
    ).catch((err) => {
      console.error('[SW] Error handling notification click:', err);
    })
  );
});

// Export empty object for SvelteKit to recognize this as a module
export {};
