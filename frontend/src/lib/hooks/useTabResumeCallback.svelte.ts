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

/**
 * Default minimum hidden duration before {@link useTabResumeAfterGapCallback}
 * counts a visibility→visible transition as a "resume after gap." Mirrors
 * the eventBus visibility-resubscribe threshold and the GraphQL client's
 * suspend-detector threshold so all three layers react on the same horizon.
 */
const DEFAULT_RESUME_GAP_MS = 30_000;

/**
 * Run a callback only when the tab becomes visible after being hidden for
 * at least `gapMs` (default 30s). Unlike {@link useTabResumeCallback}, this
 * does NOT fire on mount and ignores quick tab toggles.
 *
 * Use this for refetching data that could have gone stale during a real
 * idle period (e.g., room messages missed during a lunch break or
 * background-tab sleep) without thrashing the network on every Alt+Tab.
 *
 * Pairs with {@link useReconnectCallback} as belt-and-suspenders: the
 * reconnect callback catches sleep/network-drop cycles via WebSocket
 * lifecycle; this callback catches the case where the tab was hidden long
 * enough to miss events but the WebSocket itself stayed up (or its
 * reconnect signal didn't propagate).
 *
 * Must be called during component initialization.
 */
export function useTabResumeAfterGapCallback(
	callback: () => void,
	gapMs: number = DEFAULT_RESUME_GAP_MS
) {
	onMount(() => {
		if (typeof document === 'undefined') return;
		let lastVisibleAt = Date.now();
		const handler = () => {
			if (document.visibilityState === 'visible') {
				const gap = Date.now() - lastVisibleAt;
				if (gap > gapMs) {
					console.log(
						'[useTabResumeAfterGap] visible after %ds hidden → firing callback',
						Math.round(gap / 1000)
					);
					callback();
				}
				lastVisibleAt = Date.now();
			} else {
				lastVisibleAt = Date.now();
			}
		};
		document.addEventListener('visibilitychange', handler);
		return () => document.removeEventListener('visibilitychange', handler);
	});
}

/**
 * Reactive counter that increments when {@link useTabResumeAfterGapCallback}
 * would fire. Mirror of {@link useReconnectTrigger} for the tab-resume signal,
 * so a `$effect` can `void` both counters in one place and react to either
 * trigger uniformly.
 *
 * Must be called during component initialization.
 */
export function useTabResumeAfterGapTrigger(gapMs?: number): { readonly count: number } {
	let count = $state(0);
	useTabResumeAfterGapCallback(() => {
		count++;
	}, gapMs);
	return {
		get count() {
			return count;
		}
	};
}
