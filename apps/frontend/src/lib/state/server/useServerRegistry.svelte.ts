import { serverRegistry } from './registry.svelte';

/**
 * Bootstrap the server registry: create stores and probe the origin.
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
}
