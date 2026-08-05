<script lang="ts">
  import { fly } from 'svelte/transition';
  import { createReadStateAPI, type MarkThreadAsReadResult } from '$lib/api-client/readState';
  import { useProjectionEvent, createTypingIndicator, useUnreadMarker } from '$lib/hooks';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { isMessagePostedEvent } from '$lib/render/timelineEvents';
  import { m } from '$lib/i18n/messages';
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
  import EventList from './EventList.svelte';
  import type { PendingThreadReplyRequest } from './threadOpenOptions';
  import { ThreadFollowState } from './threadFollowState.svelte';

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

  const serverScope = useServerScope();
  const connection = () => serverScope.connection;
  const activeServerId = $derived(serverScope.serverId);
  const members = $derived(getRoomMembers());
  const stores = $derived(serverScope.store);
  const currentUser = $derived(stores.currentUser);

  const store = $derived(stores.messagesForThread(roomId, threadRootEventId));

  // Thread timelines contain decrypted history and are useful only while a
  // pane renders them. Ref-count the stable selector so closing or switching
  // a pane releases its store instead of retaining every thread ever opened.
  $effect(() => {
    const mountedStores = stores;
    const mountedStore = store;
    const mountedRoomId = roomId;
    const mountedThreadRootEventId = threadRootEventId;
    mountedStores.retainMessagesForThread(mountedRoomId, mountedThreadRootEventId, mountedStore);
    return () =>
      mountedStores.releaseMessagesForThread(mountedRoomId, mountedThreadRootEventId, mountedStore);
  });

  $effect(() =>
    onRoomMessageMutated((detail) => {
      if (detail.serverId !== activeServerId || detail.roomId !== roomId) return;
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
      result.previousReadAt ? { afterTime: result.previousReadAt, beforeTime: markedAtMs } : null,
    getMarkerEvents: () => threadEvents,
    getMarkerSkipActorId: () => currentUser.user?.id ?? null
  });

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

  const threadFollow = new ThreadFollowState({
    getConnection: connection,
    getSnapshot: () => {
      const rootEvent = threadEvents.find((event) => event.id === threadRootEventId);
      const following =
        !store.isInitialLoading && isMessagePostedEvent(rootEvent?.event)
          ? (rootEvent.event.viewerIsFollowingThread ?? false)
          : null;
      return { roomId, threadRootEventId, following };
    }
  });

  async function markThreadAsRead(
    currentThreadId: string,
    upToEventId?: string
  ): Promise<MarkThreadAsReadResult | null> {
    try {
      return await connection()
        .getAPI(createReadStateAPI)
        .markThreadAsRead({ roomId, threadRootEventId: currentThreadId, upToEventId });
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
    title={m('room.thread.title', { room: roomName })}
    onBack={onClose}
    backLabel={m('room.thread.back_to_room')}
  >
    {#snippet actions()}
      <HeaderIconButton
        icon={threadFollow.following ? 'icon-[uil--bell]' : 'icon-[uil--bell-slash]'}
        label={threadFollow.following ? m('room.thread.unfollow') : m('room.thread.follow')}
        tone={threadFollow.following ? 'active' : 'default'}
        onclick={() => void threadFollow.toggle()}
        disabled={threadFollow.pending}
      />
      <HeaderIconButton icon="icon-[uil--times]" label={m('room.thread.close')} onclick={onClose} />
    {/snippet}
  </PaneHeader>

  <EventList
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
    emptyMessage={m('room.thread.not_found')}
    unreadAfterEventId={unread.unreadMarkerEventId}
    onReachedBottom={() => unread.clearUnreadMarker()}
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
    placeholder={m('room.thread.reply_placeholder')}
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
