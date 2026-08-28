<script lang="ts">
  import { getComposerContext, type RoomMember } from '$lib/state/room';
  import type { MessagesStore } from '$lib/state/room';
  import EventList from './EventList.svelte';
  import type { OpenThreadHandler } from './threadOpenOptions';
  import { m } from '$lib/i18n/messages';
  import { toast } from '$lib/ui/toast';
  import { RoomThreadingMode } from '$lib/roomThreading';

  let {
    roomId,
    messageStore: store,
    unreadMarkerEventId = null,
    onUnreadMarkerCleared,
    onOpenThread,
    onOpenCall,
    onOpenProfile,
    pendingHighlightId = null,
    onHighlightComplete,
    typingUserIds = [],
    typingMembers = [],
    threadingMode = RoomThreadingMode.ENABLED
  }: {
    roomId: string;
    messageStore: MessagesStore;
    unreadMarkerEventId?: string | null;
    onUnreadMarkerCleared?: () => void;
    onOpenThread?: OpenThreadHandler;
    onOpenCall?: () => void;
    onOpenProfile?: (userId: string) => void;
    pendingHighlightId?: string | null;
    onHighlightComplete?: () => void;
    typingUserIds?: string[];
    typingMembers?: RoomMember[];
    threadingMode?: RoomThreadingMode;
  } = $props();

  const composerContext = getComposerContext();
  const editState = composerContext.editState;
  const jumpState = composerContext.jumpState;

  let roomEvents = $derived(store.rootEvents);
  let updateCounter = $derived(roomEvents.length);

  // Projection v2 folds retractions and crypto-erasure into the authoritative
  // message row. Keep composer state aligned without requiring a second
  // legacy event-envelope path.
  $effect(() => {
    const editingEventId = editState.eventId;
    if (!editingEventId) return;
    const editingEvent = roomEvents.find((event) => event.id === editingEventId);
    const payload = editingEvent?.event;
    if (payload && 'deletedAt' in payload && payload.deletedAt) editState.cancelEdit();
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

  // Drive store loads from roomId changes. Reconnect convergence belongs to
  // the resumable server projection and does not trigger a parallel room read.
  $effect(() => {
    store.setRoom(roomId);
  });

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
  {onOpenCall}
  {onOpenProfile}
  enableLastEditableFinder={true}
  isLoading={store.isInitialLoading}
  unreadAfterEventId={unreadMarkerEventId}
  {typingUserIds}
  {typingMembers}
  scrollToEventId={jumpState?.scrollToEventId ?? null}
  onScrollToEventComplete={(landed) => {
    if (jumpState) jumpState.scrollToEventId = null;
    onHighlightComplete?.();
    if (!landed) toast.error(m('room.jump_failed'));
  }}
  isJumpedMode={jumpState?.isJumpedMode ?? false}
  isLoadingNewer={jumpState?.isLoadingNewer ?? false}
  hasReachedEnd={jumpState?.hasReachedEnd ?? false}
  onLoadNewer={() => store.loadNewer(jumpState)}
  onJumpToPresent={() => store.jumpToPresent(jumpState)}
  onReachedPresent={handleReachedPresent}
  onReachedBottom={onUnreadMarkerCleared}
  {pendingHighlightId}
  {threadingMode}
/>
