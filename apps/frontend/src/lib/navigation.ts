import { serverRegistry } from '$lib/client';

/** URL segment used for the home (origin) server. */
const HOME_SEGMENT = '-';

/**
 * Convert an internal server registry ID to a URL segment.
 * Origin server → "-", remote → raw hostname from URL.
 */
export function serverIdToSegment(serverId: string): string {
  if (serverRegistry.isOriginServer(serverId)) return HOME_SEGMENT;

  const server = serverRegistry.getServer(serverId);
  if (!server) return HOME_SEGMENT;

  try {
    return new URL(server.url).hostname;
  } catch {
    return HOME_SEGMENT;
  }
}

/**
 * Convert a URL segment back to an internal server registry ID.
 * "-" → origin server, hostname → find matching server by URL.
 */
export function segmentToServerId(segment: string): string | null {
  if (segment === HOME_SEGMENT) {
    return serverRegistry.originServer?.id ?? null;
  }

  return serverIdForHostname(segment);
}

/** Find a registered server by hostname, independent of scheme and port. */
export function serverIdForHostname(hostname: string): string | null {
  for (const server of serverRegistry.servers) {
    try {
      if (new URL(server.url).hostname === hostname) {
        return server.id;
      }
    } catch {
      continue;
    }
  }

  return null;
}
