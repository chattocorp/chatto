<script lang="ts">
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import {
    TimelineEventKind,
    type TimelineEventView
  } from '$lib/render/timelineEvents';
  import {
    createComposerContext,
    createRoomPermissions,
    DEFAULT_ROOM_PERMISSIONS
  } from '$lib/state/room';
  import EventList from './EventList.svelte';

  let {
    eventIds,
    roomId = 'room-1',
    eventKind = 'message',
    scrollToEventId,
    onComplete,
    isLoading = false,
    isJumpedMode = false,
    onJumpToPresent,
    updateCounter = 0,
    pendingHighlightId = null,
    hasReachedStart = false
  }: {
    eventIds: string[];
    roomId?: string;
    eventKind?: 'message' | 'join';
    scrollToEventId: string | null;
    onComplete?: () => void;
    isLoading?: boolean;
    isJumpedMode?: boolean;
    onJumpToPresent?: () => Promise<boolean>;
    updateCounter?: number;
    pendingHighlightId?: string | null;
    hasReachedStart?: boolean;
  } = $props();

  createComposerContext({ scroll: true });
  createRoomPermissions(() => DEFAULT_ROOM_PERMISSIONS);

  const events = $derived(
    eventIds.map((id, index): TimelineEventView => {
      const base = {
        id,
        createdAt: `2026-06-17T10:47:${String(index).padStart(2, '0')}Z`,
        actorId: `user-${id}`,
        actor: {
          id: `user-${id}`,
          login: id,
          displayName: `User ${id}`,
          deleted: false,
          avatarUrl: null,
          presenceStatus: PresenceStatus.OFFLINE
        }
      };
      if (eventKind === 'join') {
        return {
          ...base,
          event: {
            kind: TimelineEventKind.UserJoinedRoom,
            roomId
          }
        } as unknown as TimelineEventView;
      }
      return {
        ...base,
        event: {
          kind: TimelineEventKind.MessagePosted,
          roomId,
          body: id,
          attachments: [],
          linkPreview: null,
          reactions: [],
          updatedAt: null,
          inReplyTo: null,
          threadRootEventId: null,
          echoOfEventId: null,
          echoFromThreadRootEventId: null,
          channelEchoEventId: null,
          replyCount: 0,
          lastReplyAt: null,
          threadParticipants: [],
          viewerIsFollowingThread: true
        }
      } as TimelineEventView;
    })
  );

  const messageStore = {
    refreshCurrentWindow: async () => ({
      hasOlder: false,
      hasNewer: false,
      refreshed: false,
      changed: false
    })
  };
</script>

<EventList
  {roomId}
  messageStore={messageStore as never}
  {events}
  {isLoading}
  {isJumpedMode}
  {onJumpToPresent}
  {updateCounter}
  {pendingHighlightId}
  {hasReachedStart}
  {scrollToEventId}
  onScrollToEventComplete={onComplete}
/>
