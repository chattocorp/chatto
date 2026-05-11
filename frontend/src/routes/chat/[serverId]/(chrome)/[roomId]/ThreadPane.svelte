<script lang="ts">
  import { fly } from 'svelte/transition';
  import { graphql } from '$lib/gql';
  import type { RoomEventViewFragment } from '$lib/gql/graphql';
  import { useSpaceEvent, useReconnectTrigger, createTypingIndicator } from '$lib/hooks';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';

  const getInstanceId = getActiveServer();
  const notificationStore = serverRegistry.getStore(getInstanceId()).notifications;
  import { appState } from '$lib/state/globals.svelte';
  import { getRoomMembers, createComposerContext, ThreadMessagesStore } from '$lib/state/room';
  import { getCurrentUser } from '$lib/auth/currentUser.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import MessageComposer, {
    type MessageComposerApi
  } from '$lib/components/composer/MessageComposer.svelte';
  import EventList from './EventList.svelte';
  import { onThreadFollowChanged } from '$lib/serverEventBus.svelte';

  let {
    roomId,
    roomName,
    threadRootEventId,
    onClose,
    canPostInThread = true,
    canEchoMessage = false,
    highlightEventId = null,
    onHighlightComplete
  }: {
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

  const store = new ThreadMessagesStore(
    connection().client,
    () => currentUser.user?.id ?? null
  );

  let threadEvents = $derived(store.threadEvents);
  let updateCounter = $derived(threadEvents.length);

  // Track the timestamp when the thread was last opened (for unread separator)
  let unreadAfterTime = $state<Date | null>(null);
  // Upper bound - messages arriving after we opened the thread don't show the separator
  let unreadBeforeTime = $state<Date | null>(null);

  // Resolve time-based unread boundary to an event ID for EventList
  let unreadAfterEventId = $derived.by(() => {
    if (unreadAfterTime === null) return null;
    const afterTime = unreadAfterTime.getTime();
    const beforeTime = unreadBeforeTime?.getTime() ?? Infinity;
    for (const event of threadEvents) {
      const eventTime = new Date(event.createdAt).getTime();
      if (eventTime > afterTime && eventTime <= beforeTime) {
        return event.id;
      }
    }
    return null;
  });

  // Typing indicator for this thread
  const typingIndicator = createTypingIndicator(() => ({
    roomId,
    threadRootEventId,
    currentUserId: currentUser.user?.id ?? null
  }));

  // Create thread-scoped contexts that shadow the parent Room's contexts.
  // `{ scroll: true }` gives the thread its own ScrollState so the composer's
  // scroll-to-bottom-on-own-post request lands on the *thread's* EventList,
  // not the main room's.
  const composerContext = createComposerContext({ scroll: true });
  const replyState = composerContext.replyState;

  // Thread-scoped jump state so "in reply to" clicks scroll within the thread.
  const jumpState = composerContext.jumpState;
  jumpState.setJumpHandler(async (eventId: string) => {
    jumpState.scrollToEventId = eventId;
  });

  let canPost = $derived(canPostInThread);

  const reconnect = useReconnectTrigger();

  // Reload thread events when the thread changes or WebSocket reconnects
  $effect(() => {
    void reconnect.count;
    store.setThread(roomId, threadRootEventId);
  });

  // Jump to a specific message when highlightEventId prop is set
  $effect(() => {
    if (!highlightEventId || store.isInitialLoading) return;
    jumpState.jumpToMessage(highlightEventId);
  });

  // Subscribe to space events: clear typing indicator on a thread reply
  // (component-level concern), then forward to the store.
  useSpaceEvent((spaceEvent: RoomEventViewFragment) => {
    const eventData = spaceEvent.event;
    if (!eventData) return;

    if (
      eventData.__typename === 'MessagePostedEvent' &&
      eventData.roomId === roomId &&
      eventData.inThread === threadRootEventId
    ) {
      typingIndicator.removeTypingUser(spaceEvent.actorId);
    }

    store.ingestSpaceEvent(spaceEvent);
  });

  // -- Thread follow state --
  // Subscription events (auto-follow on reply, cross-tab sync) are authoritative.
  // If one fires for this thread before the initial query resolves we must not
  // let the query's stale viewerIsFollowingThread clobber it. Track per-thread
  // so that switching to a different thread starts fresh.
  let isFollowingThread = $state(false);
  let _followSeededForThread = '';
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
      if (!store.isInitialLoading) {
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
      input: { roomId, threadRootEventId }
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
    if (!appState.isFocused) return;
    const threadId = threadRootEventId;
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
        { input: { roomId, threadRootEventId: currentThreadId } }
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
    {roomId}
    events={threadEvents}
    alwaysScrollToBottom={false}
    showNewMessagesIndicator={true}
    enablePagination={false}
    filterThreadReplies={false}
    {updateCounter}
    enableLastEditableFinder={true}
    isLoading={store.isInitialLoading}
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
