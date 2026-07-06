import { onMount } from 'svelte';
import { getActiveServer } from '$lib/state/activeServer.svelte';
import { registerServerResumeCallback } from './resumeCoordinator.svelte';

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
    return registerServerResumeCallback(getActiveServer(), () => callback());
  });
}
