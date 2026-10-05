/**
 * The newest timeline position that the viewer can see.
 *
 * `latest` is true while the timeline shows the present window and follows its
 * bottom. A read at that position covers the whole conversation. Otherwise the
 * position is the newest event in the viewport.
 */
export type TimelineReadPosition = {
  /** Newest visible event, or the newest loaded event while `latest` is true. */
  eventId: string;
  /** Creation time of `eventId` in epoch milliseconds. */
  createdAtMs: number;
  latest: boolean;
};

/** Delay after the last forward scroll before a partial read is sent. */
export const READ_THROUGH_DEBOUNCE_MS = 1000;

/**
 * Converts viewport positions into as few read requests as possible.
 *
 * The tracker keeps the creation time of the newest message that a read has
 * covered. It sends a read only when the viewer sees a newer message. A read
 * through an older message waits until scrolling stops for
 * {@link READ_THROUGH_DEBOUNCE_MS}. A read at the latest position is sent at
 * once. Scrolling back, or over messages that a read already covered, sends
 * nothing.
 *
 * The tracker is suspended from {@link reset} until the owner reports the
 * entry read with {@link noteRead}, because the entry read already covers the
 * first position.
 */
export class ReadThroughTracker {
  /** Newest covered creation time. Infinity means read through the latest message. */
  #readThroughAt = Infinity;
  #suspended = true;
  #pendingEventId: string | null = null;
  #timer: ReturnType<typeof setTimeout> | null = null;

  /**
   * @param send Sends a read. `undefined` reads through the latest message.
   */
  constructor(
    private readonly send: (upToEventId: string | undefined) => void,
    private readonly delayMs = READ_THROUGH_DEBOUNCE_MS
  ) {}

  /** Start a new conversation. Positions are ignored until the next {@link noteRead}. */
  reset(): void {
    this.#cancel();
    this.#suspended = true;
    this.#readThroughAt = Infinity;
  }

  /**
   * Record a read that another path sent, such as the entry read. `null`
   * means through the latest message.
   */
  noteRead(createdAtMs: number | null): void {
    this.#cancel();
    this.#suspended = false;
    this.#readThroughAt = createdAtMs ?? Infinity;
  }

  /**
   * Record a message that arrived while the viewer was not at the latest
   * position. The next read at or after it must include it.
   */
  noteUnreadArrival(createdAtMs: number): void {
    this.#readThroughAt = Math.min(this.#readThroughAt, createdAtMs - 1);
  }

  /** Report the current viewport position. */
  observe(position: TimelineReadPosition): void {
    if (this.#suspended) return;

    if (position.latest) {
      if (this.#readThroughAt === Infinity) return;
      this.#cancel();
      this.#readThroughAt = Infinity;
      this.send(undefined);
      return;
    }

    if (position.createdAtMs <= this.#readThroughAt) return;
    this.#readThroughAt = position.createdAtMs;
    this.#pendingEventId = position.eventId;
    if (this.#timer !== null) clearTimeout(this.#timer);
    this.#timer = setTimeout(() => {
      this.#timer = null;
      const eventId = this.#pendingEventId;
      this.#pendingEventId = null;
      if (eventId) this.send(eventId);
    }, this.delayMs);
  }

  /** Drop a scheduled read. */
  dispose(): void {
    this.#cancel();
  }

  #cancel(): void {
    if (this.#timer !== null) clearTimeout(this.#timer);
    this.#timer = null;
    this.#pendingEventId = null;
  }
}
