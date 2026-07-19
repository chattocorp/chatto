<script lang="ts">
  import { fly } from 'svelte/transition';
  import { createReadStateAPI, type MarkThreadAsReadResult } from '$lib/api-client/readState';
  import { createThreadAPI } from '$lib/api-client/threads';
  import {
    useProjectionEvent,
    createTypingIndicator,
    useUnreadMarker
  } from '$lib/hooks';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { isMessagePostedEvent } from '$lib/render/eventKinds';
  import * as m from '$lib/i18n/messages';
  import { dropZone } from '$lib/attachments/dropZone.svelte';
  import DropZoneOverlay from '$lib/attachments/DropZoneOverlay.svelte';

  import { appState } from '$lib/state/globals.svelte';
  import {
    getRoomMembers,
    createComposerContext,
    type QuoteInsertionRequest
  } from '$lib/state/room';
  import { onRoomMessageMutated } from '$lib/state/room/messageMutationEvents';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import HeaderIconButton from '$lib/ui/HeaderIconButton.svelte';
  import MessageComposer, {
    type MessageComposerApi
  } from '$lib/components/composer/MessageComposer.svelte';
  import TimelineEventsPane from './TimelineEventsPane.svelte';
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

  const stores = serverRegistry.getStore(getActiveServer());
  const store = $derived(stores.messagesForThread(roomId, threadRootEventId));

  // Thread timelines contain decrypted history and are useful only while a
  // pane renders them. Ref-count the stable selector so closing or switching
  // a pane releases its store instead of retaining every thread ever opened.
  $effect(() => {
    const mountedStore = store;
    const mountedRoomId = roomId;
    const mountedThreadRootEventId = threadRootEventId;
    stores.retainMessagesForThread(mountedRoomId, mountedThreadRootEventId, mountedStore);
    return () =>
      stores.releaseMessagesForThread(mountedRoomId, mountedThreadRootEventId, mountedStore);
  });

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

  const unread = useUnreadMarker(() => threadRootEventId, {
    markAsRead: markThreadAsRead,
    markerWindowFromReadResult: (result, markedAtMs) =>
      result.previousReadAt ? { afterTime: result.previousReadAt, beforeTime: markedAtMs } : null
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
  let isDraggingFiles = $state(false);

  const threadDropZone = $derived(
    canPostInThread && canAttach
      ? dropZone({
          onDrop: (files) => composerApi?.addFiles(files),
          onDragStateChange: (dragging) => (isDraggingFiles = dragging),
          acceptedTypes: ['image/*', 'video/*', 'audio/*']
        })
      : undefined
  );

  // Thread-scoped jump state so "in reply to" clicks scroll within the thread.
  const jumpState = composerContext.jumpState;
  jumpState.setJumpHandler(async (eventId: string) => {
    jumpState.scrollToEventId = eventId;
    return true;
  });

  let canPost = $derived(canPostInThread);

  // Reload thread events when the thread prop changes. Silent reconnect +
  // tab-resume catch-ups are owned by the server event bus.
  $effect(() => {
    store.setThread(roomId, threadRootEventId);
  });

  // Load a permalink target outside the latest page before asking the
  // virtualized timeline to scroll to it.
  let handledHighlightKey: string | null = null;
  let highlightRequestId = 0;
  $effect(() => {
    const targetEventId = highlightEventId;
    const targetThreadRootEventId = threadRootEventId;
    if (!targetEventId) {
      handledHighlightKey = null;
      highlightRequestId += 1;
      return;
    }
    if (store.isInitialLoading) return;

    const highlightKey = `${targetThreadRootEventId}:${targetEventId}`;
    if (handledHighlightKey === highlightKey) return;
    handledHighlightKey = highlightKey;
    const requestId = ++highlightRequestId;

    void (async () => {
      if (!threadEvents.some((event) => event.id === targetEventId)) {
        await store.refreshCurrentWindow(targetEventId);
      }
      if (
        requestId !== highlightRequestId ||
        threadRootEventId !== targetThreadRootEventId ||
        highlightEventId !== targetEventId
      ) {
        return;
      }
      await jumpState.jumpToMessage(targetEventId);
    })();
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
  useProjectionEvent((projectionEvent) => {
    for (const operation of projectionEvent.operations) {
      if (operation.operation.case !== 'roomTimelineEventUpsert') continue;
      const update = operation.operation.value;
      if (update.roomId !== roomId || update.event?.event.case !== 'messagePosted') continue;
      if (update.event.event.value.message?.threadRootEventId !== threadRootEventId) continue;

      const actorId = projectionEvent.actorId;
      if (actorId) typingIndicator.removeTypingUser(actorId);
      if (currentUser.user && actorId !== currentUser.user.id && appState.isPresent) {
        void unread.markAsRead(threadRootEventId, projectionEvent.id);
      }
    }
  });

  // -- Thread follow state --
  // The lazy thread root and later projection upserts both carry the current
  // viewer follow state. Observe each new authoritative row version so a
  // follow change after the initial thread query updates the pane as well.
  let isFollowingThread = $state(false);
  let _observedFollowThread = '';
  let _observedFollowValue: boolean | undefined;
  let threadFollowRequestId = 0;
  let isThreadFollowPending = $state(false);

  function setAuthoritativeThreadFollowState(value: boolean) {
    threadFollowRequestId += 1;
    isThreadFollowPending = false;
    isFollowingThread = value;
  }

  $effect(() => {
    const threadId = threadRootEventId;
    if (threadId !== _observedFollowThread) {
      threadFollowRequestId += 1;
      isThreadFollowPending = false;
      isFollowingThread = false;
      _observedFollowThread = threadId;
      _observedFollowValue = undefined;
    }

    if (store.isInitialLoading || isThreadFollowPending) return;
    const rootEvent = threadEvents.find((event) => event.id === threadId);
    if (!isMessagePostedEvent(rootEvent?.event)) return;
    const nextFollowing = rootEvent.event.viewerIsFollowingThread ?? false;
    if (_observedFollowValue === nextFollowing) return;
    _observedFollowValue = nextFollowing;
    setAuthoritativeThreadFollowState(nextFollowing);
  });

  async function toggleThreadFollow() {
    if (isThreadFollowPending) return;

    const wasFollowing = isFollowingThread;
    const nextFollowing = !wasFollowing;
    const requestId = ++threadFollowRequestId;

    isThreadFollowPending = true;
    isFollowingThread = nextFollowing;

    try {
      const conn = connection();
      const api = createThreadAPI({
        serverId: conn.serverId ?? getActiveServer(),
        baseUrl: conn.connectBaseUrl,
        bearerToken: conn.bearerToken
      });
      const input = { roomId, threadRootEventId };
      const result = wasFollowing ? await api.unfollowThread(input) : await api.followThread(input);
      if (threadFollowRequestId !== requestId) return;
      setAuthoritativeThreadFollowState(result.following);
    } catch {
      if (threadFollowRequestId !== requestId) return;
      isThreadFollowPending = false;
      isFollowingThread = wasFollowing;
    }
  }

  async function markThreadAsRead(
    currentThreadId: string,
    upToEventId?: string
  ): Promise<MarkThreadAsReadResult | null> {
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
</script>

<div
  class="absolute inset-y-0 right-0 z-10 flex min-h-0 w-full min-w-0 flex-col overflow-hidden border-l border-border bg-background shadow-[-4px_0_12px_rgba(0,0,0,0.15)] sm:w-[90%]"
  data-testid="thread-pane"
  transition:fly={{ x: 300, duration: 200 }}
  {@attach threadDropZone}
>
  <DropZoneOverlay visible={isDraggingFiles} />
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
        disabled={isThreadFollowPending}
      />
      <HeaderIconButton icon="uil--times" label={m['room.thread.close']()} onclick={onClose} />
    {/snippet}
  </PaneHeader>

  <TimelineEventsPane
    {roomId}
    permalinkThreadRootEventId={threadRootEventId}
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
    unreadMarkerEventId={unread.unreadMarkerEventId}
    unreadMarkerWindow={unread.unreadMarkerWindow}
    unreadMarkerSkipActorId={currentUser.user?.id ?? null}
    onUnreadMarkerResolved={(eventId) => unread.setUnreadMarkerEventId(eventId)}
    onUnreadMarkerCleared={() => unread.clearUnreadMarker()}
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
        void unread.markAsRead(threadRootEventId, event.id);
      } else {
        void store.refreshCurrentWindow(null);
      }
    }}
  />
</div>
