import { createContext } from 'svelte';

// ---------------------------------------------------------------------------
// EditState — tracks which message is being edited
// ---------------------------------------------------------------------------

export class EditState {
  eventId = $state<string | null>(null);
  originalBody = $state('');
  threadRootEventId = $state<string | null>(null);
  channelEchoEventId = $state<string | null>(null);
  canAddChannelEcho = $state(false);

  startEdit(eventId: string, body: string, options: EditMessageOptions = {}) {
    this.eventId = eventId;
    this.originalBody = body;
    this.threadRootEventId = options.threadRootEventId ?? null;
    this.channelEchoEventId = options.channelEchoEventId ?? null;
    this.canAddChannelEcho = options.canAddChannelEcho ?? false;
  }

  cancelEdit() {
    this.eventId = null;
    this.originalBody = '';
    this.threadRootEventId = null;
    this.channelEchoEventId = null;
    this.canAddChannelEcho = false;
  }
}

// ---------------------------------------------------------------------------
// ReplyState — tracks which message the user is replying to (in-room reply)
// ---------------------------------------------------------------------------

export class ReplyState {
  messageEventId = $state<string | null>(null);
  actorDisplayName = $state('');
  excerpt = $state('');

  startReply(messageEventId: string, actorDisplayName: string, excerpt: string) {
    this.messageEventId = messageEventId;
    this.actorDisplayName = actorDisplayName;
    this.excerpt = excerpt;
  }

  cancelReply() {
    this.messageEventId = null;
    this.actorDisplayName = '';
    this.excerpt = '';
  }
}

// ---------------------------------------------------------------------------
// QuoteInsertionState — one-shot requests to insert selected reply quotes
// ---------------------------------------------------------------------------

export type QuoteInsertionRequest = {
  id: number;
  text: QuoteInsertionContent;
};

export type SelectedQuoteBlock = {
  quoteDepth: number;
  text: string;
};

export type QuoteInsertionContent = string | SelectedQuoteBlock[];

export class QuoteInsertionState {
  request = $state<QuoteInsertionRequest | null>(null);
  private nextRequestId = 1;

  requestInsertQuote(text: QuoteInsertionContent) {
    const quoteText =
      typeof text === 'string'
        ? text.trim()
        : text
            .map((block) => ({ ...block, text: block.text.trim() }))
            .filter((block) => block.text.length > 0);
    if (typeof quoteText === 'string' ? !quoteText : quoteText.length === 0) return;

    this.request = {
      id: this.nextRequestId++,
      text: quoteText
    };
  }
}

// ---------------------------------------------------------------------------
// LastEditableMessageContext — finder for up-arrow-to-edit
// ---------------------------------------------------------------------------

export type EditMessageOptions = {
  threadRootEventId?: string | null;
  channelEchoEventId?: string | null;
  canAddChannelEcho?: boolean;
};
export type EditableMessage = { eventId: string; body: string } & EditMessageOptions;
export type FindLastEditableMessage = () => EditableMessage | null;

export class LastEditableMessageContext {
  private finder: FindLastEditableMessage | null = null;

  setFinder(fn: FindLastEditableMessage) {
    this.finder = fn;
  }

  getLastEditableMessage(): EditableMessage | null {
    return this.finder?.() ?? null;
  }
}

// ---------------------------------------------------------------------------
// ScrollState — scroll-to-bottom coordination between composer and event list
// ---------------------------------------------------------------------------

export class ScrollState {
  scrollRequestCounter = $state(0);
  private container: HTMLDivElement | null = null;
  private shouldScroll = true;

  requestScrollToBottom() {
    this.scrollRequestCounter++;
  }

  setContainer(el: HTMLDivElement | null) {
    this.container = el;
  }

  setShouldScroll(value: boolean) {
    this.shouldScroll = value;
  }

  scrollToBottomIfSticky() {
    if (this.shouldScroll && this.container) {
      this.container.scrollTop = this.container.scrollHeight;
    }
  }
}

// ---------------------------------------------------------------------------
// JumpToMessageState — jump to a specific message in the event list
// ---------------------------------------------------------------------------

export class JumpToMessageState {
  isJumpedMode = $state(false);
  scrollToEventId = $state<string | null>(null);
  hasReachedEnd = $state(false);
  hasOlderMessages = $state(false);
  isLoadingNewer = $state(false);

  private _jumpFn: ((eventId: string) => Promise<boolean>) | null = null;
  private _loadNewerFn: (() => Promise<void>) | null = null;

  setJumpHandler(fn: (eventId: string) => Promise<boolean>) {
    this._jumpFn = fn;
  }

  setLoadNewerHandler(fn: () => Promise<void>) {
    this._loadNewerFn = fn;
  }

  async jumpToMessage(eventId: string): Promise<boolean> {
    if (this._jumpFn) {
      return this._jumpFn(eventId);
    }
    return false;
  }

  async loadNewer(): Promise<void> {
    if (this._loadNewerFn) {
      await this._loadNewerFn();
    }
  }

  reset(): void {
    this.isJumpedMode = false;
    this.scrollToEventId = null;
    this.hasReachedEnd = false;
    this.hasOlderMessages = false;
    this.isLoadingNewer = false;
  }
}

// ---------------------------------------------------------------------------
// ComposerContext — bundles per-pane state (one per Room or ThreadPane)
// ---------------------------------------------------------------------------

export interface ComposerContextOptions {
  /** Whether to create a ScrollState (Room uses it, ThreadPane doesn't). */
  scroll?: boolean;
}

export class ComposerContext {
  readonly editState = new EditState();
  readonly replyState = new ReplyState();
  readonly quoteInsertionState = new QuoteInsertionState();
  readonly lastEditableMessage = new LastEditableMessageContext();
  readonly jumpState = new JumpToMessageState();
  readonly scrollState: ScrollState | null;

  constructor(options?: ComposerContextOptions) {
    this.scrollState = options?.scroll ? new ScrollState() : null;
  }
}

export const [getComposerContext, setComposerContext] = createContext<ComposerContext>();

/**
 * Create the composer context and set it in Svelte context.
 * Call from Room.svelte or ThreadPane during initialization.
 */
export function createComposerContext(options?: ComposerContextOptions): ComposerContext {
  const ctx = new ComposerContext(options);
  setComposerContext(ctx);
  return ctx;
}
