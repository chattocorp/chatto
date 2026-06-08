import type { RegisteredServer } from '$lib/state/server/registry.svelte';

export const ASSET_PROXY_PATH_PREFIX = '/__chatto/assets/';

export type AssetProxyServer = {
  id: string;
  url: string;
  token: string | null;
};

type AssetProxyMessage =
  | {
      type: 'chatto-asset-proxy-sync-servers';
      servers: AssetProxyServer[];
    }
  | {
      type: 'chatto-asset-proxy-register-url';
      serverId: string;
      virtualPath: string;
      targetUrl: string;
    }
  | {
      type: 'chatto-asset-proxy-clear-cache';
      serverId?: string;
    };

export function assetProxyController(): ServiceWorker | null {
  if (typeof navigator === 'undefined' || !('serviceWorker' in navigator)) return null;
  return navigator.serviceWorker.controller;
}

export function isAssetProxyAvailable(): boolean {
  return assetProxyController() !== null;
}

function postAssetProxyMessage(message: AssetProxyMessage): void {
  assetProxyController()?.postMessage(message);
}

export function syncAssetProxyServers(servers: readonly RegisteredServer[]): void {
  postAssetProxyMessage({
    type: 'chatto-asset-proxy-sync-servers',
    servers: servers.map((server) => ({
      id: server.id,
      url: server.url,
      token: server.token
    }))
  });
}

export function clearAssetProxyCache(serverId?: string): void {
  postAssetProxyMessage({
    type: 'chatto-asset-proxy-clear-cache',
    ...(serverId ? { serverId } : {})
  });
}

export function buildVirtualAssetPath(serverId: string, assetPathname: string): string {
  const normalizedPath = assetPathname.startsWith('/') ? assetPathname.slice(1) : assetPathname;
  return `${ASSET_PROXY_PATH_PREFIX}${encodeURIComponent(serverId)}/${normalizedPath}`;
}

export function registerAssetProxyUrl(
  serverId: string,
  virtualPath: string,
  targetUrl: string
): void {
  postAssetProxyMessage({
    type: 'chatto-asset-proxy-register-url',
    serverId,
    virtualPath,
    targetUrl
  });
}
