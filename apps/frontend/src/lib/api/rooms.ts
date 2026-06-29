export * from '@chatto/api-client/rooms';
import { createRoomCommandAPI as createBaseAPI } from '@chatto/api-client/rooms';
import { withAuthenticationRequired } from './clientHooks';
import type { ConnectAPIConfig } from '@chatto/api-client/rooms';

export function createRoomCommandAPI(config: ConnectAPIConfig) {
  return createBaseAPI(withAuthenticationRequired(config));
}
