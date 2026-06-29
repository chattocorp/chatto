export * from '@chatto/api-client/attachments';
import { createAttachmentAPI as createBaseAPI } from '@chatto/api-client/attachments';
import { withAuthenticationRequired } from './clientHooks';
import type { AttachmentAPIConfig } from '@chatto/api-client/attachments';

export function createAttachmentAPI(config: AttachmentAPIConfig) {
  return createBaseAPI(withAuthenticationRequired(config));
}
