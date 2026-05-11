import { createContext } from 'svelte';
import { untrack } from 'svelte';
import type { GraphQLClient } from './graphqlClient.svelte';

export const [getConnectionCtx, provideConnection] = createContext<() => GraphQLClient>();

/**
 * Get a connection getter from context. Call during component init.
 *
 * Returns a function that, when invoked, returns the current `GraphQLClient`
 * for the active instance. The read is **untracked** — safe to call inside
 * `$effect` and `$derived` without creating a dependency on which instance
 * is active.
 */
export function useConnection(): () => GraphQLClient {
	const getter = getConnectionCtx();
	return () => untrack(getter);
}
