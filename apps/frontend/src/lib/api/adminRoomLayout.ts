export * from '@chatto/api-client/adminRoomLayout';
import { createAdminRoomLayoutAPI as createBaseAPI } from '@chatto/api-client/adminRoomLayout';
import { withAuthenticationRequired } from './clientHooks';
import type { AdminRoomLayoutAPIConfig } from '@chatto/api-client/adminRoomLayout';

export function createAdminRoomLayoutAPI(config: AdminRoomLayoutAPIConfig) {
  return createBaseAPI(withAuthenticationRequired(config));
}
