export * from '@chatto/api-client/threads';
import { createThreadAPI as createBaseAPI } from '@chatto/api-client/threads';
import { withAuthenticationRequired } from './clientHooks';
import type { ConnectAPIConfig } from '@chatto/api-client/threads';

export function createThreadAPI(config: ConnectAPIConfig) {
  return createBaseAPI(withAuthenticationRequired(config));
}
