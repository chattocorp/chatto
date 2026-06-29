export * from '@chatto/api-client/reactions';
import { createReactionAPI as createBaseAPI } from '@chatto/api-client/reactions';
import { withAuthenticationRequired } from './clientHooks';
import type { ConnectAPIConfig } from '@chatto/api-client/reactions';

export function createReactionAPI(config: ConnectAPIConfig) {
  return createBaseAPI(withAuthenticationRequired(config));
}
