export * from '@chatto/api-client/roomDirectory';
import { createRoomDirectoryAPI as createBaseAPI } from '@chatto/api-client/roomDirectory';
import { withAuthenticationRequired } from './clientHooks';
import type { RoomDirectoryAPIConfig } from '@chatto/api-client/roomDirectory';

export function createRoomDirectoryAPI(config: RoomDirectoryAPIConfig) {
  return createBaseAPI(withAuthenticationRequired(config));
}
