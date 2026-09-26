/** Message fields used by integrations; not a complete renderable message. */
export interface ChattoMessage {
  id: string;
  roomId: string;
  authorId: string;
  threadRootId?: string;
  inReplyTo?: string;
  body?: string;
}

type Rpc = <T>(method: string, body: object, signal?: AbortSignal) => Promise<T>;

/** Bind message helpers to the client's authenticated, bounded transport. */
export function messageHelpers(rpc: Rpc) {
  /** Resolve the authenticated viewer's identity. No environment or identity cache. */
  async function getViewer({ signal }: { signal?: AbortSignal } = {}): Promise<{ id: string }> {
    const response = await rpc<{ user?: { profile?: { id?: string } } }>(
      'ViewerService/GetViewer',
      {},
      signal
    );
    const id = response.user?.profile?.id;
    if (typeof id !== 'string' || !id) throw new Error('Chatto did not return the viewer identity');
    return { id };
  }

  /** Read one visible message. Missing or malformed identities fail closed.
   * RPC failures propagate; hosts choose whether to retry or ignore them. */
  async function getMessage({
    roomId,
    messageId,
    signal
  }: {
    roomId: string;
    messageId: string;
    signal?: AbortSignal;
  }): Promise<ChattoMessage | undefined> {
    const { message } = await rpc<{
      message?: {
        id?: string;
        roomId?: string;
        actorId?: string;
        threadRootEventId?: string;
        inReplyTo?: string;
        body?: string;
      };
    }>('MessageService/GetMessage', { roomId, eventId: messageId }, signal);
    if (
      !message ||
      message.id !== messageId ||
      message.roomId !== roomId ||
      typeof message.actorId !== 'string' ||
      !message.actorId
    )
      return;
    return {
      id: message.id,
      roomId: message.roomId,
      authorId: message.actorId,
      ...(message.threadRootEventId ? { threadRootId: message.threadRootEventId } : {}),
      ...(message.inReplyTo ? { inReplyTo: message.inReplyTo } : {}),
      ...(typeof message.body === 'string' ? { body: message.body } : {})
    };
  }

  return { getViewer, getMessage };
}
