import {
  authHeaders,
  createChattoClient,
  handleAuthError,
  type ConnectAPIConfig
} from './connect.js';
import { ThreadService } from '@chatto/api-types/api/v1/threads_connect';
import type { User } from '@chatto/api-types/api/v1/users_pb';
import type { TimelineEventView } from '$lib/render/timelineEvents';
import { messageToTimelineEvent } from './roomTimeline.js';
import type { UserAvatarUserView } from '$lib/render/users';

export type FollowedThread = {
  roomId: string;
  roomName: string;
  threadRootEventId: string;
  rootMessage: TimelineEventView | null;
  latestReply: TimelineEventView | null;
  replyCount: number;
  lastReplyAt: string | null;
  participants: UserAvatarUserView[];
  participantCount: number;
  hasUnreadReplies: boolean;
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
    async listFollowedThreads(
      input: {
        limit: number;
        offset: number;
      },
      options: { signal?: AbortSignal } = {}
    ): Promise<FollowedThreadsPage> {
      try {
        const response = await client.listFollowedThreads(
          { page: { limit: input.limit, offset: input.offset } },
          {
            headers: headers(),
            ...(options.signal ? { signal: options.signal } : {})
          }
        );
        const users = response.includes?.users ?? {};
        return {
          threads: response.threads.map((thread) => {
            const rootMessage = thread.rootMessage
              ? messageToTimelineEvent(thread.rootMessage, users as Record<string, User>)
              : null;
            return {
              roomId: thread.room?.id ?? '',
              roomName: thread.room?.name ?? '',
              threadRootEventId: thread.thread?.threadRootEventId ?? '',
              rootMessage,
              latestReply: thread.latestReply
                ? messageToTimelineEvent(thread.latestReply, users as Record<string, User>)
                : null,
              replyCount: thread.thread?.replyCount ?? 0,
              lastReplyAt: timestampToISOOrNull(thread.thread?.lastReplyAt),
              participants:
                rootMessage?.event.kind === 'messagePosted'
                  ? rootMessage.event.threadParticipants
                  : [],
              participantCount: thread.thread?.participantCount ?? 0,
              hasUnreadReplies: thread.thread?.viewerState?.hasUnreadReplies ?? false
            };
          }),
          totalCount: Number(response.page?.totalCount ?? 0),
          hasMore: response.page?.hasMore ?? false
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
          headers: headers()
        });
        return {
          following: response.following,
          state: response.state ? mapThreadFollowState(response.state) : null
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
          headers: headers()
        });
        return {
          following: response.following,
          state: response.state ? mapThreadFollowState(response.state) : null
        };
      } catch (err) {
        return handleAuthError(config, err);
      }
    }
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
    following: state.following
  };
}

function timestampToISOOrNull(timestamp: { toDate(): Date } | undefined): string | null {
  return timestamp ? timestamp.toDate().toISOString() : null;
}
