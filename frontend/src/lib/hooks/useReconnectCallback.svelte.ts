import { useConnection } from '$lib/state/server/connection.svelte';

/**
 * Run a callback whenever the WebSocket reconnects after a disconnection.
 *
 * Unlike useTabResumeCallback, this does NOT fire on mount — only on actual
 * reconnections. Use this for filling gaps in event streams (e.g., invalidating
 * the message cache) where the trigger should be "we may have missed events",
 * not "the tab became visible".
 *
 * Instance switches do NOT trigger the callback because `useConnection()` reads
 * the active instance via `untrack` — the `reconnectCount` $state is tracked,
 * but the instance ID is not.
 *
 * Must be called during component initialization.
 */
export function useReconnectCallback(callback: () => void) {
	const connection = useConnection();
	let prevCount: number | undefined;

	$effect(() => {
		const count = connection().reconnectCount;

		if (prevCount !== undefined && count !== prevCount) {
			callback();
		}
		prevCount = count;
	});
}

/**
 * Reactive counter that increments only on genuine WebSocket reconnections.
 *
 * Use `void trigger.count` inside $effect or $derived to re-run on reconnect
 * without false-triggering on instance switches.
 *
 * Must be called during component initialization.
 */
export function useReconnectTrigger(): { readonly count: number } {
	let count = $state(0);
	useReconnectCallback(() => {
		count++;
	});
	return {
		get count() {
			return count;
		}
	};
}
