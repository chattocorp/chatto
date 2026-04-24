<script lang="ts">
  import { fly } from 'svelte/transition';
  import { graphql, useFragment } from '$lib/gql';
  import { RoomEventViewFragmentDoc, type RoomEventViewFragment } from '$lib/gql/graphql';
  import { useSpaceEvent, useReconnectTrigger, createTypingIndicator } from '$lib/hooks';
  import { useConnection } from '$lib/state/instance/connection.svelte';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';

  const getInstanceId = getActiveInstance();
  const notificationStore = instanceRegistry.getStore(getInstanceId()).notifications;
  import { appState } from '$lib/state/globals.svelte';
  import { getRoomMembers, createComposerContext } from '$lib/state/room';
  import { getCurrentUser } from '$lib/auth/currentUser.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import MessageComposer, {
    type MessageComposerApi
  } from '$lib/components/composer/MessageComposer.svelte';
  import EventList from './EventList.svelte';
  import { onThreadFollowChanged } from '$lib/instanceEventBus.svelte';

  let {
    spaceId,
    roomId,
    roomName,
    threadRootEventId,
    onClose,
    canPostInThread = true,
    canEchoMessage = false,
    highlightEventId = null,
    onHighlightComplete
  }: {
    spaceId: string;
    roomId: string;
    roomName: string;
    threadRootEventId: string;
    onClose: () => void;
    canPostInThread?: boolean;
    canEchoMessage?: boolean;
    highlightEventId?: string | null;
    onHighlightComplete?: () => void;
  } = $props();

  const connection = useConnection();
  const members = $derived(getRoomMembers());
  const currentUser = getCurrentUser();

  // --- Local state (replaces SpaceMessageCache) ---
  let events = $state<RoomEventViewFragment[]>([]);
  // eslint-disable-next-line svelte/prefer-svelte-reactivity -- dedup tracker, not read reactively
  let seenIds = new Set<string>();

  function getEventKey(event: RoomEventViewFragment): string {
    return event.id;
  }

  // Track the timestamp when the thread was last opened (for unread separator)
  let unreadAfterTime = $state<Date | null>(null);
  // Upper bound - messages arriving after we opened the thread don't show the separator
  let unreadBeforeTime = $state<Date | null>(null);

  // Resolve time-based unread boundary to an event ID for EventList
  let unreadAfterEventId = $derived.by(() => {
    if (unreadAfterTime === null) return null;
    const afterTime = unreadAfterTime.getTime();
    const beforeTime = unreadBeforeTime?.getTime() ?? Infinity;
    for (const event of events) {
      const eventTime = new Date(event.createdAt).getTime();
      if (eventTime > afterTime && eventTime <= beforeTime) {
        return event.id;
      }
    }
    return null;
  });

  // Typing indicator for this thread
  const typingIndicator = createTypingIndicator(() => ({
    spaceId,
    roomId,
    threadRootEventId,
    currentUserId: currentUser.user?.id ?? null
  }));

  // Create thread-scoped contexts that shadow the parent Room's contexts
  const composerContext = createComposerContext();
  const replyState = composerContext.replyState;

  // Thread-scoped jump state so "in reply to" clicks scroll within the thread.
  const jumpState = composerContext.jumpState;
  jumpState.setJumpHandler(async (eventId: string) => {
    jumpState.scrollToEventId = eventId;
  });

  // Filter: include root message OR any message in this thread (by inThread)
  function isThreadEvent(event: RoomEventViewFragment): boolean {
    const eventData = event.event;
    if (!eventData || !('roomId' in eventData) || eventData.roomId !== roomId) return false;

    // Only include messages, not system events
    if (eventData.__typename !== 'MessagePostedEvent') return false;

    // Is this the root message?
    if (event.id === threadRootEventId) return true;

    // Is this message part of this thread?
    return eventData.inThread === threadRootEventId;
  }

  let threadEvents = $derived(events.filter(isThreadEvent));

  // Determine if the user can post in this thread
  let canPost = $derived(canPostInThread);

  // Track updates for scroll triggering
  let updateCounter = $derived(threadEvents.length);

  let isLoading = $state(true);

  const reconnect = useReconnectTrigger();

  // Track current load to handle race conditions when thread changes mid-load
  let loadId = { current: 0 };

  // Load thread events when thread changes or WebSocket reconnects
  $effect(() => {
    // Re-run on WebSocket reconnect to fill message gaps from suspension
    void reconnect.count;

    const thisLoadId = ++loadId.current;
    const currentThreadId = threadRootEventId;

    // Reset local state
    events = [];
    seenIds = new Set();
    isLoading = true;

    connection().client
      .query(
        graphql(`
          query ThreadEventsQuery($spaceId: ID!, $roomId: ID!, $threadRootEventId: ID!) {
            threadEvents(
              spaceId: $spaceId
              roomId: $roomId
              threadRootEventId: $threadRootEventId
            ) {
              ...RoomEventView
            }
          }
        `),
        { spaceId, roomId, threadRootEventId: currentThreadId }
      )
      .toPromise()
      .then((result) => {
        // Skip if thread changed while loading
        if (loadId.current !== thisLoadId) return;

        if (result.error) {
          console.error('Failed to load thread events:', result.error);
        }

        if (result.data?.threadEvents) {
          const fetched = result.data.threadEvents
            .map((event) => useFragment(RoomEventViewFragmentDoc, event))
            .filter((e): e is RoomEventViewFragment => e !== null);

          // Merge fetched events with any subscription events that arrived while
          // the query was in flight (e.g. the user's own reply or a fast cross-user
          // reply). Overwriting would drop them and the test only recovers if
          // another event later nudges the subscription handler.
          const nextSeenIds = new Set<string>();
          const merged: RoomEventViewFragment[] = [];
          for (const e of fetched) {
            const key = getEventKey(e);
            if (nextSeenIds.has(key)) continue;
            nextSeenIds.add(key);
            merged.push(e);
          }
          for (const e of events) {
            const key = getEventKey(e);
            if (nextSeenIds.has(key)) continue;
            nextSeenIds.add(key);
            merged.push(e);
          }
          events = merged;
          seenIds = nextSeenIds;
        }
        isLoading = false;
      })
      .catch((error: unknown) => {
        // Skip if thread changed while loading
        if (loadId.current !== thisLoadId) return;
        console.error('Thread events query failed:', error);
        isLoading = false;
      });
  });

  // Jump to a specific message when highlightEventId prop is set
  $effect(() => {
    if (!highlightEventId || isLoading) return;
    jumpState.jumpToMessage(highlightEventId);
  });

  // Refetch a message by event ID and replace it in the local array
  async function refetchMessage(eventId: string) {
    const result = await connection().client
      .query(
        graphql(`
          query RefetchMessageByEventId($spaceId: ID!, $roomId: ID!, $eventId: ID!) {
            roomEventByEventId(spaceId: $spaceId, roomId: $roomId, eventId: $eventId) {
              ...RoomEventView
            }
          }
        `),
        { spaceId, roomId, eventId },
        { requestPolicy: 'network-only' }
      )
      .toPromise();

    if (result.data?.roomEventByEventId) {
      const updatedEvent = useFragment(RoomEventViewFragmentDoc, result.data.roomEventByEventId);
      if (updatedEvent) {
        const idx = events.findIndex((e) => e.id === eventId);
        if (idx !== -1) {
          events[idx] = updatedEvent;
        }
      }
    }
  }

  // Refetch all visible events (used when a user is deleted)
  async function refetchAllEvents() {
    for (const event of $state.snapshot(threadEvents)) {
      await refetchMessage(event.id);
    }
  }

  // Handle live events for this thread
  useSpaceEvent((spaceEvent) => {
    const eventData = spaceEvent.event;
    if (!eventData) return;

    // When a space member's account is deleted, refetch all visible messages
    if (eventData.__typename === 'SpaceMemberDeletedEvent') {
      refetchAllEvents();
      return;
    }

    // Clear typing indicator when someone posts in this thread
    if (eventData.__typename === 'MessagePostedEvent') {
      if (eventData.roomId !== roomId) return;
      if (eventData.inThread === threadRootEventId) {
        typingIndicator.removeTypingUser(spaceEvent.actorId);

        // Add the new reply to local state
        const key = getEventKey(spaceEvent);
        if (!seenIds.has(key)) {
          seenIds.add(key);
          events.push(spaceEvent);
        }
      }
      return;
    }

    // Handle message edits/deletes - refetch affected events
    if (
      eventData.__typename === 'MessageUpdatedEvent' ||
      eventData.__typename === 'MessageDeletedEvent'
    ) {
      if (eventData.roomId !== roomId) return;
      for (const e of events) {
        if (e.id === eventData.messageEventId) {
          refetchMessage(e.id);
          break;
        }
      }
      return;
    }

    // Handle reaction events - refetch the reacted message
    if (
      eventData.__typename === 'ReactionAddedEvent' ||
      eventData.__typename === 'ReactionRemovedEvent'
    ) {
      if (eventData.roomId !== roomId) return;
      const target = events.find((e) => e.id === eventData.messageEventId);
      if (target) {
        refetchMessage(target.id);
      }
      return;
    }

    // Handle video processing completed - refetch affected events
    if (eventData.__typename === 'VideoProcessingCompletedEvent') {
      if (eventData.roomId !== roomId) return;
      for (const e of events) {
        if (e.id === eventData.messageEventId) {
          refetchMessage(e.id);
          break;
        }
      }
    }
  });

  // Thread follow state — managed as plain $state
  let isFollowingThread = $state(false);
  let _followSeededForThread = '';
  // Subscription events (auto-follow on reply, cross-tab sync) are authoritative.
  // If one fires for this thread before the initial query resolves we must not
  // let the query's stale viewerIsFollowingThread clobber it. Track per-thread
  // so that switching to a different thread starts fresh.
  let _followSubFiredForThread = '';

  $effect(() => {
    const threadId = threadRootEventId;

    if (threadId !== _followSeededForThread) {
      // Only reset if the subscription hasn't already authoritatively set the
      // state for this thread (auto-follow can fire before the initial query
      // resolves).
      if (_followSubFiredForThread !== threadId) {
        isFollowingThread = false;
      }

      // Wait until data has loaded before reading follow state
      if (!isLoading) {
        _followSeededForThread = threadId;
        if (_followSubFiredForThread !== threadId) {
          const rootEvent = threadEvents.find((e) => e.id === threadId);
          if (rootEvent?.event?.__typename === 'MessagePostedEvent') {
            isFollowingThread = rootEvent.event.viewerIsFollowingThread ?? false;
          }
        }
      }
    }
  });

  const followThreadMutation = graphql(`
    mutation FollowThreadFromPane($input: FollowThreadInput!) {
      followThread(input: $input)
    }
  `);

  const unfollowThreadMutation = graphql(`
    mutation UnfollowThreadFromPane($input: UnfollowThreadInput!) {
      unfollowThread(input: $input)
    }
  `);

  async function toggleThreadFollow() {
    const wasFollowing = isFollowingThread;
    isFollowingThread = !wasFollowing;

    const mutation = wasFollowing ? unfollowThreadMutation : followThreadMutation;
    const result = await connection().client.mutation(mutation, {
      input: { spaceId, roomId, threadRootEventId }
    });

    if (result.error) {
      isFollowingThread = wasFollowing;
    }
  }

  // Sync thread follow state from live events (auto-follow on reply, cross-tab sync).
  $effect(() =>
    onThreadFollowChanged((update) => {
      if (update.threadRootEventId === threadRootEventId) {
        isFollowingThread = update.isFollowing;
        _followSubFiredForThread = update.threadRootEventId;
      }
    })
  );

  // Dismiss reply notifications when viewing this thread (only when window is focused)
  $effect(() => {
    // Only dismiss when the window is focused
    if (!appState.isFocused) return;

    // Establish explicit dependency on threadRootEventId
    const threadId = threadRootEventId;
    // Dismiss notifications for this thread (fire and forget)
    notificationStore.dismissThreadNotifications(threadId);
  });

  // Mark thread as opened and capture previous timestamp for unread separator
  $effect(() => {
    const currentThreadId = threadRootEventId;

    // Reset unread markers when switching threads
    unreadAfterTime = null;
    unreadBeforeTime = null;

    // Capture the time when we opened - messages after this shouldn't show separator
    const openedAt = new Date();

    connection().client
      .mutation(
        graphql(`
          mutation MarkThreadAsOpened($input: MarkThreadAsOpenedInput!) {
            markThreadAsOpened(input: $input) {
              previousOpenedAt
            }
          }
        `),
        { input: { spaceId, roomId, threadRootEventId: currentThreadId } }
      )
      .toPromise()
      .then((result) => {
        if (result.error) {
          console.error('Failed to mark thread as opened:', result.error);
          return;
        }
        const prevTime = result.data?.markThreadAsOpened.previousOpenedAt;
        unreadAfterTime = prevTime ? new Date(prevTime) : null;
        unreadBeforeTime = openedAt;
      });
  });
