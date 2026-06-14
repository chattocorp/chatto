import type { RegisteredServer } from '$lib/state/server/registry.svelte';
import {
  ASSET_PROXY_PATH_PREFIX,
  type AssetProxyServer,
  type AssetProxyTarget
} from './assetProxy.shared';

export { ASSET_PROXY_PATH_PREFIX };
export type { AssetProxyServer, AssetProxyTarget };

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

const registeredAssetTargets = new Map<string, AssetProxyTarget>();

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

function serializeAssetProxyServers(servers: readonly RegisteredServer[]): AssetProxyServer[] {
  return servers.map((server) => ({
    id: server.id,
    url: server.url,
    token: server.token
  }));
}

export function syncAssetProxyServers(servers: readonly RegisteredServer[]): void {
  postAssetProxyMessage({
    type: 'chatto-asset-proxy-sync-servers',
    servers: serializeAssetProxyServers(servers)
  });
}

export function clearAssetProxyCache(serverId?: string): void {
  if (serverId) {
    for (const [virtualPath, target] of registeredAssetTargets) {
      if (target.serverId === serverId) registeredAssetTargets.delete(virtualPath);
    }
  } else {
    registeredAssetTargets.clear();
  }

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
  registeredAssetTargets.set(virtualPath, { serverId, virtualPath, targetUrl });
  postAssetProxyMessage({
    type: 'chatto-asset-proxy-register-url',
    serverId,
    virtualPath,
    targetUrl
  });
}

export function installAssetProxyResyncHandler(
  getServers: () => readonly RegisteredServer[]
): () => void {
  if (typeof navigator === 'undefined' || !('serviceWorker' in navigator)) return () => {};

  const handleMessage = (event: MessageEvent) => {
    const message = event.data as Record<string, unknown> | undefined;
    const responsePort = event.ports[0];
    if (!responsePort || !message || message.type !== 'chatto-asset-proxy-resync-request') {
      return;
    }

    const requestedVirtualPath =
      typeof message.virtualPath === 'string' ? message.virtualPath : undefined;
    const targets = requestedVirtualPath
      ? [registeredAssetTargets.get(requestedVirtualPath)].filter(
          (target): target is AssetProxyTarget => Boolean(target)
        )
      : Array.from(registeredAssetTargets.values());

    responsePort.postMessage({
      type: 'chatto-asset-proxy-resync-response',
      servers: serializeAssetProxyServers(getServers()),
      targets
    });
  };

  navigator.serviceWorker.addEventListener('message', handleMessage);
  return () => navigator.serviceWorker.removeEventListener('message', handleMessage);
}
