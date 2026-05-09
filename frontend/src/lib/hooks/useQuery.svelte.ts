import type { TypedDocumentNode } from '@graphql-typed-document-node/core';
import type { AnyVariables } from '@urql/svelte';
import { useConnection } from '$lib/state/instance/connection.svelte';
import { useReconnectTrigger } from '$lib/hooks/useReconnectCallback.svelte';

/**
 * Options for useQuery hook.
 */
export interface UseQueryOptions<TData> {
	/**
	 * Reactive function that returns true when the query should be skipped.
	 * When skip returns true, the query won't execute and data will remain undefined.
	 */
	skip?: () => boolean;

	/**
	 * Callback invoked when query completes successfully.
	 * Useful for side effects that depend on the data.
	 */
	onCompleted?: (data: TData) => void;

	/**
	 * Callback invoked when query fails with an error.
	 */
	onError?: (error: string) => void;
}

/**
 * Return type for useQuery hook.
 */
export interface UseQueryReturn<TData> {
	/** The query result data, undefined until first successful fetch */
	readonly data: TData | undefined;
	/** True while the query is in flight */
	readonly loading: boolean;
	/** Error message if the query failed */
	readonly error: string | undefined;
	/** Re-execute the query manually (useful for event-triggered refreshes) */
	refetch: () => Promise<void>;
}

/**
 * Reactive GraphQL query hook with automatic race condition handling.
 *
 * Features:
 * - Automatically re-runs when variables change (reactive getter)
 * - Handles race conditions using request counter pattern
 * - Tracks loading and error state
 * - Provides manual refetch for event-triggered refreshes
 *
 * @example
 * ```typescript
 * // Basic usage
 * const { data, loading, error } = useQuery(
 *   GetUserQuery,
 *   () => ({ userId })
 * );
 *
 * // With skip condition
 * const { data, loading, error } = useQuery(
 *   GetRoomQuery,
 *   () => ({ roomId }),
 *   { skip: () => !roomId }
 * );
 *
 * // With callbacks
 * const { data, refetch } = useQuery(
 *   GetDataQuery,
 *   () => ({}),
 *   {
 *     onCompleted: (data) => console.log('Loaded:', data),
 *     onError: (error) => console.error('Failed:', error)
 *   }
 * );
 * ```
 */
export function useQuery<TData, TVariables extends AnyVariables>(
	query: TypedDocumentNode<TData, TVariables>,
	variables: () => TVariables,
	options?: UseQueryOptions<TData>
): UseQueryReturn<TData> {
	let data = $state<TData | undefined>(undefined);
	let loading = $state(true);
	let error = $state<string | undefined>(undefined);
	let requestId = 0; // Race condition counter
	const connection = useConnection();
	const reconnect = useReconnectTrigger();

	async function execute() {
		const currentId = ++requestId;
		loading = true;
		error = undefined;

		try {
			const result = await connection().client.query(query, variables()).toPromise();

			// Discard if a newer request has started
			if (currentId !== requestId) return;

			if (result.error) {
				const errorMsg = result.error.message;
				error = errorMsg;
				options?.onError?.(errorMsg);
			} else if (result.data) {
				data = result.data;
				options?.onCompleted?.(result.data);
			}
		} catch (e) {
			// Discard if a newer request has started
			if (currentId !== requestId) return;
			const errorMsg = e instanceof Error ? e.message : 'Query failed';
			error = errorMsg;
			options?.onError?.(errorMsg);
		} finally {
			// Only update loading if this is still the current request
			if (currentId === requestId) {
				loading = false;
			}
		}
	}

	$effect(() => {
		if (options?.skip?.()) {
			loading = false;
			return;
		}

		// Re-fetch after WebSocket reconnection — permissions, config, etc. may have changed
		void reconnect.count;

		execute();
	});

	return {
		get data() {
			return data;
		},
		get loading() {
			return loading;
		},
		get error() {
			return error;
		},
		refetch: execute
	};
}
