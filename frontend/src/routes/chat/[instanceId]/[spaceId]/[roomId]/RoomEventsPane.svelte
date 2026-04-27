<script lang="ts">
  import type { RoomEventViewFragment } from '$lib/gql/graphql';
  import { useSpaceEvent, useReconnectTrigger } from '$lib/hooks';
  import { useConnection } from '$lib/state/instance/connection.svelte';
  import { getComposerContext, RoomMessagesStore, type RoomMember } from '$lib/state/room';
  import { getCurrentUser } from '$lib/auth/currentUser.svelte';
  import EventList from './EventList.svelte';

  let {
    spaceId,
    roomId,
    unreadAfterTime = null,
    unreadBeforeTime = null,
    onOpenThread,
    typingUserIds = [],
    typingMembers = []
  }: {
    spaceId: string;
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
  const currentUser = getCurrentUser();

  const store = new RoomMessagesStore(
    connection().client,
    () => currentUser.user?.id ?? null
  );

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

  const reconnect = useReconnectTrigger();

  // Track previous values to distinguish room changes from reconnects
  let prevSpaceId: string | undefined;
  let prevRoomId: string | undefined;
  let prevRefetchTrigger: number | undefined;

  // Drive store loads from prop / reconnect / refetchTrigger changes
  $effect(() => {
    void reconnect.count;
    void refetchTrigger;

    const isFirstLoad = prevRoomId === undefined;
    const isRoomChange =
      !isFirstLoad && (prevSpaceId !== spaceId || prevRoomId !== roomId);
    const isRefetch =
      prevRefetchTrigger !== undefined && prevRefetchTrigger !== refetchTrigger;

    prevSpaceId = spaceId;
    prevRoomId = roomId;
    prevRefetchTrigger = refetchTrigger;

    // Show skeletons on first load, room change, or refetch trigger.
    // On reconnect, keep stale messages visible and refetch silently.
    const mode = isFirstLoad || isRoomChange || isRefetch ? 'reset' : 'catchUp';
    store.setRoom(spaceId, roomId, mode);
  });

  // Subscribe to space events: route to store, plus handle component-level
  // concerns the store doesn't own (e.g. cancel an in-progress edit).
  useSpaceEvent((spaceEvent: RoomEventViewFragment) => {
    const eventData = spaceEvent.event;
    if (!eventData) return;

    if (
      eventData.__typename === 'MessageDeletedEvent' &&
      eventData.roomId === roomId &&
      editState.eventId === eventData.messageEventId
    ) {
      editState.cancelEdit();
    }

    store.ingestSpaceEvent(spaceEvent);
  });
</script>

<EventList
  {spaceId}
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
