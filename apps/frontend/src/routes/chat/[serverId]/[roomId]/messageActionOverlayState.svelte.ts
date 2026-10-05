import type { QuoteInsertionContent } from '$lib/state/room';

export type MessageContextMenuPosition = {
  x: number;
  y: number;
  alignRight?: boolean;
  centerX?: boolean;
};

/** The action overlay that is open for a message. Only one is open at a time. */
export type MessageActionOverlay =
  /** Desktop context menu at a viewport point. */
  | { kind: 'menu'; position: MessageContextMenuPosition }
  /** Touch action sheet, opened by a long press. */
  | { kind: 'sheet' }
  /** Emoji picker. `sheet` forces a bottom sheet when the touch action sheet opened it. */
  | { kind: 'emoji'; position: { x: number; y: number }; presentation: 'auto' | 'sheet' }
  /** Accounts that reacted to the message. */
  | { kind: 'reactions' };

/** What the user pointed at when they opened the overlay. */
export type MessageActionOverlayContext = {
  /** Resolved URL of the message-body link that opened the context menu. */
  linkUrl?: string | null;
  /** URL of the image attachment that opened the context menu. */
  imageUrl?: string | null;
  /** Selected message text to quote when the user replies from the overlay. */
  replyQuote?: QuoteInsertionContent | null;
};

/** The open overlay, the message that owns it, and what the user pointed at. */
export type OpenMessageActionOverlay = MessageActionOverlay &
  Omit<MessageActionOverlayContext, 'replyQuote'> & { eventId: string };

/**
 * The message action overlays of one timeline: context menu, touch action sheet, emoji
 * picker, and reaction details.
 *
 * Message rows only request overlays. The timeline renders them in one host outside the
 * virtualized rows, so an overlay stays open when the virtualizer unmounts its row, for
 * example when the virtual keyboard opens for the emoji search and the timeline gets
 * shorter. The overlay follows its message, not the row element that showed it.
 */
export class MessageActionOverlayState {
  current = $state.raw<OpenMessageActionOverlay | null>(null);
  // Not reactive: only a reply reads it, once.
  #replyQuote: QuoteInsertionContent | null = null;

  /** The kind of the overlay that is open for the message, or null. */
  kindFor(eventId: string): MessageActionOverlay['kind'] | null {
    return this.current?.eventId === eventId ? this.current.kind : null;
  }

  /** Opens an overlay for a message and replaces any other open overlay. */
  open(eventId: string, overlay: MessageActionOverlay, context: MessageActionOverlayContext = {}) {
    const { replyQuote = null, ...pointer } = context;
    this.current = { ...overlay, ...pointer, eventId };
    this.#replyQuote = replyQuote;
  }

  /**
   * Closes the open overlay. `kind` and `eventId` limit the close to a matching overlay.
   * A menu that opens another overlay and then closes itself does not close the new
   * overlay, and a late close from one message does not close the overlay of another.
   */
  close(scope: { kind?: MessageActionOverlay['kind']; eventId?: string } = {}): void {
    const current = this.current;
    if (!current) return;
    if (scope.kind && current.kind !== scope.kind) return;
    if (scope.eventId && current.eventId !== scope.eventId) return;
    this.current = null;
    this.#replyQuote = null;
  }

  /** Returns the quote for a reply from the overlay, and clears it. */
  takeReplyQuote(): QuoteInsertionContent | null {
    const quote = this.#replyQuote;
    this.#replyQuote = null;
    return quote;
  }
}
