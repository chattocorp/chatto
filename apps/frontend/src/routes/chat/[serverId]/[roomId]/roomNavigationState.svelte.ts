import type { QuoteInsertionContent } from '$lib/state/room';
import type { PendingThreadReplyRequest, ThreadOpenOptions } from './threadOpenOptions';

export class RoomNavigationState {
  pendingThreadHighlight = $state<string | null>(null);
  pendingMainHighlightId = $state<string | null>(null);
  pendingThreadQuote = $state<{ id: number; text: QuoteInsertionContent } | null>(null);
  pendingThreadReply = $state<PendingThreadReplyRequest | null>(null);

  #mainHighlightRequestId = 0;
  #pendingThreadQuoteId = 0;
  #pendingThreadReplyId = 0;
  #appliedThreadMessageRoute: string | null = null;

  prepareThreadOpen(threadRootEventId: string, options: ThreadOpenOptions = {}): void {
    this.pendingThreadHighlight = options.highlightEventId ?? null;
    this.pendingThreadQuote = options.quoteText
      ? { id: ++this.#pendingThreadQuoteId, text: options.quoteText }
      : null;
    this.pendingThreadReply = options.reply
      ? { id: ++this.#pendingThreadReplyId, threadRootEventId, ...options.reply }
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

  beginHighlight(eventId: string, inThread: boolean): number | null {
    if (inThread) {
      this.pendingThreadHighlight = eventId;
      return null;
    }

    this.pendingMainHighlightId = eventId;
    return ++this.#mainHighlightRequestId;
  }

  failMainHighlight(requestId: number, eventId: string): boolean {
    if (this.#mainHighlightRequestId !== requestId || this.pendingMainHighlightId !== eventId) {
      return false;
    }

    this.pendingMainHighlightId = null;
    return true;
  }

  clearMainHighlight(): void {
    this.pendingMainHighlightId = null;
  }

  clearThreadHighlight(): void {
    this.pendingThreadHighlight = null;
  }

  clearThreadQuote(): void {
    this.pendingThreadQuote = null;
  }

  clearThreadReply(): void {
    this.pendingThreadReply = null;
  }
}
