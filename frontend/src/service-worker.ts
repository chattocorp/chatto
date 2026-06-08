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
import { ASSET_PROXY_PATH_PREFIX } from '$lib/assets/serviceWorkerAssetProxy';

declare const self: ServiceWorkerGlobalScope;

const CACHE_PREFIX = 'chatto-shell';
const CACHE_NAME = `${CACHE_PREFIX}-${version}`;
const ASSET_CACHE_NAME = 'chatto-assets-v1';
const MAX_ASSET_CACHE_ENTRIES = 250;
const SHELL_ASSETS = new Set([...build, ...files, OFFLINE_SHELL_PATH]);
const PRECACHE_ASSETS = Array.from(new Set([...SHELL_ASSETS, '/']));

type AssetProxyServer = {
  id: string;
  url: string;
  token: string | null;
};

type AssetProxyTarget = {
  serverId: string;
  virtualPath: string;
  targetUrl: string;
};

const assetProxyServers = new Map<string, AssetProxyServer>();
const registeredAssetTargets = new Map<string, AssetProxyTarget>();
const ASSET_PROXY_RESYNC_TIMEOUT_MS = 750;

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
  const message = event.data as Record<string, unknown> | undefined;
  if (!message || typeof message.type !== 'string') return;

  if (message.type === 'chatto-asset-proxy-sync-servers' && Array.isArray(message.servers)) {
    syncAssetProxyServers(message.servers);
    return;
  }

  if (
    message.type === 'chatto-asset-proxy-register-url' &&
    typeof message.serverId === 'string' &&
    typeof message.virtualPath === 'string' &&
    typeof message.targetUrl === 'string'
  ) {
    registerAssetProxyTarget({
      serverId: message.serverId,
      virtualPath: message.virtualPath,
      targetUrl: message.targetUrl
    });
    return;
  }

  if (message.type === 'chatto-asset-proxy-clear-cache') {
    event.waitUntil(
      clearAssetCache(typeof message.serverId === 'string' ? message.serverId : undefined)
    );
  }
});

function isAssetProxyServerMessage(value: unknown): value is AssetProxyServer {
  if (!value || typeof value !== 'object') return false;
  const server = value as Partial<AssetProxyServer>;
  return (
    typeof server.id === 'string' &&
    typeof server.url === 'string' &&
    (typeof server.token === 'string' || server.token === null || server.token === undefined)
  );
}

function isAssetProxyTargetMessage(value: unknown): value is AssetProxyTarget {
  if (!value || typeof value !== 'object') return false;
  const target = value as Partial<AssetProxyTarget>;
  return (
    typeof target.serverId === 'string' &&
    typeof target.virtualPath === 'string' &&
    typeof target.targetUrl === 'string'
  );
}

function syncAssetProxyServers(servers: unknown[]): void {
  assetProxyServers.clear();
  mergeAssetProxyServers(servers);
}

function mergeAssetProxyServers(servers: unknown[]): void {
  for (const server of servers) {
    if (!isAssetProxyServerMessage(server)) continue;
    assetProxyServers.set(server.id, {
      id: server.id,
      url: server.url,
      token: typeof server.token === 'string' ? server.token : null
    });
  }
}

function registerAssetProxyTarget(target: AssetProxyTarget): void {
  registeredAssetTargets.set(target.virtualPath, target);
}

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
        return cached ?? fetch(event.request);
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

type AssetProxyRequest = {
  serverId: string;
  virtualPath: string;
  assetPath: string;
};

function parseAssetProxyRequest(requestUrl: string, origin: string): AssetProxyRequest | null {
  const url = new URL(requestUrl);
  if (url.origin !== origin) return null;
  if (!url.pathname.startsWith(ASSET_PROXY_PATH_PREFIX)) return null;

  const rest = url.pathname.slice(ASSET_PROXY_PATH_PREFIX.length);
  const slashIndex = rest.indexOf('/');
  if (slashIndex <= 0) return null;

  const serverId = decodeURIComponent(rest.slice(0, slashIndex));
  const assetPath = `/${rest.slice(slashIndex + 1)}`;
  if (!assetPath.startsWith('/assets/files/')) return null;

  return {
    serverId,
    virtualPath: url.pathname,
    assetPath
  };
}

