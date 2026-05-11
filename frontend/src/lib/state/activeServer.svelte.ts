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

/**
 * Returns a getter for the active instance's primary space ID. Convenience
 * over reaching into the registry directly. Used by admin/settings pages
 * that still need a space ID for GraphQL queries; goes away once those
 * queries stop requiring one (the post-#330 API surface retires
 * `Query.space(id:)` and friends in favour of instance-level resolvers).
 *
 * Returns an empty string while the instance store is loading or absent.
 */
export function getActiveServerSpaceId(): () => string {
  const getId = getActiveServer();
  return () => serverRegistry.tryGetStore(getId())?.instance.primarySpaceId ?? '';
}
