import { serverRegistry } from './registry.svelte';

/**
 * Bootstrap the server registry: create stores, probe the origin,
 * and re-fetch server info when the tab resumes.
 *
 * Must be called during component initialization (root layout script).
 *
 * @param getUser - Getter returning the current user (truthy = known server,
 *   falsy = probe needed). Passed as a getter so reads happen inside `$effect`.
 */
export function useServerRegistry(getUser: () => unknown): void {
	serverRegistry.init();
	const hasUser = !!getUser();
	serverRegistry.probeOrigin(hasUser);
	if (!hasUser) {
		serverRegistry.settleOriginUnauthenticated();
	}

	// Re-fetch server info when the tab becomes visible
	$effect(() => {
		const originId = serverRegistry.originServer?.id;
		if (!originId) return;

		const onVisibilityChange = () => {
			if (document.visibilityState === 'visible') {
				const store = serverRegistry.getStore(originId);
				void store.serverInfo.init();
				if (store.isAuthenticated) {
					store.serverInfo.refreshAuthenticatedSettings().catch((err) => {
						console.error(
							`[server:${store.serverId}] failed to refresh authenticated server settings`,
							err
						);
					});
				}
			}
		};

		document.addEventListener('visibilitychange', onVisibilityChange);
		return () => document.removeEventListener('visibilitychange', onVisibilityChange);
	});
}
