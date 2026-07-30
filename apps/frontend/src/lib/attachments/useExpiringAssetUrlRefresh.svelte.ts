import { onMount, untrack } from 'svelte';

type ExpiringAssetUrlRefreshOptions = {
	getRefreshAt: () => number | null;
	hasStaleUrl: () => boolean;
	refresh: () => void | Promise<unknown>;
	errorMessage: string;
	refreshOnFocus?: boolean;
};

/**
 * Refresh expiring asset URLs at their next deadline and after a suspended tab
 * becomes active again.
 *
 * Getters keep the caller's reactive URL state owned by the component or store
 * while this hook owns only browser lifecycle orchestration.
 */
export function useExpiringAssetUrlRefresh({
	getRefreshAt,
	hasStaleUrl,
	refresh,
	errorMessage,
	refreshOnFocus = true
}: ExpiringAssetUrlRefreshOptions): void {
	async function runRefresh(): Promise<void> {
		try {
			await refresh();
		} catch (error: unknown) {
			console.warn(errorMessage, error);
		}
	}

	function refreshIfStale(): void {
		if (hasStaleUrl()) void runRefresh();
	}

	$effect(() => {
		const refreshAt = getRefreshAt();
		const hasStale = hasStaleUrl();
		if (refreshAt === null) return;

		const delay = refreshAt - Date.now();
		if (delay <= 0 && hasStale) {
			untrack(() => void runRefresh());
			return;
		}

		const timeout = window.setTimeout(() => void runRefresh(), delay);
		return () => window.clearTimeout(timeout);
	});

	onMount(() => {
		const handleVisibilityChange = () => {
			if (document.visibilityState === 'visible') refreshIfStale();
		};

		if (refreshOnFocus) window.addEventListener('focus', refreshIfStale);
		document.addEventListener('visibilitychange', handleVisibilityChange);

		return () => {
			if (refreshOnFocus) window.removeEventListener('focus', refreshIfStale);
			document.removeEventListener('visibilitychange', handleVisibilityChange);
		};
	});
}
