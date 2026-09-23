import { RoomKind, type ChattoClient, type ChattoMessage, type RealtimeEvent } from "@chatto/client";

export type AddressingReason = "direct_message" | "mention" | "reply";

/** Select bot addressing conventions. All three reasons are enabled by default. */
export interface AddressingOptions {
  signal?: AbortSignal;
  reasons?: readonly AddressingReason[];
}

/** A textual realtime message addressed to the authenticated viewer. */
export interface AddressedMessage extends ChattoMessage {
  body: string;
  occurredAt?: string;
  reasons: AddressingReason[];
}

/** Recognize DMs, viewer mentions, and verified replies to the viewer.
 * viewerId must come from this connection's getViewer(). Mentions use the
 * server's includesViewer flag. Only unmentioned non-DM replies require a lookup.
 * Self messages, non-message events, and unavailable text are ignored. Lookup
 * errors and cancellation propagate without logging or changing a checkpoint.
 */
export async function addressedMessage(client: Pick<ChattoClient, "getMessage">, event: RealtimeEvent, {
  viewerId, signal, reasons: enabled = ["direct_message", "mention", "reply"],
}: AddressingOptions & { viewerId: string }): Promise<AddressedMessage | undefined> {
  signal?.throwIfAborted();
  if (!viewerId || event.event.case !== "messagePosted" || !event.id || !event.actorId || event.actorId === viewerId) return;
  const message = event.event.value;
  if (message.bodyPlaintext === undefined || !message.roomId) return;
  const reasons: AddressingReason[] = [];
  if (enabled.includes("direct_message") && message.roomKind === RoomKind.DM) reasons.push("direct_message");
  if (enabled.includes("mention") && message.mentions.some(mention => mention.includesViewer)) reasons.push("mention");
  if (enabled.includes("reply") && !reasons.length && message.inReplyTo) {
    const target = await client.getMessage({ roomId: message.roomId, messageId: message.inReplyTo, signal });
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
