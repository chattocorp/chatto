import { createContext } from 'svelte';
import { page } from '$app/state';
import { segmentToServerId } from '$lib/navigation';
import { serverRegistry } from './server/registry.svelte';

/**
 * Svelte context for the active instance ID.
 *
 * Provided by the root layout via {@link provideActiveServerFromUrl} and
 * available to every descendant. The value is a getter function — call it
 * inside a reactive context ($derived / $effect / template) to track URL
 * changes. Must be looked up during component initialization.
 */
export const [getActiveServer, setActiveServer] = createContext<() => string>();

/**
 * Resolves the active instance ID from the URL and provides it via context.
 * Origin segment ("-") and instance-agnostic routes both fall back to the
 * origin instance.
 */
export function provideActiveServerFromUrl(): void {
  setActiveServer(
    () =>
      segmentToServerId(page.params.serverId ?? '-') ??
      serverRegistry.originServer?.id ??
      ''
  );
}
