/// <reference lib="webworker" />

import {
  ASSET_PROXY_PATH_PREFIX,
  type AssetProxyServer,
  type AssetProxyTarget
} from './assetProxy.shared';

declare const self: ServiceWorkerGlobalScope;

const ASSET_CACHE_NAME = 'chatto-assets-v1';
const ASSET_PROXY_RESYNC_TIMEOUT_MS = 750;

const assetProxyServers = new Map<string, AssetProxyServer>();
const registeredAssetTargets = new Map<string, AssetProxyTarget>();

export type AssetProxyRequest = {
  serverId: string;
  virtualPath: string;
  assetPath: string;
};

type AssetProxyFetchOptions = {
  navigationFallback?: () => Promise<Response | undefined>;
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
  proxyRequest: AssetProxyRequest,
  options: AssetProxyFetchOptions = {}
): Promise<Response> {
  if (request.method !== 'GET') {
    return assetProxyErrorResponse(request, options, 'Method not allowed', 405);
  }

  let server = assetProxyServers.get(proxyRequest.serverId);
  let registered = matchingRegisteredAssetTarget(proxyRequest);
  if (!server || !registered) {
    await requestAssetProxyResync(proxyRequest);
    server = assetProxyServers.get(proxyRequest.serverId);
    registered = matchingRegisteredAssetTarget(proxyRequest);
  }
  if (!server) {
    return assetProxyErrorResponse(request, options, 'Asset target is not registered', 404);
  }

  const targetUrl =
    registered?.targetUrl ?? buildFallbackAssetTarget(server, proxyRequest.assetPath);
  if (!targetUrl) {
    return assetProxyErrorResponse(request, options, 'Asset target is not registered', 404);
  }

  const rangeHeader = request.headers.get('Range');
  if (rangeHeader) {
    return Response.redirect(targetUrl, 302);
  }

  const headers = new Headers();
  headers.set('X-Chatto-Asset-Proxy', '1');

  let networkResponse: Response;
  try {
    networkResponse = await fetch(targetUrl, {
      headers,
      credentials: sameOrigin(targetUrl, self.location.origin) ? 'include' : 'omit',
      redirect: 'follow'
    });
  } catch {
    return assetProxyErrorResponse(request, options, 'Asset target is not available', 502);
  }

  if (request.mode === 'navigate' && !networkResponse.ok) {
    const fallback = await options.navigationFallback?.();
    if (fallback) return fallback;
  }

  return new Response(networkResponse.body, {
    status: networkResponse.status,
    statusText: networkResponse.statusText,
    headers: networkResponse.headers
  });
}

async function assetProxyErrorResponse(
  request: Request,
  options: AssetProxyFetchOptions,
  message: string,
  status: number
): Promise<Response> {
  if (request.mode === 'navigate') {
    const fallback = await options.navigationFallback?.();
    if (fallback) return fallback;
  }
  return new Response(message, { status });
}

function isAssetProxyServerMessage(value: unknown): value is AssetProxyServer {
  if (!value || typeof value !== 'object') return false;
  const server = value as Partial<AssetProxyServer>;
  return typeof server.id === 'string' && typeof server.url === 'string';
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
      url: server.url
    });
  }
}

function registerAssetProxyTarget(target: AssetProxyTarget): void {
  registeredAssetTargets.set(target.virtualPath, target);
}

function matchingRegisteredAssetTarget(
  proxyRequest: AssetProxyRequest
): AssetProxyTarget | undefined {
  const registered = registeredAssetTargets.get(proxyRequest.virtualPath);
  if (registered?.serverId !== proxyRequest.serverId) return undefined;
  return registered;
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

function sameOrigin(value: string, origin: string): boolean {
  try {
    return new URL(value).origin === origin;
  } catch {
    return false;
  }
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
