<script lang="ts">
  import { onDestroy } from 'svelte';
  import { fly } from 'svelte/transition';
  import { createReadStateAPI } from '$lib/api-client/readState';
  import { createThreadAPI } from '$lib/api-client/threads';
  import { useEvent, createTypingIndicator } from '$lib/hooks';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { isMessagePostedEvent } from '$lib/render/eventKinds';
  import * as m from '$lib/i18n/messages';

  const stores = serverRegistry.getStore(getActiveServer());
  const notificationStore = stores.notifications;
  import { appState } from '$lib/state/globals.svelte';
  import {
    getRoomMembers,
    createComposerContext,
    MessagesStore,
    type QuoteInsertionRequest
  } from '$lib/state/room';
  import { onRoomMessageMutated } from '$lib/state/room/messageMutationEvents';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import HeaderIconButton from '$lib/ui/HeaderIconButton.svelte';
  import MessageComposer, {
    type MessageComposerApi
  } from '$lib/components/composer/MessageComposer.svelte';
  import EventList from './EventList.svelte';
  import { onThreadFollowChanged } from '$lib/eventBus.svelte';
  import type { PendingThreadReplyRequest } from './threadOpenOptions';

  let {
    roomId,
    roomName,
    threadRootEventId,
    onClose,
    canPostInThread = true,
    canAttach = true,
    canEchoMessage = false,
    highlightEventId = null,
    pendingQuote = null,
    pendingReply = null,
    onHighlightComplete,
    onQuoteConsumed,
    onReplyConsumed
  }: {
    roomId: string;
    roomName: string;
    threadRootEventId: string;
    onClose: () => void;
    canPostInThread?: boolean;
    canAttach?: boolean;
    canEchoMessage?: boolean;
    highlightEventId?: string | null;
    pendingQuote?: QuoteInsertionRequest | null;
    pendingReply?: PendingThreadReplyRequest | null;
    onHighlightComplete?: () => void;
    onQuoteConsumed?: () => void;
    onReplyConsumed?: () => void;
  } = $props();

  const connection = useConnection();
  const members = $derived(getRoomMembers());
  const currentUser = $derived(serverRegistry.getStore(getActiveServer()).currentUser);

  const store = new MessagesStore(connection(), () => currentUser.user?.id ?? null);
  onDestroy(() => store.dispose());

  $effect(() =>
    onRoomMessageMutated((detail) => {
      if (detail.roomId !== roomId) return;
      if (detail.reason === 'message-deleted') {
        store.applyLocalMessageDeletion(detail.eventId);
        return;
      }
      const anchorEventId = store.refreshAnchorForMessageMutation(detail.eventId);
      if (!anchorEventId) return;
      void store.refreshCurrentWindow(anchorEventId);
    })
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
    const currentUserId = currentUser.user?.id ?? null;
    const afterTime = unreadAfterTime.getTime();
    const beforeTime = unreadBeforeTime?.getTime() ?? Infinity;
    for (const event of threadEvents) {
      if (currentUserId && event.actorId === currentUserId) continue;
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
  let consumedQuoteId = 0;
  let consumedReplyId = 0;
  let composerApi = $state<MessageComposerApi | null>(null);

  // Thread-scoped jump state so "in reply to" clicks scroll within the thread.
  const jumpState = composerContext.jumpState;
  jumpState.setJumpHandler(async (eventId: string) => {
    jumpState.scrollToEventId = eventId;
  });

  let canPost = $derived(canPostInThread);

  // Reload thread events when the thread prop changes. Silent reconnect +
  // tab-resume catch-ups are owned by the server event bus.
  $effect(() => {
    store.setThread(roomId, threadRootEventId);
  });

  // Jump to a specific message when highlightEventId prop is set
  $effect(() => {
    if (!highlightEventId || store.isInitialLoading) return;
    jumpState.jumpToMessage(highlightEventId);
  });

  $effect(() => {
    const quote = pendingQuote;
    const api = composerApi;
    if (!quote || !api || quote.id === consumedQuoteId) return;

    consumedQuoteId = quote.id;
    composerContext.quoteInsertionState.requestInsertQuote(quote.text);
    onQuoteConsumed?.();
  });

  $effect(() => {
    const reply = pendingReply;
    const api = composerApi;
    if (
      !reply ||
      reply.threadRootEventId !== threadRootEventId ||
      !api ||
      reply.id === consumedReplyId
    ) {
      return;
    }

    consumedReplyId = reply.id;
    replyState.startReply(reply.eventId, reply.actorDisplayName, reply.excerpt);
    api.focus();
    onReplyConsumed?.();
  });

  // Subscribe to server events: clear typing indicator on a thread reply,
  // forward to the store, and mark the thread as read (with explicit event
  // ID) for replies arriving from other users while the user is present.
  useEvent((serverEvent) => {
    const eventData = serverEvent.event;
    if (!eventData) return;

    if (
      isMessagePostedEvent(eventData) &&
      eventData.roomId === roomId &&
      eventData.threadRootEventId === threadRootEventId
    ) {
      const actorId = serverEvent.actorId;
      if (actorId) {
        typingIndicator.removeTypingUser(actorId);
      }

      if (currentUser.user && actorId !== currentUser.user.id && appState.isPresent) {
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
          if (isMessagePostedEvent(rootEvent?.event)) {
            isFollowingThread = rootEvent.event.viewerIsFollowingThread ?? false;
          }
        }
      }
    }
  });

  async function toggleThreadFollow() {
    const wasFollowing = isFollowingThread;
    isFollowingThread = !wasFollowing;

    try {
      const conn = connection();
      const api = createThreadAPI({
        serverId: conn.serverId ?? getActiveServer(),
        baseUrl: conn.connectBaseUrl,
        bearerToken: conn.bearerToken
      });
      if (wasFollowing) {
        await api.unfollowThread({ roomId, threadRootEventId });
      } else {
        await api.followThread({ roomId, threadRootEventId });
      }
    } catch {
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
    const currentRoomId = roomId;
    void notificationStore.dismissThreadNotifications(threadId).then((counts) => {
      const dismissedForRoom = counts.byRoom[currentRoomId] ?? 0;
      if (dismissedForRoom > 0) {
        stores.rooms.decrementUnreadNotification(currentRoomId, dismissedForRoom);
        void stores.rooms.refreshNotificationCounts();
      }
    });
  });

  async function markThreadAsRead(currentThreadId: string, upToEventId?: string) {
    try {
      const conn = connection();
      return await createReadStateAPI({
        serverId: conn.serverId ?? getActiveServer(),
        baseUrl: conn.connectBaseUrl,
        bearerToken: conn.bearerToken
      }).markThreadAsRead({ roomId, threadRootEventId: currentThreadId, upToEventId });
    } catch (err) {
      console.error('Failed to mark thread as read:', err);
      return null;
    }
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
    title={m['room.thread.title']({ room: roomName })}
    onBack={onClose}
    backLabel={m['room.thread.back_to_room']()}
  >
    {#snippet actions()}
      <HeaderIconButton
        icon={isFollowingThread ? 'uil--bell' : 'uil--bell-slash'}
        label={isFollowingThread ? m['room.thread.unfollow']() : m['room.thread.follow']()}
        tone={isFollowingThread ? 'active' : 'default'}
        onclick={toggleThreadFollow}
      />
      <HeaderIconButton icon="uil--times" label={m['room.thread.close']()} onclick={onClose} />
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
    showStartMarker={false}
    onLoadMore={() => store.loadMore()}
    filterThreadReplies={false}
    {updateCounter}
    enableLastEditableFinder={true}
    isLoading={store.isInitialLoading}
    emptyMessage={m['room.thread.not_found']()}
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
    placeholder={m['room.thread.reply_placeholder']()}
    {canPost}
    {canAttach}
    showAlsoSendToChannel={canEchoMessage}
    onEscape={onClose}
    onReady={(api: MessageComposerApi) => {
      composerApi = api;
      api.focus();
    }}
    onTyping={() => typingIndicator?.sendTypingIndicator()}
    onMessageSent={(event) => {
      typingIndicator?.resetDebounce();
      if (event) {
        store.ingestEvent(event);
        void markThreadAsRead(threadRootEventId, event.id);
      } else {
        void store.refreshCurrentWindow(null);
      }
    }}
  />
</div>
