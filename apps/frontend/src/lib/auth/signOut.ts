import type { RegisteredServer } from '$lib/state/server/registry.svelte';
import { csrfFetch } from './csrf';

export const SIGN_OUT_TIMEOUT_MS = 5000;

function logoutUrl(server: RegisteredServer): string {
	return new URL('/auth/logout', server.url).toString();
}

function withSignOutTimeout<T>(request: (signal: AbortSignal) => Promise<T>): Promise<T> {
	const controller = new AbortController();
	const timeoutId = setTimeout(() => controller.abort(), SIGN_OUT_TIMEOUT_MS);
	return request(controller.signal).finally(() => clearTimeout(timeoutId));
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
		return withSignOutTimeout((signal) => csrfFetch('/auth/logout', {
			method: 'POST',
			headers,
			signal
		}));
	}

	return withSignOutTimeout((signal) => fetch(logoutUrl(server), {
		method: 'POST',
		headers,
		signal
	}));
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
