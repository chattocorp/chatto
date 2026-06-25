import type { TypedDocumentNode } from '@graphql-typed-document-node/core';
import type { AnyVariables } from '@urql/svelte';
import { useConnection } from '$lib/state/server/connection.svelte';

/**
 * Result type for mutation execution.
 */
export interface MutationResult<TData> {
  /** The mutation result data if successful */
  data: TData | undefined;
  /** Error message if the mutation failed */
  error: string | undefined;
}

/**
 * Options for useMutation hook.
 */
export interface UseMutationOptions<TData> {
  /**
   * Callback invoked when mutation completes successfully.
   */
  onCompleted?: (data: TData) => void;

  /**
   * Callback invoked when mutation fails with an error.
   */
  onError?: (error: string) => void;
}

/**
 * Return type for useMutation hook.
 */
export interface UseMutationReturn<TData, TVariables extends AnyVariables> {
  /** The last mutation result data */
  readonly data: TData | undefined;
  /** True while the mutation is in flight */
  readonly loading: boolean;
  /** Error message if the last mutation failed */
  readonly error: string | undefined;
  /** Execute the mutation with the given variables */
  execute: (variables: TVariables) => Promise<MutationResult<TData>>;
  /** Reset the mutation state (data, error) to initial values */
  reset: () => void;
}

/**
 * GraphQL mutation hook with loading and error state tracking.
 *
 * Unlike useQuery, mutations are user-triggered (not reactive), so this
 * hook returns an `execute` function instead of running automatically.
 *
 * @example
 * ```typescript
 * // Basic usage
 * const { execute, loading, error } = useMutation(CreateSpaceMutation);
 *
 * async function handleSubmit() {
 *   const result = await execute({ name: spaceName, description });
 *   if (result.data) {
 *     goto(`/chat/${result.data.createSpace.id}`);
 *   }
 * }
 *
 * // With callbacks
 * const { execute, loading } = useMutation(DeleteItemMutation, {
 *   onCompleted: () => toast.success('Item deleted'),
 *   onError: (error) => toast.error(error)
 * });
 * ```
 */
export function useMutation<TData, TVariables extends AnyVariables>(
  mutation: TypedDocumentNode<TData, TVariables>,
  options?: UseMutationOptions<TData>
): UseMutationReturn<TData, TVariables> {
  let data = $state<TData | undefined>(undefined);
  let loading = $state(false);
  let error = $state<string | undefined>(undefined);
  const connection = useConnection();

  async function execute(variables: TVariables): Promise<MutationResult<TData>> {
    loading = true;
    error = undefined;

    try {
      const result = await connection().client.mutation(mutation, variables).toPromise();

      if (result.error) {
        const errorMsg = result.error.message;
        error = errorMsg;
        options?.onError?.(errorMsg);
        return { data: undefined, error: errorMsg };
      }

      data = result.data;
      if (result.data) {
        options?.onCompleted?.(result.data);
      }
      return { data: result.data, error: undefined };
    } catch (e) {
      const errorMsg = e instanceof Error ? e.message : 'Mutation failed';
      error = errorMsg;
      options?.onError?.(errorMsg);
      return { data: undefined, error: errorMsg };
    } finally {
      loading = false;
    }
  }

  function reset() {
    data = undefined;
    error = undefined;
  }

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
    execute,
    reset
  };
}
