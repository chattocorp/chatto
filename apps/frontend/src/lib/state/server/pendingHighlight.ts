import { ReactiveMap } from '@chatto/client/reactivity/index';

/**
 * Transient store for "next time we land in room X (or thread X/T), highlight
 * event Y." Set by in-app navigations (e.g. notification clicks) before the
 * navigation, then consumed by the room view once the destination's data has
 * loaded and the room id matches.
 *
 * Why not URL params? `?highlight=` is reactive and survives refresh (which
 * means the highlight re-fires every time the URL is parsed), and the
 * strip-URL-then-jump dance has a built-in race where the strip lands before
 * the destination is ready. A keyed transient store is one-shot, scoped to
 * the destination, and immune to refresh.
 *
 * The `?highlight=` URL param remains the right semantic for shareable
 * permalinks; the room view checks both, with this store taking precedence.
 */
export class PendingHighlightStore {
  private highlights = new ReactiveMap<
    string,
    { eventId: string; notificationId: string | null }
  >();

  set(
    roomId: string,
    threadRootId: string | null,
    eventId: string,
    notificationId: string | null = null
  ): void {
    this.highlights.set(this.#key(roomId, threadRootId), { eventId, notificationId });
  }

  /** Whether a destination has a highlight that is not consumed yet. Reactive. */
  has(roomId: string, threadRootId: string | null): boolean {
    return this.highlights.has(this.#key(roomId, threadRootId));
  }

  /**
   * Remove and return the pending highlight for a destination, if any.
   */
  consume(
    roomId: string,
    threadRootId: string | null
  ): { eventId: string; notificationId: string | null } | null {
    const k = this.#key(roomId, threadRootId);
    const highlight = this.highlights.get(k);
    if (highlight === undefined) return null;
    this.highlights.delete(k);
    return highlight;
  }

  clear(): void {
    this.highlights.clear();
  }

  #key(roomId: string, threadRootId: string | null): string {
    return threadRootId ? `${roomId} ${threadRootId}` : roomId;
  }
}