</script>

<div
  class="absolute inset-y-0 right-0 z-10 flex min-h-0 w-full min-w-0 flex-col overflow-hidden border-l border-border bg-background shadow-[-4px_0_12px_rgba(0,0,0,0.15)] sm:w-[90%]"
  data-testid="thread-pane"
  transition:fly={{ x: 300, duration: 200 }}
>
  <PaneHeader title="Thread in #{roomName}">
    {#snippet prefix()}
      <button
        class="iconify cursor-pointer text-xl text-muted uil--arrow-left hover:text-text"
        onclick={onClose}
        title="Back to room"
      >
      </button>
    {/snippet}
    {#snippet actions()}
      <button
        class={[
          'iconify cursor-pointer text-xl hover:text-text',
          isFollowingThread ? 'text-text uil--bell' : 'text-muted uil--bell-slash'
        ]}
        onclick={toggleThreadFollow}
        title={isFollowingThread ? 'Unfollow thread' : 'Follow thread'}
      >
      </button>
      <button
        class="iconify cursor-pointer text-xl text-muted uil--times hover:text-text"
        onclick={onClose}
        title="Close thread"
      >
      </button>
    {/snippet}
  </PaneHeader>

  <EventList
    {spaceId}
    {roomId}
    events={threadEvents}
    alwaysScrollToBottom={false}
    showNewMessagesIndicator={true}
    enablePagination={false}
    filterThreadReplies={false}
    {updateCounter}
    enableLastEditableFinder={true}
    {isLoading}
    emptyMessage="Thread not found"
    {unreadAfterEventId}
    typingUserIds={typingIndicator.userIds}
    typingMembers={members}
    scrollToEventId={jumpState.scrollToEventId}
    onScrollToEventComplete={() => {
      jumpState.scrollToEventId = null;
      onHighlightComplete?.();
    }}
    pendingHighlightId={highlightEventId}
  />
  <MessageComposer
    {spaceId}
    {roomId}
    inThread={threadRootEventId}
    inReplyTo={replyState.messageEventId ?? undefined}
    replyDisplayName={replyState.actorDisplayName || undefined}
    replyExcerpt={replyState.excerpt || undefined}
    onCancelReply={() => replyState.cancelReply()}
    placeholder="Reply in thread..."
    {canPost}
    showAlsoSendToChannel={canEchoMessage}
    onEscape={onClose}
    onReady={(api: MessageComposerApi) => api.focus()}
    onTyping={() => typingIndicator?.sendTypingIndicator()}
    onMessageSent={() => typingIndicator?.resetDebounce()}
  />
</div>
