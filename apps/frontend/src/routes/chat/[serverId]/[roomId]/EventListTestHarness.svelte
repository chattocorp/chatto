<script lang="ts">
  import { onMount } from 'svelte';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { TimelineEventKind, type TimelineEventView } from '$lib/render/timelineEvents';
  import {
    createComposerContext,
    createRoomMembers,
    createRoomPermissions,
    DEFAULT_ROOM_PERMISSIONS,
    type ComposerContext
  } from '$lib/state/room';
  import EventList from './EventList.svelte';

  let {
    eventIds,
    roomId = 'room-1',
    permalinkThreadRootEventId = null,
    eventKind = 'message',
    scrollToEventId,
    onComplete,
    isLoading = false,
    isJumpedMode = false,
    onJumpToPresent,
    pendingHighlightId = null,
    hasReachedStart = false,
    recoveryViewport = null,
    unreadAfterEventId = null,
    onComposerReady,
    onStoreRead
  }: {
    eventIds: string[];
    roomId?: string;
    permalinkThreadRootEventId?: string | null;
    eventKind?: 'message' | 'join';
    scrollToEventId: string | null;
    onComplete?: () => void;
    isLoading?: boolean;
    isJumpedMode?: boolean;
    onJumpToPresent?: () => Promise<boolean>;
    pendingHighlightId?: string | null;
    hasReachedStart?: boolean;
    recoveryViewport?: { eventId: string; offset: number; hasNewer?: boolean } | null;
    unreadAfterEventId?: string | null;
    onComposerReady?: (context: ComposerContext) => void;
    onStoreRead?: () => void;
  } = $props();

  const composerContext = createComposerContext();
  onMount(() => onComposerReady?.(composerContext));
  // Production panes drive these through the shared jump state.
  $effect.pre(() => {
    composerContext.jumpState.isJumpedMode = isJumpedMode;
  });
  $effect.pre(() => {
    composerContext.jumpState.scrollToEventId = scrollToEventId;
  });
  createRoomPermissions(() => DEFAULT_ROOM_PERMISSIONS);
  createRoomMembers();

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

  // Match Room's derived store selection so delayed work sees real owner disposal.
  const messageStore = $derived({
    get isInitialLoading() {
      return isLoading;
    },
    isLoadingMore: false,
    get hasReachedStart() {
      return hasReachedStart;
    },
    loadMore: async () => {},
    loadNewer: async () => {},
    jumpToPresent: async () => (await onJumpToPresent?.()) ?? false,
    get recoveryViewport() {
      onStoreRead?.();
      return recoveryViewport;
    },
    set recoveryViewport(value) {
      recoveryViewport = value;
    },
    clearViewport: () => {
      recoveryViewport = null;
    },
    setViewport: () => {},
    refreshCurrentWindow: async () => ({
      hasOlder: false,
      hasNewer: false,
      refreshed: false,
      changed: false
    })
  });
</script>

<output data-testid="recovery-anchor">{recoveryViewport?.eventId ?? ''}</output>

<EventList
  {roomId}
  {permalinkThreadRootEventId}
  messageStore={messageStore as never}
  {events}
  {pendingHighlightId}
  {unreadAfterEventId}
  onScrollToEventComplete={onComplete}
/>
