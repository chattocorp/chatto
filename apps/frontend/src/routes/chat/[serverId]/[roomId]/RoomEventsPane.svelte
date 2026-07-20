<script lang="ts">
  import type { UnreadMarkerWindow } from '$lib/hooks';
  import { getComposerContext, type RoomMember } from '$lib/state/room';
  import type { MessagesStore } from '$lib/state/room';
  import TimelineEventsPane from './TimelineEventsPane.svelte';
  import type { OpenThreadHandler } from './threadOpenOptions';
  import * as m from '$lib/i18n/messages';
  import { toast } from '$lib/ui/toast';

  let {
    roomId,
    messageStore: store,
    unreadMarkerEventId = null,
    unreadMarkerWindow = null,
    onUnreadMarkerResolved,
    onUnreadMarkerCleared,
    onOpenThread,
    pendingHighlightId = null,
    onHighlightComplete,
    typingUserIds = [],
    typingMembers = []
  }: {
    roomId: string;
    messageStore: MessagesStore;
    unreadMarkerEventId?: string | null;
    unreadMarkerWindow?: UnreadMarkerWindow | null;
    onUnreadMarkerResolved?: (eventId: string) => void;
    onUnreadMarkerCleared?: () => void;
    onOpenThread?: OpenThreadHandler;
    pendingHighlightId?: string | null;
    onHighlightComplete?: () => void;
    typingUserIds?: string[];
    typingMembers?: RoomMember[];
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

<TimelineEventsPane
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
  {unreadMarkerEventId}
  {unreadMarkerWindow}
  {onUnreadMarkerResolved}
  {typingUserIds}
  {typingMembers}
  scrollToEventId={jumpState?.scrollToEventId ?? null}
  onScrollToEventComplete={(landed) => {
    if (jumpState) jumpState.scrollToEventId = null;
    onHighlightComplete?.();
    if (!landed) toast.error(m['room.jump_failed']());
  }}
  isJumpedMode={jumpState?.isJumpedMode ?? false}
  isLoadingNewer={jumpState?.isLoadingNewer ?? false}
  hasReachedEnd={jumpState?.hasReachedEnd ?? false}
  onLoadNewer={() => store.loadNewer(jumpState)}
  onJumpToPresent={() => store.jumpToPresent(jumpState)}
  onReachedPresent={handleReachedPresent}
  {onUnreadMarkerCleared}
  {pendingHighlightId}
/>
