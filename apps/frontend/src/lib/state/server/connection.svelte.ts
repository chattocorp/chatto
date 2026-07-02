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

/**
 * Get a connection getter that tracks the caller-provided scope before reading
 * the untracked connection context.
 *
 * Use this for route-derived server scopes where the connection should refresh
 * when the active URL server changes. Keep `useConnection()` for places that
 * deliberately need a stable, untracked snapshot.
 */
export function useTrackedConnection(track: () => unknown): () => ServerConnection {
	const getter = getConnectionCtx();
	return () => {
		track();
		return untrack(getter);
	};
}
