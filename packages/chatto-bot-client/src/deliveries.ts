/** Accepted delivery IDs, retained in memory for one server and bot identity.
 * This does not reserve concurrent deliveries or provide durable acceptance. */
export interface DeliveryTracker {
  has(id: string): boolean;
  /** Call only after inbox insertion or successful run registration. */
  accept(id: string): void;
}

/** Create a process-local replay filter. Retain the object across configuration reloads.
 * Expired IDs can be accepted again. The default retention is 24 hours. */
export function createDeliveryTracker({
  retentionMs = 86_400_000,
  now = Date.now,
  accepted = new Map<string, number>()
}: {
  retentionMs?: number;
  now?: () => number;
  /** Optional retained storage: delivery ID to expiry time in milliseconds. */
  accepted?: Map<string, number>;
} = {}): DeliveryTracker {
  if (!Number.isFinite(retentionMs) || retentionMs <= 0)
    throw new Error('Delivery retention must be positive and finite');
  function prune() {
    const time = now();
    for (const [id, expires] of accepted) if (expires <= time) accepted.delete(id);
    return time;
  }
  return {
    has(id) {
      prune();
      return accepted.has(id);
    },
    accept(id) {
      accepted.set(id, prune() + retentionMs);
    }
  };
}
