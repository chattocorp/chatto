/// <reference lib="webworker" />

import {
  ASSET_PROXY_PATH_PREFIX,
  type AssetProxyServer,
  type AssetProxyTarget
} from './assetProxy.shared';

declare const self: ServiceWorkerGlobalScope;

const ASSET_CACHE_NAME = 'chatto-assets-v1';
const MAX_ASSET_CACHE_ENTRIES = 250;
const ASSET_PROXY_RESYNC_TIMEOUT_MS = 750;

const assetProxyServers = new Map<string, AssetProxyServer>();
const registeredAssetTargets = new Map<string, AssetProxyTarget>();

export type AssetProxyRequest = {
  serverId: string;
  virtualPath: string;
  assetPath: string;
};

export function handleAssetProxyMessage(event: ExtendableMessageEvent): boolean {
  const message = event.data as Record<string, unknown> | undefined;
  if (!message || typeof message.type !== 'string') return false;

  if (message.type === 'chatto-asset-proxy-sync-servers' && Array.isArray(message.servers)) {
    syncAssetProxyServers(message.servers);
    return true;
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
    return true;
  }

  if (message.type === 'chatto-asset-proxy-clear-cache') {
    event.waitUntil(
      clearAssetCache(typeof message.serverId === 'string' ? message.serverId : undefined)
    );
    return true;
  }

  return false;
}

export function parseAssetProxyRequest(
  requestUrl: string,
  origin: string
): AssetProxyRequest | null {
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

export async function handleAssetProxyFetch(
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
