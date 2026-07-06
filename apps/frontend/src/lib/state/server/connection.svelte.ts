import { createContext } from 'svelte';
import { untrack } from 'svelte';
import type { ServerConnection } from './serverConnection.svelte';

export const [getConnectionCtx, provideConnection] = createContext<() => ServerConnection>();

/**
 * Get a connection getter from context. Call during component init.
 *
 * Returns a function that, when invoked, returns the current `ServerConnection`
 * for the active instance. The read is **untracked** — safe to call inside
 * `$effect` and `$derived` without creating a dependency on which instance
 * is active.
 */
export function useConnection(): () => ServerConnection {
	const getter = getConnectionCtx();
	return () => untrack(getter);
}
