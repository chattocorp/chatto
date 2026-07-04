import { authHeaders, createChattoClient, handleAuthError } from "./connect.js";
import { MessageService } from "@chatto/api-types/api/v1/messages_connect";
import type { MessageReaction } from "@chatto/api-types/api/v1/message_types_pb";

export type ConnectAPIConfig = {
  serverId?: string;
  baseUrl: string;
  bearerToken: string | null;
  onAuthenticationRequired?: (serverId: string) => void;
};

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

export function createReactionAPI(config: ConnectAPIConfig) {
  const client = createChattoClient(MessageService, config);
  const headers = () => authHeaders(config);
  return {
    async addReaction(input: ReactionInput): Promise<AddReactionResult> {
      try {
        const response = await client.addReaction(input, {
          headers: headers(),
        });
        return {
          added: response.added,
          reaction: mapReactionSummary(response.reaction),
        };
      } catch (err) {
        return handleAuthError(config, err);
      }
    },

    async removeReaction(input: ReactionInput): Promise<RemoveReactionResult> {
      try {
        const response = await client.removeReaction(input, {
          headers: headers(),
        });
        return {
          removed: response.removed,
          reaction: mapReactionSummary(response.reaction),
        };
      } catch (err) {
        return handleAuthError(config, err);
      }
    },
  };
}

function mapReactionSummary(
  reaction: MessageReaction | undefined,
): ReactionSummary | null {
  if (!reaction || !reaction.emoji) return null;
  return {
    emoji: reaction.emoji,
    count: reaction.count,
    hasReacted: reaction.hasReacted,
    previewUserIds: [...reaction.previewUserIds],
  };
}
