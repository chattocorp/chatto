<script lang="ts">
  import { onDestroy } from 'svelte';
  import { useEvent } from '$lib/hooks';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { getComposerContext, RoomMessagesStore, type RoomMember } from '$lib/state/room';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import EventList from './EventList.svelte';

  let {
    roomId,
    unreadAfterTime = null,
    unreadBeforeTime = null,
    onOpenThread,
    typingUserIds = [],
    typingMembers = []
  }: {
    roomId: string;
    unreadAfterTime?: string | null;
    unreadBeforeTime?: string | null;
    onOpenThread?: (threadRootEventId: string, highlightEventId?: string) => void;
    typingUserIds?: string[];
    typingMembers?: RoomMember[];
  } = $props();

  const connection = useConnection();
  const composerContext = getComposerContext();
  const editState = composerContext.editState;
  const jumpState = composerContext.jumpState;
  const currentUser = $derived(serverRegistry.getStore(getActiveServer()).currentUser);

  const store = new RoomMessagesStore(
    connection(),
    () => currentUser.user?.id ?? null
  );
  onDestroy(() => store.dispose());

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

  let refetchTrigger = $state(0);

  // Wire jumpState handlers to the store
  if (jumpState) {
    jumpState.setJumpHandler((eventId: string) => store.jumpToMessage(eventId, jumpState));
    jumpState.setJumpToPresentHandler(() => store.jumpToPresent(jumpState));
    jumpState.setLoadNewerHandler(() => store.loadNewer(jumpState));
  }

  // Reset jump state when room changes
  $effect(() => {
    void roomId;
    if (jumpState) jumpState.reset();
  });

  // Drive store loads from roomId / manual-refetch prop changes. Silent
  // reconnect + tab-resume catch-ups are owned by the store itself — they
  // do not flow through this effect.
  $effect(() => {
    void refetchTrigger;
    store.setRoom(roomId);
  });

  // Subscribe to server events: route to store, plus handle component-level
  // concerns the store doesn't own (e.g. cancel an in-progress edit).
  useEvent((serverEvent) => {
    const eventData = serverEvent.event;
    if (!eventData) return;

    if (
      (eventData.__typename === 'MessageRetractedEvent' ||
        eventData.__typename === 'MessageDeletedEvent') &&
      eventData.roomId === roomId &&
      editState.eventId === eventData.messageEventId
    ) {
      editState.cancelEdit();
    }

    store.ingestServerEvent(serverEvent);
  });
</script>

<EventList
  {roomId}
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
/>
