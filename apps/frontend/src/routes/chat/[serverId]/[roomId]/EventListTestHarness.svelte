<script lang="ts">
  import { untrack } from 'svelte';
  import { setTimelineViewport } from '$lib/state/room/timelineViewport';
  import type { MessagesStore } from '@chatto/client/room/messages/MessagesStore';
  import { onMount } from 'svelte';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import {
    TimelineEventKind,
    type TimelineEventView
  } from '@chatto/client/timeline/timelineEvents';
  import {
    createComposerContext,
    createRoomMembers,
    createRoomPermissions,
    DEFAULT_ROOM_PERMISSIONS,
    type ComposerContext
  } from '$lib/state/room';
  import EventList from './EventList.svelte';
  import type { TimelineReadPosition } from './readThroughTracker';
  import type { PendingHighlight } from '$lib/state/server/pendingHighlight';

  let {
    eventIds,
    roomId = 'room-1',
    permalinkThreadRootEventId = null,
    eventKind = 'message',
    eventKinds = {},
    scrollToEventId,
    onComplete,
    isLoading = false,
    loadError = null,
    isJumpedMode = false,
    onJumpToPresent,
    onPresentRequested,
    pendingHighlightId = null,
    highlightRequest = null,
    hasReachedStart = false,
    recoveryViewport = null,
    unreadAfterEventId = null,
    onComposerReady,
    onStoreRead,
    onReadPosition,
    onReachedBottom,
    onClearAnchor,
    onLoadMore
  }: {
    eventIds: string[];
    roomId?: string;
    permalinkThreadRootEventId?: string | null;
    eventKind?: 'message' | 'join' | 'leave';
    /** Override individual rows for mixed activity and message windows. */
    eventKinds?: Record<string, 'message' | 'join' | 'leave' | 'deleted'>;
    scrollToEventId: string | null;
    onComplete?: () => void;
    isLoading?: boolean;
    loadError?: unknown;
    isJumpedMode?: boolean;
    onJumpToPresent?: () => Promise<boolean>;
    onPresentRequested?: () => void;
    pendingHighlightId?: string | null;
    highlightRequest?: PendingHighlight | null;
    hasReachedStart?: boolean;
    recoveryViewport?: { eventId: string; offset: number; hasNewer?: boolean } | null;
    unreadAfterEventId?: string | null;
    onComposerReady?: (context: ComposerContext) => void;
    onStoreRead?: () => void;
    onReadPosition?: (position: TimelineReadPosition) => void;
    onReachedBottom?: () => void;
    /** Called when the timeline clears the store's anchor. */
    onClearAnchor?: () => void;
    /** Supplies an older page for pagination tests. */
    onLoadMore?: () => Promise<void>;
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
      const kind = eventKinds[id] ?? eventKind;
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
      if (kind === 'join' || kind === 'leave') {
        return {
          ...base,
          event: {
            kind:
              kind === 'join' ? TimelineEventKind.UserJoinedRoom : TimelineEventKind.UserLeftRoom,
            roomId
          }
        } as unknown as TimelineEventView;
      }
      return {
        ...base,
        event: {
          kind: TimelineEventKind.MessagePosted,
          roomId,
          body: kind === 'deleted' ? null : id,
          deletedAt: kind === 'deleted' ? base.createdAt : null,
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
    get loadError() {
      return loadError;
    },
    isLoadingMore: false,
    get hasReachedStart() {
      return hasReachedStart;
    },
    loadMore: async () => {
      await onLoadMore?.();
    },
    canLoadNewer: false,
    loadNewer: async () => ({ status: 'declined' }),
    jumpToLatest: async () => (await onJumpToPresent?.()) ?? false,
    get recoveryAnchor() {
      onStoreRead?.();
      return recoveryViewport
        ? { eventId: recoveryViewport.eventId, hasNewer: recoveryViewport.hasNewer }
        : null;
    },
    completeRecovery: () => {
      recoveryViewport = null;
    },
    clearAnchor: () => {
      recoveryViewport = null;
      onClearAnchor?.();
    },
    setAnchor: () => true,
    refreshCurrentWindow: async () => ({
      hasOlder: false,
      hasNewer: false,
      refreshed: false,
      changed: false
    })
  });
  // Keep the recovery offset where the room view keeps it.
  untrack(() => {
    if (recoveryViewport) {
      setTimelineViewport(messageStore as unknown as MessagesStore, recoveryViewport);
    }
  });
</script>

<output data-testid="recovery-anchor">{recoveryViewport?.eventId ?? ''}</output>

<EventList
  {roomId}
  {permalinkThreadRootEventId}
  messageStore={messageStore as never}
  {events}
  {pendingHighlightId}
  {highlightRequest}
  {unreadAfterEventId}
  onScrollToEventComplete={onComplete}
  onJumpToPresent={onPresentRequested}
  {onReadPosition}
  {onReachedBottom}
/>