async function handleAssetProxyFetch(
  request: Request,
  proxyRequest: AssetProxyRequest
): Promise<Response> {
  if (request.method !== 'GET') {
    return new Response('Method not allowed', { status: 405 });
  }

  const cache = await caches.open(ASSET_CACHE_NAME);
  const cacheKey = assetProxyCacheKey(request.url);
  const rangeHeader = request.headers.get('Range');
  if (!rangeHeader) {
    const cached = await cache.match(cacheKey);
    if (cached) return cached;
  }

  let server = assetProxyServers.get(proxyRequest.serverId);
  let registered = registeredAssetTargets.get(proxyRequest.virtualPath);
  if (!server || !registered) {
    await requestAssetProxyResync(proxyRequest);
    server = assetProxyServers.get(proxyRequest.serverId);
    registered = registeredAssetTargets.get(proxyRequest.virtualPath);
  }

  const targetUrl =
    registered?.targetUrl ?? buildFallbackAssetTarget(server, proxyRequest.assetPath);
  if (!targetUrl) {
    return new Response('Asset target is not registered', { status: 404 });
  }

  if (rangeHeader) {
    return Response.redirect(targetUrl, 302);
  }

  const headers = new Headers();
  if (server?.token) {
    headers.set('Authorization', `Bearer ${server.token}`);
  }
  headers.set('X-Chatto-Asset-Proxy', '1');

  const networkResponse = await fetch(targetUrl, {
    headers,
    credentials: server?.token ? 'omit' : 'include',
    redirect: 'follow'
  });
  const response = new Response(networkResponse.body, {
    status: networkResponse.status,
    statusText: networkResponse.statusText,
    headers: networkResponse.headers
  });

  if (response.ok && response.status === 200) {
    await cache.put(cacheKey, response.clone());
    await pruneAssetCache(cache);
  }

  return response;
}

async function requestAssetProxyResync(proxyRequest: AssetProxyRequest): Promise<void> {
  const clients = await self.clients.matchAll({
    type: 'window',
    includeUncontrolled: true
  });
  if (clients.length === 0) return;

  await Promise.race([
    Promise.all(clients.map((client) => requestAssetProxyResyncFromClient(client, proxyRequest))),
    new Promise<void>((resolve) => setTimeout(resolve, ASSET_PROXY_RESYNC_TIMEOUT_MS))
  ]);
}

async function requestAssetProxyResyncFromClient(
  client: Client,
  proxyRequest: AssetProxyRequest
): Promise<void> {
  return new Promise((resolve) => {
    const channel = new MessageChannel();
    const timeout = setTimeout(resolve, ASSET_PROXY_RESYNC_TIMEOUT_MS);

    channel.port1.onmessage = (event) => {
      clearTimeout(timeout);
      applyAssetProxyResyncResponse(event.data);
      resolve();
    };

    try {
      client.postMessage(
        {
          type: 'chatto-asset-proxy-resync-request',
          serverId: proxyRequest.serverId,
          virtualPath: proxyRequest.virtualPath
        },
        [channel.port2]
      );
    } catch {
      clearTimeout(timeout);
      resolve();
    }
  });
}

function applyAssetProxyResyncResponse(message: unknown): void {
  if (!message || typeof message !== 'object') return;
  const response = message as Record<string, unknown>;
  if (response.type !== 'chatto-asset-proxy-resync-response') return;

  if (Array.isArray(response.servers)) {
    mergeAssetProxyServers(response.servers);
  }

  if (Array.isArray(response.targets)) {
    for (const target of response.targets) {
      if (!isAssetProxyTargetMessage(target)) continue;
      registerAssetProxyTarget(target);
    }
  }
}

function buildFallbackAssetTarget(
  server: AssetProxyServer | undefined,
  assetPath: string
): string | null {
  if (!server) return null;
  try {
    return new URL(assetPath, server.url).href;
  } catch {
    return null;
  }
}

function assetProxyCacheKey(requestUrl: string): Request {
  const url = new URL(requestUrl);
  url.search = '';
  url.hash = '';
  return new Request(url.href, { method: 'GET' });
}

async function pruneAssetCache(cache: Cache): Promise<void> {
  const keys = await cache.keys();
  if (keys.length <= MAX_ASSET_CACHE_ENTRIES) return;
  await Promise.all(
    keys.slice(0, keys.length - MAX_ASSET_CACHE_ENTRIES).map((key) => cache.delete(key))
  );
}

async function clearAssetCache(serverId?: string): Promise<void> {
  if (!serverId) {
    registeredAssetTargets.clear();
    await caches.delete(ASSET_CACHE_NAME);
    return;
  }

  const serverPrefix = `${ASSET_PROXY_PATH_PREFIX}${encodeURIComponent(serverId)}/`;
  for (const [virtualPath, target] of registeredAssetTargets) {
    if (target.serverId === serverId || virtualPath.startsWith(serverPrefix)) {
      registeredAssetTargets.delete(virtualPath);
    }
  }

  const cache = await caches.open(ASSET_CACHE_NAME);
  const keys = await cache.keys();
  await Promise.all(
    keys
      .filter((key) => new URL(key.url).pathname.startsWith(serverPrefix))
      .map((key) => cache.delete(key))
  );
}

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
