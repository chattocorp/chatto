import { notifyAuthenticationRequired } from "./hooks.js";
import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { MessageService } from "@chatto/api-types/api/v1/messages_connect";
import type { RoomTimelineReaction } from "@chatto/api-types/api/v1/room_timeline_pb";

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
  const transport = createConnectTransport({
    baseUrl: config.baseUrl,
    useBinaryFormat: true,
  });
  const client = createClient(MessageService, transport);
  const headers = () =>
    config.bearerToken
      ? { Authorization: `Bearer ${config.bearerToken}` }
      : undefined;

  async function handleAuthError(err: unknown): Promise<never> {
    if (
      err instanceof ConnectError &&
      err.code === Code.Unauthenticated &&
      config.serverId
    ) {
      notifyAuthenticationRequired(
        config.serverId,
        config.onAuthenticationRequired,
      );
    }
    throw err;
  }

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
        return handleAuthError(err);
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
        return handleAuthError(err);
      }
    },
  };
}

function mapReactionSummary(
  reaction: RoomTimelineReaction | undefined,
): ReactionSummary | null {
  if (!reaction || !reaction.emoji) return null;
  return {
    emoji: reaction.emoji,
    count: reaction.count,
    hasReacted: reaction.hasReacted,
    previewUserIds: [...reaction.previewUserIds],
  };
}
