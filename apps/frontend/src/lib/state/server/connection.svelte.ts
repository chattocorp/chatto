import { untrack } from 'svelte';
import type { ServerConnection } from './serverConnection.svelte';
import { useServerScope } from './scope.svelte';

/**
 * Get the current server scope's connection getter. Call during component init.
 *
 * Returns a function that, when invoked, returns the current `ServerConnection`
 * for the active instance. The read is **untracked** — safe to call inside
 * `$effect` and `$derived` without creating a dependency on which instance
 * is active.
 */
export function useConnection(): () => ServerConnection {
  const serverScope = useServerScope();
  return () => untrack(() => serverScope.connection);
}
