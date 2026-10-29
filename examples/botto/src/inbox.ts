/** Chatto batching policy. Runling owns scheduling, capacity, and shutdown. */
import { BatchQueue } from 'runling';
import { replyDestination, type RealtimeEvent, type ThreadLocation } from '@chatto/client';
import type { IncomingMessage } from './messages.ts';
import { DEFAULT_INBOX, type InboxSettings } from './settings.ts';
import { threadKey } from './threads.ts';

const keyOf = (message: IncomingMessage) => threadKey(replyDestination(message));

/** Any addressed message promotes the complete pending batch to the short quiet period. */
const batchDelay = (messages: readonly IncomingMessage[], settings: InboxSettings) =>
  messages.some((message) => message.reasons.length > 0)
    ? settings.debounceMs
    : settings.passiveDebounceMs;

/** Adapt typing and addressed-message quiet periods to Runling's keyed scheduler. */
export class Inbox extends BatchQueue<IncomingMessage> {
  constructor(
    turn: (messages: readonly IncomingMessage[]) => Promise<void>,
    failed: () => void,
    private readonly settings: InboxSettings = DEFAULT_INBOX
  ) {
    super({
      key: keyOf,
      id: (message) => message.id,
      delay: (messages) => batchDelay(messages, settings),
      run: turn,
      failed: () => failed(),
      maxKeys: settings.maxThreads,
      maxItems: settings.maxPendingMessages
    });
  }

  /** Include queued and running turns when deciding whether to follow a thread. */
  isActive(location: ThreadLocation): boolean {
    return this.has(threadKey(location));
  }

  /** Report the quiet period selected for an incoming message. */
  debounceMs(message: IncomingMessage): number {
    return batchDelay([...this.queued(keyOf(message)), message], this.settings);
  }

  /** Typing extends queued batches. Room typing applies only to queued roots. */
  typing(event: RealtimeEvent, viewerId: string): void {
    if (event.event.case !== 'userTyping' || !event.actorId || event.actorId === viewerId) return;
    const { roomId, threadRootEventId } = event.event.value;
    if (!roomId) return;
    this.touch(
      ([first], key) =>
        first?.roomId === roomId &&
        (threadRootEventId
          ? key === threadKey({ roomId, threadRootId: threadRootEventId })
          : !first.threadRootId)
    );
  }
}
