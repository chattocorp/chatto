import { serverRegistry } from '$lib/state/server/registry.svelte';

/** URL segment used for the home (origin) instance. */
const HOME_SEGMENT = '-';

/**
 * Convert an internal instance registry ID to a URL segment.
 * Origin instance → "-", remote → raw hostname from URL.
 */
export function serverIdToSegment(serverId: string): string {
	if (serverRegistry.isOriginInstance(serverId)) return HOME_SEGMENT;

	const instance = serverRegistry.getInstance(serverId);
	if (!instance) return HOME_SEGMENT;

	try {
		return new URL(instance.url).hostname;
	} catch {
		return HOME_SEGMENT;
	}
}

/**
 * Convert a URL segment back to an internal instance registry ID.
 * "-" → origin instance, hostname → find matching instance by URL.
 */
export function segmentToServerId(segment: string): string | null {
	if (segment === HOME_SEGMENT) {
		return serverRegistry.originServer?.id ?? null;
	}

	// Find instance whose URL hostname matches the segment
	for (const instance of serverRegistry.instances) {
		try {
			if (new URL(instance.url).hostname === segment) {
				return instance.id;
			}
		} catch {
			continue;
		}
	}

	return null;
}
