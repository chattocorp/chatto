import { onMount } from 'svelte';

// eslint-disable-next-line svelte/prefer-svelte-reactivity -- module-level callback registry, not read reactively
const callbacks = new Set<() => void>();

if (typeof document !== 'undefined') {
	document.addEventListener('visibilitychange', () => {
		if (document.visibilityState === 'visible') {
			for (const cb of callbacks) cb();
		}
	});
}

/**
 * Run a callback on mount and whenever the browser tab becomes visible again.
 *
 * Useful for loading state that may become stale while the tab is hidden
 * (e.g., active call participants, instance config). Fires immediately on
 * mount for the initial load, then again each time the user returns to the tab.
 *
 * Must be called during component initialization.
 */
export function useTabResumeCallback(callback: () => void) {
	onMount(() => {
		callback();
		callbacks.add(callback);
		return () => callbacks.delete(callback);
	});
}

