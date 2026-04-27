import { vi, type Mock } from 'vitest';
import type { Client } from '@urql/svelte';

/**
 * Mock urql `Client` whose `query`/`mutation`/`subscription` are vitest mocks.
 *
 * By default each operation returns `{ data: null, error: null }` via a
 * `toPromise()`-shaped object. Override `mutationData` / `queryData` to make
 * the mock resolve with a happy-path payload, or grab the returned `query`/
 * `mutation`/`subscription` mocks to assert call shape.
 *
 * ```ts
 * const client = createMockGraphqlClient({
 *   mutationData: { postMessage: { id: 'evt_1' } }
 * });
 * render(Composer, { context: new Map([['$$_urql', client]]) });
 * expect(client.mutation).toHaveBeenCalled();
 * ```
 */
export function createMockGraphqlClient(opts: {
  mutationData?: unknown;
  queryData?: unknown;
} = {}): Client & { query: Mock; mutation: Mock; subscription: Mock } {
  return {
    query: vi.fn().mockReturnValue({
      toPromise: vi.fn().mockResolvedValue({ data: opts.queryData ?? null, error: null })
    }),
    mutation: vi.fn().mockReturnValue({
      toPromise: vi.fn().mockResolvedValue({ data: opts.mutationData ?? null, error: null })
    }),
    subscription: vi.fn()
  } as unknown as Client & { query: Mock; mutation: Mock; subscription: Mock };
}

/**
 * The shape returned from `useConnection()` — used by components that read
 * `client`/`isConnected` straight off the connection context. Pass to
 * `vi.mock('$lib/state/instance/connection.svelte', ...)`:
 *
 * ```ts
 * vi.mock('$lib/state/instance/connection.svelte', () => ({
 *   useConnection: () => () => createMockConnection({ mutationData: ... })
 * }));
 * ```
 */
export function createMockConnection(opts: {
  isConnected?: boolean;
  showConnectionLostBanner?: boolean;
  mutationData?: unknown;
  queryData?: unknown;
} = {}) {
  return {
    isConnected: opts.isConnected ?? true,
    showConnectionLostBanner: opts.showConnectionLostBanner ?? false,
    client: createMockGraphqlClient({
      mutationData: opts.mutationData,
      queryData: opts.queryData
    })
  };
}
