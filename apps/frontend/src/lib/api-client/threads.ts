import { authHeaders, createChattoClient, handleAuthError } from "./connect.js";
import { ThreadService } from "@chatto/api-types/api/v1/threads_connect";
import type { User } from "@chatto/api-types/api/v1/users_pb";
import type { RawEvent } from "./events.js";
import { roomTimelineEventToRawEvent } from "./roomTimeline.js";

export type ConnectAPIConfig = {
  serverId?: string;
  baseUrl: string;
  bearerToken: string | null;
  onAuthenticationRequired?: (serverId: string) => void;
};

export type FollowedThread = {
  roomId: string;
  roomName: string;
  threadRootEventId: string;
  rootMessage: RawEvent | null;
  replyCount: number;
  lastReplyAt: string | null;
  hasUnread: boolean;
};

export type FollowedThreadsPage = {
  threads: FollowedThread[];
  totalCount: number;
  hasMore: boolean;
};

export type ThreadFollowState = {
  roomId: string;
  threadRootEventId: string;
  following: boolean;
};

export type ThreadFollowResult = {
  following: boolean;
  state: ThreadFollowState | null;
};

export function createThreadAPI(config: ConnectAPIConfig) {
  const client = createChattoClient(ThreadService, config);
  const headers = () => authHeaders(config);
  return {
    async listFollowedThreads(input: {
      limit: number;
      offset: number;
    }): Promise<FollowedThreadsPage> {
      try {
        const response = await client.listFollowedThreads(
          { page: { limit: input.limit, offset: input.offset } },
          { headers: headers() },
        );
        const users = response.includes?.users ?? {};
        return {
          threads: response.threads.map((thread) => ({
            roomId: thread.roomId,
            roomName: thread.roomName,
            threadRootEventId: thread.threadRootEventId,
            rootMessage: thread.rootMessage
              ? roomTimelineEventToRawEvent(
                  thread.rootMessage,
                  users as Record<string, User>,
                )
              : null,
            replyCount: thread.replyCount,
            lastReplyAt: timestampToISOOrNull(thread.lastReplyAt),
            hasUnread: thread.hasUnread,
          })),
          totalCount: Number(response.page?.totalCount ?? 0),
          hasMore: response.page?.hasMore ?? false,
        };
      } catch (err) {
        return handleAuthError(config, err);
      }
    },

    async followThread(input: {
      roomId: string;
      threadRootEventId: string;
    }): Promise<ThreadFollowResult> {
      try {
        const response = await client.followThread(input, {
          headers: headers(),
        });
        return {
          following: response.following,
          state: response.state ? mapThreadFollowState(response.state) : null,
        };
      } catch (err) {
        return handleAuthError(config, err);
      }
    },

    async unfollowThread(input: {
      roomId: string;
      threadRootEventId: string;
    }): Promise<ThreadFollowResult> {
      try {
        const response = await client.unfollowThread(input, {
          headers: headers(),
        });
        return {
          following: response.following,
          state: response.state ? mapThreadFollowState(response.state) : null,
        };
      } catch (err) {
        return handleAuthError(config, err);
      }
    },
  };
}

function mapThreadFollowState(state: {
  roomId: string;
  threadRootEventId: string;
  following: boolean;
}): ThreadFollowState {
  return {
    roomId: state.roomId,
    threadRootEventId: state.threadRootEventId,
    following: state.following,
  };
}

function timestampToISOOrNull(
  timestamp: { toDate(): Date } | undefined,
): string | null {
  return timestamp ? timestamp.toDate().toISOString() : null;
}
