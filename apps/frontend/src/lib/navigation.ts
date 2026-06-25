import { serverRegistry } from '$lib/state/server/registry.svelte';

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

	// Find the server whose URL hostname matches the segment
	for (const server of serverRegistry.servers) {
		try {
			if (new URL(server.url).hostname === segment) {
				return server.id;
			}
		} catch {
			continue;
		}
	}

	return null;
}
