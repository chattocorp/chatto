import { serverRegistry } from '$lib/state/server/registry.svelte';

const ASSET_PATH_PREFIXES = ['/assets/files/', '/assets/attachments/'];

function isAssetPath(pathname: string): boolean {
	return ASSET_PATH_PREFIXES.some((prefix) => pathname.startsWith(prefix));
}

export function assetUrlForServer(serverId: string, rawUrl: string | null | undefined): string | null {
	if (!rawUrl) return null;
	if (typeof window === 'undefined') return rawUrl;

	const server = serverRegistry.getServer(serverId);
	if (!server) return rawUrl;

	try {
		const serverOrigin = new URL(server.url).origin;
		const parsed = rawUrl.startsWith('/')
			? new URL(rawUrl, serverOrigin)
			: new URL(rawUrl);

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
