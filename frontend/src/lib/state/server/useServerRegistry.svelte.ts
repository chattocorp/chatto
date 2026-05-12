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
	serverRegistry.probeOrigin(!!getUser());

	// Re-fetch server info when the tab becomes visible
	$effect(() => {
		const originId = serverRegistry.originServer?.id;
		if (!originId) return;

		const onVisibilityChange = () => {
			if (document.visibilityState === 'visible') {
				serverRegistry.getStore(originId).serverInfo.init();
			}
		};

		document.addEventListener('visibilitychange', onVisibilityChange);
		return () => document.removeEventListener('visibilitychange', onVisibilityChange);
	});
}
