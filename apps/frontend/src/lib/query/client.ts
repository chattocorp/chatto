/**
 * Svelte bindings for the shared client query cache.
 *
 * This replaces a `QueryClientProvider` in a layout. Measured on 2026-09-28, a
 * provider in the `/chat` layout loads TanStack with every chat page and adds
 * about 11 KiB gzip to the overview and room routes, which do not use queries.
 */

import {
  createInfiniteQuery as createInfiniteQueryWithClient,
  createMutation as createMutationWithClient,
  createQuery as createQueryWithClient,
  type Accessor,
  type QueryClient
} from '@tanstack/svelte-query';
import { queryClient } from './queryClient';

export * from './queryClient';

type CreateFunction = (options: never, client?: Accessor<QueryClient>) => unknown;

/** Bind a TanStack `create*` function to the shared client, keeping its types. */
function withSharedClient<F extends CreateFunction>(create: F): F {
  const bound: CreateFunction = (options, client) => create(options, client ?? (() => queryClient));
  return bound as F;
}

export const createQuery = withSharedClient(createQueryWithClient);
export const createInfiniteQuery = withSharedClient(createInfiniteQueryWithClient);
export const createMutation = withSharedClient(createMutationWithClient);
