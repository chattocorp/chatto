import type { RegisteredServer } from '$lib/state/server/registry.svelte';
import { csrfFetch } from './csrf';

function logoutUrl(server: RegisteredServer): string {
	return new URL('/auth/logout', server.url).toString();
}

/**
 * Best-effort server-side logout for a registered server.
 *
 * Callers intentionally continue with local cleanup if this rejects, so users
 * can escape stale or unreachable server registrations.
 */
export function signOutServer(server: RegisteredServer, isOriginServer: boolean): Promise<Response> {
	const headers = server.token ? { Authorization: `Bearer ${server.token}` } : undefined;

	if (isOriginServer) {
		return csrfFetch('/auth/logout', {
			method: 'POST',
			headers
		});
	}

	return fetch(logoutUrl(server), {
		method: 'POST',
		headers
	});
}

export async function signOutServers(
	servers: RegisteredServer[],
	isOriginServer: (serverId: string) => boolean
): Promise<void> {
	await Promise.all(
		servers.map((server) =>
			signOutServer(server, isOriginServer(server.id)).catch(() => undefined)
		)
	);
}

export function hardRedirectAfterSignOut(href = '/'): void {
	window.location.href = href;
}
