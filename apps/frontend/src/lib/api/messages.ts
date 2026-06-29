import { createMessageAPI as createBaseAPI } from '@chatto/api-client/messages';
import { withAuthenticationRequired } from './clientHooks';
import type {
  MessageAPIConfig,
  PostMessageInput,
  UpdateMessageInput
} from '@chatto/api-client/messages';
import type { RoomEventView } from '$lib/render/types';

export type { MessageAPIConfig, PostMessageInput, UpdateMessageInput };

export type PostMessageResult =
  | {
      kind: 'event';
      event: RoomEventView | null;
    }
  | {
      kind: 'mentionConfirmation';
      recipientCount: number;
      token: string;
    };

export type MessageAPI = Omit<ReturnType<typeof createBaseAPI>, 'postMessage'> & {
  postMessage(input: PostMessageInput): Promise<PostMessageResult>;
};

export function createMessageAPI(config: MessageAPIConfig): MessageAPI {
  return createBaseAPI(withAuthenticationRequired(config)) as unknown as MessageAPI;
}
