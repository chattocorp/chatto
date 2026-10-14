/**
 * Human approval of actions, as in an agent harness that asks the user before a risky tool runs.
 * The host posts the exact action, never the model's summary of it, so text in the model's context
 * cannot change what people approve. An approver reacts with ✅ to approve or ❌ to decline.
 * The reaction is temporary UI until bots get interactive UI in Chatto (#2786).
 */
import type { Destination } from '@chatto/client';

/** Gemoji shortcodes, as Chatto stores reactions. */
export const APPROVE = 'white_check_mark';
export const DECLINE = 'x';

export type Decision = 'approved' | 'declined' | 'expired';

/** A reaction to one of the bot's messages. The author ID is server-authenticated. */
export interface Reaction {
  messageId: string;
  emoji: string;
  authorId: string;
}

export interface ApprovalOptions {
  /** Chatto user IDs that can approve. */
  approvers: ReadonlySet<string>;
  /** Post the request and return its message ID. */
  post(destination: Destination, text: string): Promise<string | undefined>;
  /** How long a request waits for a decision. */
  timeoutMs?: number;
}

/** Requests wait at most this long by default. */
const TIMEOUT_MS = 60 * 60_000;

export class Approvals {
  private pending = new Map<string, (decision: Decision) => void>();

  constructor(private options: ApprovalOptions) {}

  /** Post a request for the action that `text` describes and wait for a decision. */
  async request(text: string, replyTo: Destination, signal: AbortSignal): Promise<Decision> {
    if (!this.options.approvers.size) return 'declined';
    // Host text: the request must show the exact action, not words that the model chose.
    const messageId = await this.options.post(
      replyTo,
      `${text}\n\nA maintainer can approve this with ✅ or decline it with ❌.`
    );
    if (!messageId) throw new Error('Could not post the approval request');
    // The conversation can end while the request is posted.
    signal.throwIfAborted();
    return new Promise<Decision>((resolve, reject) => {
      const done = (decision: Decision) => {
        clearTimeout(timer);
        signal.removeEventListener('abort', abort);
        this.pending.delete(messageId);
        resolve(decision);
      };
      const abort = () => {
        clearTimeout(timer);
        this.pending.delete(messageId);
        reject(signal.reason);
      };
      const timer = setTimeout(() => done('expired'), this.options.timeoutMs ?? TIMEOUT_MS);
      timer.unref?.();
      signal.addEventListener('abort', abort, { once: true });
      this.pending.set(messageId, done);
    });
  }

  /** Apply a reaction. Only an approver's ✅ or ❌ on a pending request counts. */
  react({ messageId, emoji, authorId }: Reaction): boolean {
    const done = this.pending.get(messageId);
    if (!done || !this.options.approvers.has(authorId)) return false;
    if (emoji === APPROVE) done('approved');
    else if (emoji === DECLINE) done('declined');
    else return false;
    return true;
  }
}
