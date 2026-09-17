import { StaleResponseError } from '$lib/api-client/connect';
import type { ServerScope } from '$lib/state/server/scope.svelte';

/** A real navigation ends the visit; rebuilding private UI does not. */
export class NavigationVisits {
  #visit = 0;
  leave(): void {
    this.#visit++;
  }
  capture(): () => boolean {
    const visit = this.#visit;
    return () => visit === this.#visit;
  }
}

/** Owned by the application layout, outside any resettable server route. */
export const navigationVisits = new NavigationVisits();

/** Capture safe completion before a request starts. No returned data is retained. */
export function captureMutationCompletion(scope: ServerScope): () => boolean {
  const visitCurrent = navigationVisits.capture();
  const connection = scope.connection;
  return () => visitCurrent() && scope.isCurrent() && scope.connection === connection;
}

/** Complete outside the component's mutation observer, which a reset can destroy.
 * A discarded successful mutation may navigate using submitted IDs, but its old
 * response body must never be returned to caches or rendering code.
 */
export async function completeMutation<T>(
  operation: () => Promise<T>,
  isCurrent: () => boolean,
  onComplete: () => void
): Promise<T | undefined> {
  let result: T | undefined;
  try {
    result = await operation();
  } catch (error) {
    if (!(error instanceof StaleResponseError) || !error.mutationSucceeded) throw error;
  }
  if (isCurrent()) onComplete();
  return result;
}
