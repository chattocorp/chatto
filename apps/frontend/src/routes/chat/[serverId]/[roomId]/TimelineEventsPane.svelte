<script lang="ts">
  import type { UnreadMarkerWindow } from '$lib/hooks';
  import * as m from '$lib/i18n/messages';
  import type { RoomEventView } from '$lib/render/types';
  import {
    type MessagesStore,
    type RefreshCurrentWindowResult,
    type RoomMember
  } from '$lib/state/room';
  import EventList from './EventList.svelte';
  import type { OpenThreadHandler } from './threadOpenOptions';

  let {
    roomId,
    messageStore,
    events,
    updateCounter = events.length,
    unreadMarkerEventId = null,
    unreadMarkerWindow = null,
    unreadMarkerSkipActorId = null,
    onUnreadMarkerResolved,
    onUnreadMarkerCleared,
    alwaysScrollToBottom = false,
    showNewMessagesIndicator = true,
    enablePagination = false,
    isLoadingMore = false,
    hasReachedStart = false,
    showStartMarker = true,
    onLoadMore,
    onOpenThread,
    filterThreadReplies = true,
    enableLastEditableFinder = false,
    isLoading = false,
    emptyMessage = m['room.message.empty'](),
    typingUserIds = [],
    typingMembers = [],
    scrollToEventId = null,
    onScrollToEventComplete,
    isJumpedMode = false,
    isLoadingNewer = false,
    hasReachedEnd = false,
    onLoadNewer,
    onJumpToPresent,
    onReachedPresent,
    onSoftRefresh,
    pendingHighlightId = null
  }: {
    roomId: string;
    messageStore: MessagesStore;
    events: RoomEventView[];
    updateCounter?: number;
    unreadMarkerEventId?: string | null;
    unreadMarkerWindow?: UnreadMarkerWindow | null;
    unreadMarkerSkipActorId?: string | null;
    onUnreadMarkerResolved?: (eventId: string) => void;
    onUnreadMarkerCleared?: () => void;
    alwaysScrollToBottom?: boolean;
    showNewMessagesIndicator?: boolean;
    enablePagination?: boolean;
    isLoadingMore?: boolean;
    hasReachedStart?: boolean;
    showStartMarker?: boolean;
    onLoadMore?: () => Promise<void>;
    onOpenThread?: OpenThreadHandler;
    filterThreadReplies?: boolean;
    enableLastEditableFinder?: boolean;
    isLoading?: boolean;
    emptyMessage?: string;
    typingUserIds?: string[];
    typingMembers?: RoomMember[];
    scrollToEventId?: string | null;
    onScrollToEventComplete?: (landed: boolean) => void;
    isJumpedMode?: boolean;
    isLoadingNewer?: boolean;
    hasReachedEnd?: boolean;
    onLoadNewer?: () => Promise<void>;
    onJumpToPresent?: () => void;
    onReachedPresent?: () => void;
    onSoftRefresh?: (result: RefreshCurrentWindowResult, anchored: boolean) => void;
    pendingHighlightId?: string | null;
  } = $props();

  // Resolve a fresh-entry server timestamp window once, then commit the
  // concrete event id back to the unread hook. EventList only renders an
  // explicit event-id marker.
  let resolvedUnreadMarkerEventId = $derived.by(() => {
    if (unreadMarkerWindow === null) return null;
    const afterMs = Date.parse(unreadMarkerWindow.afterTime);
    const beforeMs =
      typeof unreadMarkerWindow.beforeTime === 'number'
        ? unreadMarkerWindow.beforeTime
        : Date.parse(unreadMarkerWindow.beforeTime);

    for (const event of events) {
      if (unreadMarkerSkipActorId && event.actorId === unreadMarkerSkipActorId) continue;

      const eventMs = Date.parse(event.createdAt);
      if (eventMs > afterMs && eventMs <= beforeMs) {
        return event.id;
      }
    }
    return null;
  });

  $effect(() => {
    if (!resolvedUnreadMarkerEventId) return;
    onUnreadMarkerResolved?.(resolvedUnreadMarkerEventId);
  });
</script>

<EventList
  {roomId}
  {messageStore}
  {events}
  {alwaysScrollToBottom}
  {showNewMessagesIndicator}
  {enablePagination}
  {isLoadingMore}
  {hasReachedStart}
  {showStartMarker}
  {onLoadMore}
  {onOpenThread}
  {filterThreadReplies}
  {updateCounter}
  {enableLastEditableFinder}
  {isLoading}
  {emptyMessage}
  unreadAfterEventId={unreadMarkerEventId}
  {typingUserIds}
  {typingMembers}
  {scrollToEventId}
  {onScrollToEventComplete}
  {isJumpedMode}
  {isLoadingNewer}
  {hasReachedEnd}
  {onLoadNewer}
  {onJumpToPresent}
  {onReachedPresent}
  onReachedBottom={onUnreadMarkerCleared}
  {onSoftRefresh}
  {pendingHighlightId}
/>
