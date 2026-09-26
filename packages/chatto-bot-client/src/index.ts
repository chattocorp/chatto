import type {
  ChattoClient,
  ChattoMessage,
  Destination,
  RealtimeEvent,
  ThreadLocation,
  ThreadMessage
} from '@chatto/client';
import { addressedMessage, type AddressingOptions } from './addressing.js';
export {
  addressedMessage,
  type AddressedMessage,
  type AddressingReason,
  type AddressingOptions
} from './addressing.js';
export { createDeliveryTracker, type DeliveryTracker } from './deliveries.js';

/** Thread text classified relative to this bot; other bots also have the human role. */
export interface BotThreadMessage extends ThreadMessage {
  role: 'bot' | 'human';
}

/** Read thread text with roles relative to a known bot identity. */
export async function readBotThread(
  client: Pick<ChattoClient, 'readThread'>,
  viewerId: string,
  location: ThreadLocation,
  signal?: AbortSignal
): Promise<BotThreadMessage[]> {
  return (await client.readThread(location, signal)).map((message) => ({
    ...message,
    role: message.authorId === viewerId ? 'bot' : 'human'
  }));
}

/** Reply in the original thread and reference the message that prompted the reply. */
export function replyDestination(
  message: Pick<ChattoMessage, 'id' | 'roomId' | 'threadRootId'>
): Destination {
  return {
    roomId: message.roomId,
    threadRootId: message.threadRootId ?? message.id,
    inReplyTo: message.id
  };
}

/** Default conversation scope: bot, room, thread, and sender. Scope storage to one server.
 * Hosts can use their own key when participants should share a conversation. */
export function conversationKey(
  viewerId: string,
  message: Pick<ChattoMessage, 'id' | 'roomId' | 'threadRootId' | 'authorId'>
): string {
  return JSON.stringify([
    viewerId,
    message.roomId,
    message.threadRootId ?? message.id,
    message.authorId
  ]);
}

/** Resolve identity once for this client. Create a new adapter when credentials change.
 * This adds no connection, event loop, environment loading, or workflow lifecycle. */
export async function createBotClient(
  client: ChattoClient,
  { signal }: { signal?: AbortSignal } = {}
) {
  const { id: viewerId } = await client.getViewer({ signal });
  return {
    client,
    viewerId,
    addressedMessage: (event: RealtimeEvent, options: AddressingOptions = {}) =>
      addressedMessage(client, event, { viewerId, ...options }),
    conversationKey: (message: ChattoMessage) => conversationKey(viewerId, message),
    reply: (message: ChattoMessage, body: string, signal?: AbortSignal) =>
      client.postMessage(replyDestination(message), body, signal),
    readThread: (location: ThreadLocation, signal?: AbortSignal) =>
      readBotThread(client, viewerId, location, signal)
  };
}

export type BotClient = Awaited<ReturnType<typeof createBotClient>>;
