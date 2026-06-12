<script lang="ts">
  import { onDestroy } from 'svelte';
  import { fly } from 'svelte/transition';
  import { graphql } from '$lib/gql';
  import { useEvent, createTypingIndicator } from '$lib/hooks';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';

  const notificationStore = serverRegistry.getStore(getActiveServer()).notifications;
  import { appState } from '$lib/state/globals.svelte';
  import { getRoomMembers, createComposerContext, MessagesStore } from '$lib/state/room';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import HeaderIconButton from '$lib/ui/HeaderIconButton.svelte';
  import MessageComposer, {
    type MessageComposerApi
  } from '$lib/components/composer/MessageComposer.svelte';
  import EventList from './EventList.svelte';
  import { onThreadFollowChanged } from '$lib/eventBus.svelte';

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
  const currentUser = $derived(serverRegistry.getStore(getActiveServer()).currentUser);

  const store = new MessagesStore(
    connection(),
    () => currentUser.user?.id ?? null
  );
  onDestroy(() => store.dispose());

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

  // Reload thread events when the thread prop changes. Silent reconnect +
  // tab-resume catch-ups are owned by the store itself.
  $effect(() => {
    store.setThread(roomId, threadRootEventId);
  });

  // Jump to a specific message when highlightEventId prop is set
  $effect(() => {
    if (!highlightEventId || store.isInitialLoading) return;
    jumpState.jumpToMessage(highlightEventId);
  });

  // Subscribe to server events: clear typing indicator on a thread reply,
  // forward to the store, and mark the thread as read (with explicit event
  // ID) for replies arriving from other users while the user is present.
  useEvent((serverEvent) => {
    const eventData = serverEvent.event;
    if (!eventData) return;

    if (
      eventData.__typename === 'MessagePostedEvent' &&
      eventData.roomId === roomId &&
      eventData.threadRootEventId === threadRootEventId
    ) {
      const actorId = serverEvent.actorId;
      if (actorId) {
        typingIndicator.removeTypingUser(actorId);
      }

      if (
        currentUser.user &&
        actorId !== currentUser.user.id &&
        appState.isPresent
      ) {
        void markThreadAsRead(threadRootEventId, serverEvent.id);
      }
    }

    store.ingestServerEvent(serverEvent);
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

  async function markThreadAsRead(currentThreadId: string, upToEventId?: string) {
    const result = await connection()
      .client.mutation(
        graphql(`
          mutation MarkThreadAsRead($input: MarkThreadAsReadInput!) {
            markThreadAsRead(input: $input) {
              previousReadAt
            }
          }
        `),
        { input: { roomId, threadRootEventId: currentThreadId, upToEventId } }
      )
      .toPromise();

    if (result.error) {
      console.error('Failed to mark thread as read:', result.error);
    }
    return result.data?.markThreadAsRead ?? null;
  }

  // Fire mark-thread-as-read on every presence-true edge (fresh open or
  // refocus/tab-reveal) and on thread changes while present. The result
  // drives the unread separator so a refocus shows what arrived during
  // the away period.
  let lastFiredThreadId = '';
  let wasPresentThread = false;

  $effect(() => {
    const currentThreadId = threadRootEventId;
    const present = appState.isPresent;

    if (!present) {
      // Presence-false edge: anchor the unread separator at "now" with no
      // upper bound so replies arriving while the user is away show up
      // below the marker in real time, rather than only on return. The
      // presence-true edge below refines it when the user comes back.
      if (wasPresentThread) {
        unreadAfterTime = new Date();
        unreadBeforeTime = null;
      }
      wasPresentThread = false;
      return;
    }

    if (wasPresentThread && lastFiredThreadId === currentThreadId) return;
    wasPresentThread = true;
    lastFiredThreadId = currentThreadId;

    unreadAfterTime = null;
    unreadBeforeTime = null;

    const openedAt = new Date();
    markThreadAsRead(currentThreadId).then((data) => {
      if (!data) return;
      const prevTime = data.previousReadAt;
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
  <PaneHeader
    title="Thread in #{roomName}"
    onBack={onClose}
    backLabel="Back to room"
  >
    {#snippet actions()}
      <HeaderIconButton
        icon={isFollowingThread ? 'uil--bell' : 'uil--bell-slash'}
        label={isFollowingThread ? 'Unfollow thread' : 'Follow thread'}
        tone={isFollowingThread ? 'active' : 'default'}
        onclick={toggleThreadFollow}
      />
      <HeaderIconButton icon="uil--times" label="Close thread" onclick={onClose} />
    {/snippet}
  </PaneHeader>

  <EventList
    {roomId}
    messageStore={store}
    events={threadEvents}
    alwaysScrollToBottom={false}
    showNewMessagesIndicator={true}
    enablePagination={true}
    isLoadingMore={store.isLoadingMore}
    hasReachedStart={store.hasReachedStart}
    onLoadMore={() => store.loadMore()}
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
