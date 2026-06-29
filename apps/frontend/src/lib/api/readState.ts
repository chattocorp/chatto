export * from '@chatto/api-client/readState';
import { createReadStateAPI as createBaseAPI } from '@chatto/api-client/readState';
import { withAuthenticationRequired } from './clientHooks';
import type { ConnectAPIConfig } from '@chatto/api-client/readState';

export function createReadStateAPI(config: ConnectAPIConfig) {
  return createBaseAPI(withAuthenticationRequired(config));
}
