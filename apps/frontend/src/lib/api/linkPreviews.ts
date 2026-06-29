export * from '@chatto/api-client/linkPreviews';
import { createLinkPreviewAPI as createBaseAPI } from '@chatto/api-client/linkPreviews';
import { withAuthenticationRequired } from './clientHooks';
import type { LinkPreviewAPIConfig } from '@chatto/api-client/linkPreviews';

export function createLinkPreviewAPI(config: LinkPreviewAPIConfig) {
  return createBaseAPI(withAuthenticationRequired(config));
}
