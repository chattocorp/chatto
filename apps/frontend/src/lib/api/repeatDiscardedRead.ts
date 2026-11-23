import { StaleResponseError } from '@chatto/client/api/connect';

/**
 * Run a read, and run it once more when a privacy reset discarded its
 * response.
 *
 * The first realtime snapshot of a server that has no displayed view is a
 * privacy reset: the connection discards every response that was in flight,
 * because the snapshot cannot vouch for older reads. A page that reads before
 * that snapshot arrives gets a `StaleResponseError` once. Server queries
 * handle this with their retry. Use this helper for a read outside a query.
 *
 * Use it only for reads. Do not repeat a mutation: a discarded mutation
 * response can belong to a successful change.
 */
export async function repeatDiscardedRead<T>(read: () => Promise<T>): Promise<T> {
  try {
    return await read();
  } catch (error) {
    if (!(error instanceof StaleResponseError) || error.mutationSucceeded) throw error;
    return read();
  }
}
