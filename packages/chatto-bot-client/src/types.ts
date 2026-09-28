/** Message fields used by bots; not a complete renderable message. */
export interface ChattoMessage {
  id: string;
  roomId: string;
  authorId: string;
  threadRootId?: string;
  inReplyTo?: string;
  body?: string;
}

/** A thread destination, independent of the workflow that sends the message. */
export interface Destination {
  roomId: string;
  threadRootId: string;
  /** Message that prompted this reply, within the destination thread. */
  inReplyTo?: string;
}

/** Send ordered thread messages; workflow adapters supply their cancellation signal. */
export type ChattoPost = (
  destination: Destination,
  body: string,
  signal: AbortSignal
) => Promise<void>;

/** Refresh a thread's typing indicator once. */
export type ChattoTyping = (destination: Destination, signal: AbortSignal) => Promise<void>;

/** Locate a thread independently of a bot or webhook delivery. */
export interface ThreadLocation {
  roomId: string;
  threadRootId: string;
}

/** A textual thread message; attachments and non-message events are omitted. */
export interface ThreadMessage {
  id: string;
  authorId?: string;
  body: string;
}

/** Safe realtime diagnostics, without remote messages or connection details. */
export type RealtimeStatus =
  | { state: 'connecting' | 'reconnecting' }
  | {
      state: 'ready';
      /**
       * True when the server could not resume the stream and sent a new
       * snapshot instead. Events since the previous connection can be missing.
       */
      gap: boolean;
    };
