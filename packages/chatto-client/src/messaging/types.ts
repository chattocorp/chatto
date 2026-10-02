/**
 * Plain message types for request helpers and message handlers. They hold the
 * fields that hosts such as bots use; they are not complete renderable
 * messages. Use the stores for rendering.
 */

/** A message that the client read or received. */
export interface ChattoMessage {
  id: string;
  roomId: string;
  authorId: string;
  /** The thread root, or absent for a message at the room level. */
  threadRootId?: string;
  /** The message that this message replies to. */
  inReplyTo?: string;
  /** The message text. Absent for a message without text. */
  body?: string;
}

/** Where to send a message: a room and a thread in it. */
export interface Destination {
  roomId: string;
  /** The thread root. Use the message's own ID to start a thread on it. */
  threadRootId: string;
  /** The message that prompted this one, within the destination thread. */
  inReplyTo?: string;
}

/** A thread in a room. */
export interface ThreadLocation {
  roomId: string;
  threadRootId: string;
}

/** A file attached to a message: its metadata only. `readAttachment` reads its content. */
export interface MessageAttachmentInfo {
  id: string;
  filename: string;
  /** MIME type, such as `image/png`. */
  contentType: string;
  /** Image or video width in pixels, when known. */
  width?: number;
  /** Image or video height in pixels, when known. */
  height?: number;
  /** The author's description of the file, when given. */
  description?: string;
}

/** A thread message with text, attachments, or both; non-message events are left out. */
export interface ThreadMessage {
  id: string;
  authorId?: string;
  /** The author's display name, or login when no display name is set. */
  authorName?: string;
  authorLogin?: string;
  /** The message text. Empty for a message with attachments only. */
  body: string;
  /** Files attached to the message. Absent when there are none. */
  attachments?: MessageAttachmentInfo[];
  /** Whether the viewer, the account of the API key, wrote the message. */
  fromViewer: boolean;
}

/** Messages from one thread read, with a cursor for reading newer messages later. */
export interface ThreadRead {
  messages: ThreadMessage[];
  /** Pass as `after` to read only newer messages. Absent when the thread has no messages. */
  cursor?: string;
  /** True when older replies exist that this read left out. */
  olderOmitted: boolean;
}

/** Options for one request. */
export interface RequestOptions {
  /**
   * Cancels the request. A request is not sent when the signal already
   * aborted. Each request also has a ten-second timeout.
   */
  signal?: AbortSignal;
}

/** Options for a thread read; see `readThread`. */
export interface ThreadReadOptions extends RequestOptions {
  /** A `cursor` from an earlier read: return only newer messages. */
  after?: string;
  /** Most replies to return without `after`. Default: 100. */
  limit?: number;
}

/** Options for `readAttachment`. */
export interface AttachmentReadOptions extends RequestOptions {
  /**
   * Resize an image on the server to fit within this many pixels on each side
   * (at most 2048). Other files, and images without this option, return the
   * original.
   */
  maxImageSize?: number;
  /** Reject content larger than this many bytes. Default: 5 MB. */
  maxBytes?: number;
}

/** The content of one attachment. */
export interface AttachmentContent {
  filename: string;
  /** MIME type of `data`. A resized image can have another type than the original. */
  contentType: string;
  data: Uint8Array;
}

/** Realtime status for hosts, without remote messages or connection details. */
export type RealtimeStatus =
  | {
      state: 'connecting' | 'reconnecting';
      /**
       * Why the last attempt to reach the server failed, for example an
       * unreachable server. The message contains no remote data.
       */
      error?: Error;
    }
  | {
      state: 'ready';
      /**
       * True when events since the previous connection can be missing, for
       * example because the server could not resume the stream and sent a
       * new snapshot instead.
       */
      gap: boolean;
    };
