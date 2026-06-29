export * from '@chatto/api-client/presence';
import { createPresenceAPI as createBaseAPI } from '@chatto/api-client/presence';
import { withAuthenticationRequired } from './clientHooks';
import type { PresenceAPIConfig } from '@chatto/api-client/presence';

export function createPresenceAPI(config: PresenceAPIConfig) {
  return createBaseAPI(withAuthenticationRequired(config));
}
