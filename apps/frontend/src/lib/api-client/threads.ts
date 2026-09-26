import { createChattoClient, type ConnectAPIConfig } from './connect.js';
import { ThreadService } from '@chatto/api-types/api/v1/threads_connect';
import { MessageSearchService } from '@chatto/api-types/api/v1/message_search_connect';
import {
  MessageSearchScope,
  MessageSearchGroupBy,
  MessageSearchOrder
} from '@chatto/api-types/api/v1/message_search_pb';
import type { User } from '@chatto/api-types/api/v1/users_pb';
import type { TimelineEventView } from '$lib/render/timelineEvents';
import { messageToTimelineEvent } from './roomTimeline.js';
import type { UserAvatarUserView } from '$lib/render/users';
import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';

export type FollowedThread = {
  roomId: string;
  roomName: string;
  isDirectMessage: boolean;
  directMessageParticipants: UserAvatarUserView[];
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
  /** Search pages use opaque cursors; ordinary thread lists use offsets. */
  nextCursor?: string | null;
};

export type ThreadFollowState = {
  roomId: string;
  threadRootEventId: string;
  following: boolean;
};

export type ThreadFollowResult = {
  state: ThreadFollowState | null;
};

export function createThreadAPI(config: ConnectAPIConfig) {
  const client = createChattoClient(ThreadService, config);
  const search = createChattoClient(MessageSearchService, config);
  return {
    async listFollowedThreads(
      input: {
        limit: number;
        offset: number;
        /** Search roots and replies across all accessible followed threads. */
        query?: string;
        /** Continuation for a non-empty search query. Offset is ignored in this mode. */
        cursor?: string;
        /**
         * Ask the server for unread threads only. Ignored in search mode. Servers
         * without `followedThreadUnreadFilter` support ignore it and return all threads.
         */
        unreadOnly?: boolean;
      },
      options: { signal?: AbortSignal } = {}
    ): Promise<FollowedThreadsPage> {
      const query = input.query?.trim();
      const request = {
        includeDirectMessageThreads: true,
        unreadOnly: input.unreadOnly ?? false,
        page: { limit: input.limit, offset: input.offset }
      };
      const requestOptions = { signal: options.signal };
      const response = query
        ? await search
            .searchMessages(
              {
                query,
                scope: MessageSearchScope.FOLLOWED_THREADS,
                groupBy: MessageSearchGroupBy.THREAD,
                order: MessageSearchOrder.THREAD_ACTIVITY,
                pageSize: input.limit,
                cursor: input.cursor ?? ''
              },
              requestOptions
            )
            .then((result) => ({
              threads: result.results.flatMap((match) =>
                match.threadContext ? [match.threadContext] : []
              ),
              includes: result.includes,
              page: { totalCount: result.threadTotalCount ?? 0n, hasMore: !!result.nextCursor },
              nextCursor: result.nextCursor || null
            }))
        : await client.listFollowedThreads(request, requestOptions);
      const users = response.includes?.users ?? {};
      return {
        threads: response.threads.map((thread) => {
          const rootMessage = thread.rootMessage
            ? messageToTimelineEvent(thread.rootMessage, users as Record<string, User>)
            : null;
          return {
            roomId: thread.room?.id ?? '',
            roomName: thread.room?.name ?? '',
            isDirectMessage: thread.room?.kind === RoomKind.DM,
            directMessageParticipants: (thread.directMessageParticipantUserIds ?? [])
              .map((id) => users[id])
              .filter((user): user is User => user !== undefined)
              .map((user) => ({
                id: user.id,
                login: user.login,
                displayName: user.displayName,
                deleted: user.deleted,
                isBot: !!user.bot,
                avatarUrl: user.avatarUrl || null,
                presenceStatus: PresenceStatus.OFFLINE
              })),
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
        hasMore: response.page?.hasMore ?? false,
        ...('nextCursor' in response ? { nextCursor: response.nextCursor } : {})
      };
    },

    async followThread(input: {
      roomId: string;
      threadRootEventId: string;
    }): Promise<ThreadFollowResult> {
      const response = await client.followThread(input);
      return {
        state: response.state ? mapThreadFollowState(response.state) : null
      };
    },

    async unfollowThread(input: {
      roomId: string;
      threadRootEventId: string;
    }): Promise<ThreadFollowResult> {
      const response = await client.unfollowThread(input);
      return {
        state: response.state ? mapThreadFollowState(response.state) : null
      };
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
