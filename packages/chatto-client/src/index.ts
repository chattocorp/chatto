/**
 * The Chatto client.
 *
 * - {@link createClient} creates an isolated client. `client.connect()`
 *   adds a server with an API key; the server's `run()` handles the messages
 *   addressed to it. `client.server(id)` returns a server that the client
 *   has, the same type.
 * - {@link createApi} makes stateless requests, for example in a webhook
 *   handler.
 * - `@chatto/client/types` re-exports the protocol messages and services.
 *
 * Applications with a UI use the stores of a client, and the adapter in
 * `@chatto/client/svelte`.
 */

export { createClient, ChattoClient, type ClientOptions } from './client.js';
export {
  Server,
  type ConnectOptions,
  type ConsumeEventsOptions,
  type MessageContext,
  type RunOptions,
  type ServerStatus,
  type SnapshotInfo
} from './server/server.js';
export type { AuthorityChange, ProjectionReset, RoomAccessLoss } from './server/storeEvents.js';
export { Api, createApi, type ApiOptions } from './api.js';
export {
  conversationKey,
  MessagingRequests,
  replyDestination,
  type AddressedMessage,
  type AddressingOptions,
  type AddressingReason,
  type ServiceSource
} from './messaging/requests.js';
export type {
  ChattoMessage,
  Destination,
  RealtimeStatus,
  RequestOptions,
  ThreadLocation,
  ThreadMessage,
  ThreadRead,
  ThreadReadOptions
} from './messaging/types.js';
export { withTyping, startTyping, type TypingUpdate } from './messaging/typing.js';
export { createDeliveryTracker, type DeliveryTracker } from './messaging/deliveries.js';
export type { LiveServers } from './server/realtimeTransport.js';
export { setDebugLogging } from './util/debugLog.js';
export { RealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
export { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
