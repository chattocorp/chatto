import type { QuoteInsertionContent } from '$lib/state/room';
import type { PendingThreadReply, ThreadOpenOptions } from './threadOpenOptions';
import { PendingHighlightStore, type PendingHighlight } from '$lib/state/server/pendingHighlight';

export type { PendingHighlight } from '$lib/state/server/pendingHighlight';

/** Composer input that waits until the target thread pane has a composer. */
export type PendingComposerInput = {
  roomId: string;
  threadRootEventId: string;
  quote?: QuoteInsertionContent;
  reply?: PendingThreadReply;
};

/**
 * Room-level navigation requests for the room and thread conversation panes.
 *
 * Each request names its target timeline. A pane reads only its own requests
 * and clears one after it handles it. The clear methods take the handled
 * request, so a late completion cannot clear a newer request.
 */
export class RoomNavigationState {
  composerInput = $state.raw<PendingComposerInput | null>(null);
  private readonly highlights: PendingHighlightStore;

  /** Use the server's store to retain jumps across room hydration and remounts. */
  constructor(highlights = new PendingHighlightStore()) {
    this.highlights = highlights;
  }

  get highlight(): PendingHighlight | null {
    return this.highlights.current;
  }

  #appliedThreadMessageRoute: string | null = null;
  #appliedHighlightParam: string | null = null;

  prepareThreadOpen(
    roomId: string,
    threadRootEventId: string,
    options: ThreadOpenOptions = {}
  ): void {
    if (options.highlightEventId) {
      this.beginHighlight(roomId, threadRootEventId, options.highlightEventId);
    } else if (this.highlight?.threadRootEventId) {
      this.highlights.complete(this.highlight);
    }
    this.composerInput =
      options.quoteText || options.reply
        ? { roomId, threadRootEventId, quote: options.quoteText, reply: options.reply }
        : null;
  }

  consumeThreadMessageRoute(
    roomId: string,
    threadRootEventId: string | undefined,
    messageEventId: string | undefined
  ): string | null | undefined {
    if (!threadRootEventId || !messageEventId) {
      this.#appliedThreadMessageRoute = null;
      return undefined;
    }

    const route = `${roomId}:${threadRootEventId}:${messageEventId}`;
    if (this.#appliedThreadMessageRoute === route) return null;
    this.#appliedThreadMessageRoute = route;
    return messageEventId;
  }

  /**
   * Return a `?highlight=` permalink target once per room, thread, and target.
   * Route activation can repeat during hydration or before the URL update
   * removes the parameter. A missing parameter resets the guard.
   */
  consumeHighlightParam(
    roomId: string,
    threadRootEventId: string | undefined,
    eventId: string | null
  ): string | null {
    if (!eventId) {
      this.#appliedHighlightParam = null;
      return null;
    }

    const key = `${roomId}:${threadRootEventId ?? ''}:${eventId}`;
    if (this.#appliedHighlightParam === key) return null;
    this.#appliedHighlightParam = key;
    return eventId;
  }

  beginHighlight(
    roomId: string,
    threadRootEventId: string | null,
    eventId: string,
    notificationId: string | null = null
  ): void {
    this.highlights.set(roomId, threadRootEventId, eventId, notificationId);
  }

  highlightFor(roomId: string, threadRootEventId: string | null): PendingHighlight | null {
    return this.highlights.peek(roomId, threadRootEventId);
  }

  composerInputFor(roomId: string, threadRootEventId: string): PendingComposerInput | null {
    const input = this.composerInput;
    return input?.roomId === roomId && input.threadRootEventId === threadRootEventId ? input : null;
  }

  clearHighlight(highlight: PendingHighlight): void {
    this.highlights.complete(highlight);
  }

  /** Drop a room-timeline highlight when its room stops being active. */
  clearMainHighlight(roomId: string): void {
    const highlight = this.highlights.peek(roomId, null);
    if (highlight) this.highlights.complete(highlight);
  }

  clearComposerInput(input: PendingComposerInput): void {
    if (this.composerInput === input) this.composerInput = null;
  }
}
