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
  /** The author's display name, or login when no display name is set. */
  authorName?: string;
  authorLogin?: string;
  body: string;
}

/** Messages from one thread read, with a cursor for reading newer messages later. */
export interface ThreadRead {
  messages: ThreadMessage[];
  /** Pass as `after` to read only newer messages. Absent when the thread has no messages. */
  cursor?: string;
  /** True when older replies exist that this read left out. */
  olderOmitted: boolean;
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
