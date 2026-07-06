import { serverRegistry } from '$lib/state/server/registry.svelte';

const STABLE_ASSET_PATH_PREFIX = '/assets/files/';

function isAssetPath(pathname: string): boolean {
  return pathname.startsWith(STABLE_ASSET_PATH_PREFIX);
}

export function assetUrlForServer(
  serverId: string,
  rawUrl: string | null | undefined
): string | null {
  if (!rawUrl) return null;
  if (typeof window === 'undefined') return rawUrl;

  const server = serverRegistry.getServer(serverId);
  if (!server) return rawUrl;

  try {
    const serverOrigin = new URL(server.url).origin;
    const parsed = rawUrl.startsWith('/') ? new URL(rawUrl, serverOrigin) : new URL(rawUrl);

    if (!isAssetPath(parsed.pathname)) {
      return rawUrl;
    }

    if (parsed.origin === window.location.origin) {
      return `${parsed.pathname}${parsed.search}`;
    }
    return parsed.href;
  } catch {
    return rawUrl;
  }
}
