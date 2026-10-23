/** Debounced conversation queues. A failed turn never blocks later messages. */
import type { AddressedMessage, ThreadLocation } from '@chatto/client';
import { DEFAULT_INBOX, type InboxSettings } from './settings.ts';

interface Conversation {
  messages: AddressedMessage[];
  timer?: ReturnType<typeof setTimeout>;
  running?: Promise<void>;
}

/** Serialize each conversation while allowing different conversations to run concurrently.
 * Apply the configured queue limits and remember the last 10,000 accepted IDs.
 * Queues and duplicate detection do not survive process restart. */
export class Inbox {
  private readonly threads = new Map<string, Conversation>();
  private readonly seen = new Set<string>();
  private pending = 0;
  private closed = false;

  constructor(
    private readonly turn: (messages: readonly AddressedMessage[]) => Promise<void>,
    private readonly failed: () => void,
    private readonly settings: InboxSettings = DEFAULT_INBOX
  ) {}

  /** Pending work activates a thread even before Pi has saved its first transcript. */
  isActive({ roomId, threadRootId }: ThreadLocation): boolean {
    return this.threads.has(JSON.stringify([roomId, threadRootId]));
  }

  /** Addressed messages promote the whole pending batch to the short quiet period.
   * Later passive messages retain that choice until the batch starts. */
  debounceMs(message: AddressedMessage): number {
    if (message.reasons.includes('direct_message') && !message.threadRootId) return 0;
    const key = JSON.stringify([message.roomId, message.threadRootId ?? message.id]);
    const addressed =
      message.reasons.length > 0 ||
      this.threads.get(key)?.messages.some((item) => item.reasons.length > 0);
    return addressed ? this.settings.debounceMs : this.settings.passiveDebounceMs;
  }

  /** Enqueue without waiting for the model, so realtime delivery remains responsive. */
  accept(message: AddressedMessage): 'accepted' | 'duplicate' | 'full' | 'closed' {
    if (this.closed) return 'closed';
    if (this.seen.has(message.id)) return 'duplicate';
    const key = JSON.stringify([message.roomId, message.threadRootId ?? message.id]);
    if (
      this.pending >= this.settings.maxPendingMessages ||
      (!this.threads.has(key) && this.threads.size >= this.settings.maxThreads)
    )
      return 'full';
    this.seen.add(message.id);
    if (this.seen.size > 10_000) this.seen.delete(this.seen.values().next().value!);
    this.pending++;
    const conversation = this.threads.get(key) ?? { messages: [] };
    this.threads.set(key, conversation);
    conversation.messages.push(message);
    clearTimeout(conversation.timer);
    // Separate DM roots each start a thread, so delaying them cannot combine their replies.
    if (message.reasons.includes('direct_message') && !message.threadRootId) {
      conversation.timer = undefined;
      this.flush(key, conversation);
      return 'accepted';
    }
    conversation.timer = setTimeout(() => {
      conversation.timer = undefined;
      this.flush(key, conversation);
    }, this.debounceMs(message));
    return 'accepted';
  }

  /** The quiet period can elapse during a running turn; never overlap turns. */
  private flush(key: string, conversation: Conversation): void {
    if (this.closed || conversation.running || conversation.timer || !conversation.messages.length)
      return;
    const batch = conversation.messages.splice(0);
    conversation.running = Promise.resolve()
      .then(() => {
        if (!this.closed) return this.turn(batch);
      })
      .catch(() => this.failed())
      .finally(() => {
        this.pending -= batch.length;
        conversation.running = undefined;
        if (!conversation.messages.length) this.threads.delete(key);
        else this.flush(key, conversation);
      });
  }

  /** Drop queued work and wait for active turns; the caller must abort those turns first. */
  async close(): Promise<void> {
    this.closed = true;
    for (const conversation of this.threads.values()) {
      clearTimeout(conversation.timer);
      this.pending -= conversation.messages.length;
      conversation.messages = [];
    }
    await Promise.all([...this.threads.values()].map((conversation) => conversation.running));
    this.threads.clear();
  }
}
