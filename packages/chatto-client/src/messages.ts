import type { RealtimeEvent } from "@chatto/api-types/realtime/v1/realtime_pb";
import { RoomKind } from "@chatto/api-types/api/v1/rooms_pb";

/** Message fields used by integrations; not a complete renderable message. */
export interface ChattoMessage {
  id: string;
  roomId: string;
  authorId: string;
  threadRootId?: string;
  inReplyTo?: string;
  body?: string;
}

export type AddressingReason = "direct_message" | "mention" | "reply";

/** A textual realtime message addressed to the authenticated viewer. */
export interface AddressedMessage extends ChattoMessage {
  body: string;
  occurredAt?: string;
  reasons: AddressingReason[];
}

type Rpc = <T>(method: string, body: object, signal?: AbortSignal) => Promise<T>;

/** Bind message helpers to the client's authenticated, bounded transport. */
export function messageHelpers(rpc: Rpc) {
  /** Resolve the authenticated viewer's identity. No environment or identity cache. */
  async function getViewer({ signal }: { signal?: AbortSignal } = {}): Promise<{ id: string }> {
    const response = await rpc<{ user?: { profile?: { id?: string } } }>("ViewerService/GetViewer", {}, signal);
    const id = response.user?.profile?.id;
    if (typeof id !== "string" || !id) throw new Error("Chatto did not return the viewer identity");
    return { id };
  }

  /** Read one visible message. Missing or malformed identities fail closed.
   * RPC failures propagate; hosts choose whether to retry or ignore them. */
  async function getMessage({ roomId, messageId, signal }: {
    roomId: string; messageId: string; signal?: AbortSignal;
  }): Promise<ChattoMessage | undefined> {
    const { message } = await rpc<{ message?: {
      id?: string; roomId?: string; actorId?: string; threadRootEventId?: string; inReplyTo?: string; body?: string;
    } }>("MessageService/GetMessage", { roomId, eventId: messageId }, signal);
    if (!message || message.id !== messageId || message.roomId !== roomId || typeof message.actorId !== "string" || !message.actorId) return;
    return { id: message.id, roomId: message.roomId, authorId: message.actorId,
      ...(message.threadRootEventId ? { threadRootId: message.threadRootEventId } : {}),
      ...(message.inReplyTo ? { inReplyTo: message.inReplyTo } : {}),
      ...(typeof message.body === "string" ? { body: message.body } : {}),
    };
  }

  /** Recognize DMs, viewer mentions, and verified replies to the viewer.
   * viewerId must come from this connection's getViewer(). Mentions use the
   * server's includesViewer flag. Only unmentioned non-DM replies require a lookup.
   * Self messages, non-message events, and unavailable text are ignored. Lookup
   * errors and cancellation propagate without logging or changing a checkpoint.
   */
  async function addressedMessage(event: RealtimeEvent, { viewerId, signal }: {
    viewerId: string; signal?: AbortSignal;
  }): Promise<AddressedMessage | undefined> {
    signal?.throwIfAborted();
    if (!viewerId || event.event.case !== "messagePosted" || !event.id || !event.actorId || event.actorId === viewerId) return;
    const message = event.event.value;
    if (message.bodyPlaintext === undefined || !message.roomId) return;
    const reasons: AddressingReason[] = [];
    if (message.roomKind === RoomKind.DM) reasons.push("direct_message");
    if (message.mentions.some(mention => mention.includesViewer)) reasons.push("mention");
    if (!reasons.length && message.inReplyTo) {
      const target = await getMessage({ roomId: message.roomId, messageId: message.inReplyTo, signal });
      signal?.throwIfAborted();
      if (target?.authorId === viewerId &&
          (target.threadRootId || target.id) === (message.threadRootEventId || message.inReplyTo)) reasons.push("reply");
    }
    if (!reasons.length) return;
    return { id: event.id, roomId: message.roomId, authorId: event.actorId, body: message.bodyPlaintext,
      ...(message.threadRootEventId ? { threadRootId: message.threadRootEventId } : {}),
      ...(message.inReplyTo ? { inReplyTo: message.inReplyTo } : {}),
      ...(event.createdAt ? { occurredAt: event.createdAt.toDate().toISOString() } : {}), reasons,
    };
  }

  return { getViewer, getMessage, addressedMessage };
}
