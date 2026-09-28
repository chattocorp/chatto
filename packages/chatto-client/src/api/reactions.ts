import { createChattoClient, type ConnectAPIConfig } from './connect.js';
import { MessageService } from '@chatto/api-types/api/v1/messages_connect';
import type { MessageReaction } from '@chatto/api-types/api/v1/message_types_pb';

export type ReactionInput = {
  roomId: string;
  messageEventId: string;
  emoji: string;
};

export type ReactionSummary = {
  emoji: string;
  count: number;
  hasReacted: boolean;
  previewUserIds: string[];
};

export type AddReactionResult = {
  added: boolean;
  reaction: ReactionSummary | null;
};

export type RemoveReactionResult = {
  removed: boolean;
  reaction: ReactionSummary | null;
};

/** One bounded page of accounts that gave a specific reaction. */
export type ReactionUsersPage = {
  userIds: string[];
  totalCount: number;
  hasMore: boolean;
};

export function createReactionAPI(config: ConnectAPIConfig) {
  const client = createChattoClient(MessageService, config);
  return {
    async addReaction(input: ReactionInput): Promise<AddReactionResult> {
      const response = await client.addReaction(input);
      return {
        added: response.added,
        reaction: mapReactionSummary(response.reaction)
      };
    },

    async removeReaction(input: ReactionInput): Promise<RemoveReactionResult> {
      const response = await client.removeReaction(input);
      return {
        removed: response.removed,
        reaction: mapReactionSummary(response.reaction)
      };
    },

    async listReactionUsers(
      input: ReactionInput,
      offset: number,
      limit = 50,
      signal?: AbortSignal
    ): Promise<ReactionUsersPage> {
      const response = await client.listReactionUsers(
        { ...input, page: { offset, limit } },
        { signal }
      );
      return {
        userIds: [...response.userIds],
        totalCount: Number(response.page?.totalCount ?? 0),
        hasMore: response.page?.hasMore ?? false
      };
    }
  };
}

function mapReactionSummary(reaction: MessageReaction | undefined): ReactionSummary | null {
  if (!reaction || !reaction.emoji) return null;
  return {
    emoji: reaction.emoji,
    count: reaction.count,
    hasReacted: reaction.hasReacted,
    previewUserIds: [...reaction.previewUserIds]
  };
}
