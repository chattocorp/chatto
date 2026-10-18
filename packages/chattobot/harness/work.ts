/** Conversation-owned state for delegated work. Execution uses ordinary Runling invocations. */
import type { Destination } from '@chatto/client';
import type { Invocation } from 'runling';

export interface WorkDetails {
  kind: 'investigation' | 'github' | 'paseo';
  label: string;
  replyTo: Destination;
  /** Saved investigation identity, independent of this task invocation. */
  sessionId?: string;
}

export interface PendingWork extends WorkDetails {
  /** The actual Runling invocation ID. */
  id: string;
  startedAt: number;
  latest?: string;
  child: Invocation<unknown, unknown, unknown>;
  /** Settles after result delivery and cooperative child cleanup. */
  done: Promise<void>;
}

export type WorkOutcome = { result: string } | { error: string };

/** Delegation tools give their already-started child to the owning conversation. */
export interface WorkOwner {
  readonly pending: ReadonlyMap<string, PendingWork>;
  track<T>(
    child: Invocation<unknown, unknown, T>,
    details: WorkDetails,
    formatResult: (result: T) => string
  ): PendingWork;
}
