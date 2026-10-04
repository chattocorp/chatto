import { batch, ReactiveMap } from '@chatto/client/reactivity/index';

/**
 * A jump request whose object identity stays stable until completion.
 * A new click creates a new request, including a click on the same message.
 */
export type PendingHighlight = Readonly<{
  roomId: string;
  /** The target thread, or null for the room timeline. */
  threadRootEventId: string | null;
  eventId: string;
  /** Mark this occurrence read only after its message is visible and highlighted. */
  notificationId: string | null;
}>;

/**
 * Transient store for "next time we land in room X (or thread X/T), highlight
 * event Y." Set by in-app navigations (e.g. notification clicks) before the
 * navigation. The destination reads it without removing it, so hydration and
 * component remounts cannot lose an unfinished jump. Only completion, departure
 * from the destination, or a newer request removes it.
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
  private highlights = new ReactiveMap<string, PendingHighlight>();

  /** The active request for this server, including one awaiting navigation. */
  get current(): PendingHighlight | null {
    return this.highlights.values().next().value ?? null;
  }

  /** Replace an unfinished request with a new request for the selected destination. */
  set(
    roomId: string,
    threadRootId: string | null,
    eventId: string,
    notificationId: string | null = null
  ): void {
    batch(() => {
      this.highlights.clear();
      this.highlights.set(this.#key(roomId, threadRootId), {
        roomId,
        threadRootEventId: threadRootId,
        eventId,
        notificationId
      });
    });
  }

  /** Whether a destination has an unfinished highlight. Reactive. */
  has(roomId: string, threadRootId: string | null): boolean {
    return this.highlights.has(this.#key(roomId, threadRootId));
  }

  /**
   * Read a destination's request without acknowledging it. Reactive.
   */
  peek(roomId: string, threadRootId: string | null): PendingHighlight | null {
    return this.highlights.get(this.#key(roomId, threadRootId)) ?? null;
  }

  /** Remove only this request; a late completion cannot remove its replacement. */
  complete(highlight: PendingHighlight): void {
    const key = this.#key(highlight.roomId, highlight.threadRootEventId);
    if (this.highlights.get(key) === highlight) this.highlights.delete(key);
  }

  /** Discard navigation state on a viewer privacy reset or server disposal. */
  clear(): void {
    this.highlights.clear();
  }

  #key(roomId: string, threadRootId: string | null): string {
    return threadRootId ? `${roomId} ${threadRootId}` : roomId;
  }
}
