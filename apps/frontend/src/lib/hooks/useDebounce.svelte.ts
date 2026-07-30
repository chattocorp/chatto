import { onDestroy } from 'svelte';

/**
 * Own a replaceable timeout for callbacks scheduled by a component.
 */
export function useDebounce() {
	let timeout: ReturnType<typeof setTimeout> | undefined;

	function cancel(): void {
		clearTimeout(timeout);
		timeout = undefined;
	}

	function run(callback: () => void, delay: number): void {
		cancel();
		timeout = setTimeout(() => {
			timeout = undefined;
			callback();
		}, delay);
	}

	onDestroy(cancel);
	return { cancel, run };
}
