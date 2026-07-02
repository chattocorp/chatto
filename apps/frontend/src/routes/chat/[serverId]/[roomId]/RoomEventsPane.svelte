<script lang="ts">
  import { useEvent } from '$lib/hooks';
  import { RoomEventKind, roomEventKind, type RoomEventKindSource } from '$lib/render/eventKinds';
  import {
    getComposerContext,
    type RefreshCurrentWindowResult,
    type RoomMember
  } from '$lib/state/room';
  import type { MessagesStore } from '$lib/state/room';
  import EventList from './EventList.svelte';
  import type { OpenThreadHandler } from './threadOpenOptions';

  type MessageRetractedEventPayload = {
    roomId?: string | null;
    messageEventId?: string | null;
  };

  function messageRetractedPayload(
    event: RoomEventKindSource
  ): MessageRetractedEventPayload | null {
    if (roomEventKind(event) !== RoomEventKind.MessageRetracted) return null;
    if (!event || typeof event !== 'object') return null;
    return event as MessageRetractedEventPayload;
  }

  let {
    roomId,
    messageStore: store,
    unreadAfterTime = null,
    unreadBeforeTime = null,
    onOpenThread,
    typingUserIds = [],
    typingMembers = []
  }: {
    roomId: string;
    messageStore: MessagesStore;
    unreadAfterTime?: string | null;
    unreadBeforeTime?: string | null;
    onOpenThread?: OpenThreadHandler;
    typingUserIds?: string[];
    typingMembers?: RoomMember[];
  } = $props();

  const composerContext = getComposerContext();
  const editState = composerContext.editState;
  const jumpState = composerContext.jumpState;

  let roomEvents = $derived(store.rootEvents);
  let updateCounter = $derived(roomEvents.length);

  // Resolve time-based unread boundary to an event ID for EventList
  let unreadAfterEventId = $derived.by(() => {
    if (unreadAfterTime === null) return null;
    const afterMs = new Date(unreadAfterTime).getTime();
    const beforeMs = unreadBeforeTime ? new Date(unreadBeforeTime).getTime() : Infinity;
    for (const event of roomEvents) {
      const eventMs = new Date(event.createdAt).getTime();
      if (eventMs > afterMs && eventMs <= beforeMs) {
        return event.id;
      }
    }
    return null;
  });

  // Wire jumpState handlers to the store
  if (jumpState) {
    jumpState.setJumpHandler((eventId: string) => store.jumpToMessage(eventId, jumpState));
    jumpState.setLoadNewerHandler(() => store.loadNewer(jumpState));
  }

  // Reset jump state when room changes
  $effect(() => {
    void roomId;
    if (jumpState) jumpState.reset();
  });

  // Drive store loads from roomId changes. Silent reconnect + tab-resume
  // catch-ups refresh the current message window without resetting the store.
  $effect(() => {
    store.setRoom(roomId);
  });

  // Subscribe to server events: route to store, plus handle component-level
  // concerns the store doesn't own (e.g. cancel an in-progress edit).
  useEvent((serverEvent) => {
    const eventData = messageRetractedPayload(serverEvent.event);
    if (!eventData) {
      store.ingestServerEvent(serverEvent);
      return;
    }

    if (eventData.roomId === roomId && editState.eventId === eventData.messageEventId) {
      editState.cancelEdit();
    }

    store.ingestServerEvent(serverEvent);
  });

  function handleSoftRefresh(result: RefreshCurrentWindowResult, anchored: boolean): void {
    console.debug('[room-refresh] room pane refresh result', {
      roomId,
      anchored,
      hasOlder: result.hasOlder,
      hasNewer: result.hasNewer
    });
    if (!anchored || !jumpState) return;
    jumpState.isJumpedMode = result.hasNewer;
    jumpState.hasReachedEnd = !result.hasNewer;
    jumpState.hasOlderMessages = result.hasOlder;
    console.debug('[room-refresh] forward pagination state updated', {
      roomId,
      isJumpedMode: jumpState.isJumpedMode,
      hasReachedEnd: jumpState.hasReachedEnd,
      hasOlderMessages: jumpState.hasOlderMessages
    });
  }

  function handleReachedPresent(): void {
    if (!jumpState) return;

    console.debug('[room-refresh] exiting jumped mode at present', { roomId });
    jumpState.reset();
  }
</script>

<EventList
  {roomId}
  messageStore={store}
  events={roomEvents}
  alwaysScrollToBottom={false}
  showNewMessagesIndicator={true}
  enablePagination={true}
  isLoadingMore={store.isLoadingMore}
  hasReachedStart={store.hasReachedStart}
  onLoadMore={() => store.loadMore()}
  {updateCounter}
  {onOpenThread}
  enableLastEditableFinder={true}
  isLoading={store.isInitialLoading}
  {unreadAfterEventId}
  {typingUserIds}
  {typingMembers}
  scrollToEventId={jumpState?.scrollToEventId ?? null}
  onScrollToEventComplete={() => {
    if (jumpState) jumpState.scrollToEventId = null;
  }}
  isJumpedMode={jumpState?.isJumpedMode ?? false}
  isLoadingNewer={jumpState?.isLoadingNewer ?? false}
  hasReachedEnd={jumpState?.hasReachedEnd ?? false}
  onLoadNewer={() => store.loadNewer(jumpState)}
  onJumpToPresent={() => store.jumpToPresent(jumpState)}
  onReachedPresent={handleReachedPresent}
  onSoftRefresh={handleSoftRefresh}
/>
