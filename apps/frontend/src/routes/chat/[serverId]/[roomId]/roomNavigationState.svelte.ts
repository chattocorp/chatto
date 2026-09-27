import type { QuoteInsertionContent } from '$lib/state/room';
import type { PendingThreadReply, ThreadOpenOptions } from './threadOpenOptions';

/** A message that a conversation pane must jump to and highlight once. */
export type PendingHighlight = {
  roomId: string;
  /** The thread timeline that shows the message, or null for the room timeline. */
  threadRootEventId: string | null;
  eventId: string;
  /** A notification that becomes read after a successful jump. */
  notificationId: string | null;
};

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
  highlight = $state.raw<PendingHighlight | null>(null);
  composerInput = $state.raw<PendingComposerInput | null>(null);

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
      this.highlight = null;
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
   * The effect that reads the parameter can run again before the URL update
   * that removes it. A missing parameter resets the guard.
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
    this.highlight = { roomId, threadRootEventId, eventId, notificationId };
  }

  highlightFor(roomId: string, threadRootEventId: string | null): PendingHighlight | null {
    const highlight = this.highlight;
    return highlight?.roomId === roomId && highlight.threadRootEventId === threadRootEventId
      ? highlight
      : null;
  }

  composerInputFor(roomId: string, threadRootEventId: string): PendingComposerInput | null {
    const input = this.composerInput;
    return input?.roomId === roomId && input.threadRootEventId === threadRootEventId ? input : null;
  }

  clearHighlight(highlight: PendingHighlight): void {
    if (this.highlight === highlight) this.highlight = null;
  }

  /** Drop a room-timeline highlight when its room stops being active. */
  clearMainHighlight(): void {
    if (this.highlight?.threadRootEventId === null) this.highlight = null;
  }

  clearComposerInput(input: PendingComposerInput): void {
    if (this.composerInput === input) this.composerInput = null;
  }
}
