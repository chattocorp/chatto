/**
 * Framework-neutral Chatto client.
 *
 * The root entry serves headless hosts such as bots: {@link connectChatto}
 * connects one server with an API key and keeps its state live, and
 * {@link createChattoApi} makes stateless typed requests. Applications with a UI use the same
 * stores through the module entries, for example
 * `@chatto/client/server/registry`, and the adapter in `@chatto/client/svelte`.
 */

export { createChattoApi, type ChattoApi, type ChattoApiOptions } from './apiClient.js';
export {
  connectChatto,
  type ChattoConnection,
  type ChattoConnectionStatus,
  type ChattoReset,
  type ConnectChattoOptions
} from './connect.js';
export { setDebugLogging } from './util/debugLog.js';
export { RealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
export { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
