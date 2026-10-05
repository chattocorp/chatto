import { ownerOfServerId } from '../server/serverIds.js';

const STABLE_ASSET_PATH_PREFIX = '/assets/files/';

function isAssetPath(pathname: string): boolean {
  return pathname.startsWith(STABLE_ASSET_PATH_PREFIX);
}

export function assetUrlForServer(
  serverId: string,
  rawUrl: string | null | undefined
): string | null {
  if (!rawUrl) return null;

  const server = ownerOfServerId(serverId)?.getServer(serverId);
  if (!server) return rawUrl;

  try {
    const serverOrigin = new URL(server.url).origin;
    const parsed = rawUrl.startsWith('/') ? new URL(rawUrl, serverOrigin) : new URL(rawUrl);

    if (!isAssetPath(parsed.pathname)) {
      return rawUrl;
    }

    // A browser page on the same origin uses a relative path. Other hosts,
    // such as bots in Node, need the absolute URL.
    if (typeof window !== 'undefined' && parsed.origin === window.location.origin) {
      return `${parsed.pathname}${parsed.search}`;
    }
    return parsed.href;
  } catch {
    return rawUrl;
  }
}
