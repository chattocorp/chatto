<!--
@component

One conversation timeline with its composer: the room timeline, or one thread.
The room view and the thread pane both render it.

The pane owns the per-conversation state: the composer context, the typing
indicator, unread handling, realtime read updates, jump-to-message, pending
highlights and composer input, and file drops. The owner supplies the header
and the posting policy.

`threadRootEventId` selects the timeline kind when the pane mounts, so an
owner must keep it null or non-null for the pane's lifetime. The room and
thread IDs can change while the pane stays mounted.
-->
<script lang="ts" module>
  import type { MessageComposerProps } from '$lib/components/composer/messageComposerState.svelte';
  import { queryCaches } from '$lib/query/cacheRegistry';
  import { serverUi } from '$lib/state/server/serverUi';

  /** Composer options that the owner decides. The pane supplies the rest. */
  export type ConversationComposerOptions = Omit<
    MessageComposerProps,
    | 'roomId'
    | 'inThread'
    | 'canPost'
    | 'canAttach'
    | 'onReady'
    | 'onTyping'
    | 'onMessageSent'
    | 'mentionPriorityUserIds'
  >;
</script>

<script lang="ts">
  import { onDestroy, untrack, type Snippet } from 'svelte';
  import type { ClassValue, HTMLAttributes } from 'svelte/elements';
  import { createReadStateAPI, type MarkThreadAsReadResult } from '@chatto/client/api/readState';
  import { dropZone } from '$lib/dom/dropZone.svelte';
  import DropZoneOverlay from '$lib/dom/DropZoneOverlay.svelte';
  import MessageComposer, {
    type MessageComposerApi
  } from '$lib/components/composer/MessageComposer.svelte';
  import {
    createTypingIndicator,
    useProjectionEvent,
    useRoomUnread,
    useUnreadMarker
  } from '$lib/hooks';
  import { m } from '$lib/i18n/messages';
  import { RoomThreadingMode } from '@chatto/client/util/roomThreading';
  import { appState } from '$lib/state/globals.svelte';
  import { createComposerContext, getRoomMembers, type MessagesStore } from '$lib/state/room';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { EmptyState } from '$lib/ui';
  import { toast } from '$lib/ui/toast';
  import EventList from './EventList.svelte';
  import HighlightJump from './HighlightJump.svelte';
  import type { PendingComposerInput, PendingHighlight } from './roomNavigationState.svelte';
  import type { OpenThreadHandler } from './threadOpenOptions';
  import { threadParticipantIds } from './threadParticipants';
  import { isMessagePostedEvent } from '@chatto/client/timeline/timelineEvents';
  import { ReadThroughTracker, type TimelineReadPosition } from './readThroughTracker';
  import { clearTimelineViewport } from '$lib/state/room/timelineViewport';

  let {
    roomId,
    threadRootEventId = null,
    messageStore,
    threadingMode = RoomThreadingMode.ENABLED,
    canReadMessages = true,
    hasLimitedMessageAccess = false,
    canPost,
    canAttach,
    composer = {},
    highlight = null,
    onHighlightComplete,
    composerInput = null,
    onComposerInputConsumed,
    onOpenThread,
    onOpenCall,
    onOpenProfile,
    header,
    class: className,
    ...rest
  }: Omit<HTMLAttributes<HTMLDivElement>, 'class'> & {
    roomId: string;
    /** The thread timeline to show, or null for the room timeline. */
    threadRootEventId?: string | null;
    messageStore: MessagesStore;
    threadingMode?: RoomThreadingMode;
    /** False shows the read-denied state instead of the timeline. */
    canReadMessages?: boolean;
    /** Display the server's interaction-only read scope without changing timeline loading. */
    hasLimitedMessageAccess?: boolean;
    canPost: boolean;
    canAttach: boolean;
    composer?: ConversationComposerOptions;
    /** A message to jump to and highlight once. The owner clears it on completion. */
    highlight?: PendingHighlight | null;
    onHighlightComplete?: (highlight: PendingHighlight) => void;
    /** A quote or reply to put into the composer once it is ready. */
    composerInput?: PendingComposerInput | null;
    onComposerInputConsumed?: (input: PendingComposerInput) => void;
    onOpenThread?: OpenThreadHandler;
    onOpenCall?: () => void;
    onOpenProfile?: (userId: string) => void;
    /** Rendered above the timeline, inside the file drop area. */
    header?: Snippet;
    class?: ClassValue;
  } = $props();

  const serverScope = useServerScope();
  const stores = serverScope.store;
  const members = $derived(getRoomMembers());
  const isThread = untrack(() => threadRootEventId !== null);

  // The composer and the timeline below share this pane's context. A thread
  // pane shadows the room's context, so its jumps, replies, and scroll
  // requests stay in the thread.
  const composerContext = createComposerContext();
  const { editState, replyState, jumpState, quoteInsertionState } = composerContext;

  const events = $derived(isThread ? messageStore.threadEvents : messageStore.rootEvents);
  // The unread separator marks the first message from another user after the
  // read cursor. System rows, such as the viewer's own join, never start it.
  const markerEvents = $derived(events.filter((event) => isMessagePostedEvent(event.event)));
  /** Other users in this thread, ranked first in @mention autocomplete. */
  const mentionPriorityUserIds = $derived(
    isThread && threadRootEventId
      ? threadParticipantIds(events, threadRootEventId, stores.viewerId)
      : undefined
  );
  const targetKey = $derived(threadRootEventId ? `${roomId}:${threadRootEventId}` : roomId);

  const typingIndicator = createTypingIndicator(() => ({
    roomId,
    threadRootEventId,
    currentUserId: stores.viewerId
  }));

  // The conversation is read up to the newest message that the viewer has
  // seen, not up to its latest message. After a jump to an older message,
  // newer activity and its notifications stay unread until the viewer
  // scrolls to them.
  let readPosition = $state.raw<{ key: string; position: TimelineReadPosition } | null>(null);
  const currentReadPosition = $derived(
    readPosition?.key === targetKey ? readPosition.position : null
  );
  // A jump to a message is about to start or is running. The entry read waits
  // for it, so that it does not read past the target.
  const highlightPending = $derived(
    highlight !== null || serverUi(stores).pendingHighlights.has(roomId, threadRootEventId)
  );
  const atLatest = $derived(!highlightPending && (currentReadPosition?.latest ?? true));

  const readThrough = new ReadThroughTracker((upToEventId) => {
    // A read scheduled for a previous conversation names an event of that
    // conversation. Drop it.
    if (readThroughKey !== targetKey) return;
    void unread.markAsRead(threadRootEventId ?? roomId, upToEventId);
  });
  let readThroughKey = untrack(() => targetKey);
  onDestroy(() => readThrough.dispose());

  /** The tracker for the current conversation, reset when the conversation changes. */
  function currentReadThrough(): ReadThroughTracker {
    if (readThroughKey !== targetKey) {
      readThroughKey = targetKey;
      readThrough.reset();
    }
    return readThrough;
  }

  function lifecycleUpToEventId(): string | undefined | null {
    if (highlightPending) return null;
    const position = currentReadPosition;
    return position && !position.latest ? position.eventId : undefined;
  }

  function handleLifecycleRead(upToEventId: string | undefined) {
    const position = currentReadPosition;
    currentReadThrough().noteRead(
      upToEventId !== undefined && position?.eventId === upToEventId ? position.createdAtMs : null
    );
  }

  function handleReadPosition(position: TimelineReadPosition) {
    readPosition = { key: targetKey, position };
    if (appState.isPresent && !highlightPending) currentReadThrough().observe(position);
  }

  async function markThreadAsRead(
    targetThreadRootEventId: string,
    upToEventId: string | undefined,
    signal: AbortSignal
  ): Promise<MarkThreadAsReadResult> {
    const readStores = stores;
    const readRoomId = roomId;
    const connection = serverScope.connection;
    const dataGeneration = connection.dataGeneration;
    const result = await connection
      .getAPI(createReadStateAPI)
      .markThreadAsRead(
        { roomId: readRoomId, threadRootEventId: targetThreadRootEventId, upToEventId },
        { signal }
      );
    if (!signal.aborted && dataGeneration === connection.dataGeneration) {
      readStores.reconcileThreadRead(readRoomId, targetThreadRootEventId);
      queryCaches.followedThreads?.refresh(readStores.serverId);
    }
    return result;
  }

  const unread = isThread
    ? useUnreadMarker(() => threadRootEventId!, {
        markAsRead: markThreadAsRead,
        // The read starts when the viewer is verified, as in the room timeline.
        canMarkAsRead: () => stores.isAuthenticated,
        markerWindowFromReadResult: (result, markedAtMs) =>
          result.previousLastReadAt
            ? { afterTime: result.previousLastReadAt, beforeTime: markedAtMs }
            : null,
        getMarkerEvents: () => markerEvents,
        getMarkerSkipActorId: () => stores.viewerId,
        onMarkAsReadError: (error) => console.error('Failed to mark thread as read:', error),
        getLifecycleUpToEventId: lifecycleUpToEventId,
        onLifecycleRead: handleLifecycleRead
      })
    : useRoomUnread(() => ({
        roomId,
        events: markerEvents,
        canReadMessages,
        getLifecycleUpToEventId: lifecycleUpToEventId,
        onLifecycleRead: handleLifecycleRead
      }));

  // A reply or a jump belongs to the conversation that started it. The server
  // store loads each timeline when it creates it, so a target change needs no
  // explicit load here.
  let previousTargetKey = untrack(() => targetKey);
  $effect(() => {
    const key = targetKey;
    if (key === previousTargetKey) return;
    previousTargetKey = key;
    replyState.cancelReply();
    jumpState.reset();
  });

  // Register before any child can request a jump. The store loads the window
  // around a target that the loaded window does not contain.
  jumpState.setJumpHandler(async (eventId: string) => {
    if (!canReadMessages) return false;
    // A quote jump replaces an unfinished notification jump without reading it.
    // Release its pending target so EventList can scroll to the new message.
    const pending = highlight;
    if (pending && pending.eventId !== eventId) onHighlightComplete?.(pending);
    // An explicit target takes precedence over the position saved for recovery.
    clearTimelineViewport(messageStore);
    return jumpState.show(messageStore, eventId);
  });

  // Projection v2 folds retractions and crypto-erasure into the authoritative
  // message row, so an edit of a deleted message ends here.
  $effect(() => {
    const editingEventId = editState.eventId;
    if (!editingEventId) return;
    const payload = events.find((event) => event.id === editingEventId)?.event;
    if (payload && 'deletedAt' in payload && payload.deletedAt) editState.cancelEdit();
  });

  let composerApi = $state<MessageComposerApi | null>(null);

  // Apply a quote or reply that another pane requested before this composer existed.
  $effect(() => {
    const input = composerInput;
    const api = composerApi;
    if (!input || !api) return;

    if (input.quote) quoteInsertionState.requestInsertQuote(input.quote);
    if (input.reply) {
      const reply = input.reply;
      replyState.startReply(
        reply.eventId,
        reply.actorDisplayName,
        reply.excerpt,
        reply.actorIdentity
      );
      api.focus();
    }
    onComposerInputConsumed?.(input);
  });

  // Clear typing and mark the conversation read for messages from other users
  // that arrive while the viewer is present at the latest message. While the
  // viewer is away, show the unread separator above the first such message at
  // once. A message that arrives while the viewer reads older history stays
  // unread until the viewer scrolls to it.
  useProjectionEvent((projectionEvent) => {
    const semantic = projectionEvent.event?.event;
    if (semantic?.case !== 'messagePosted' || semantic.value.roomId !== roomId) return;
    if ((semantic.value.threadRootEventId || null) !== threadRootEventId) return;

    const actorId = projectionEvent.event?.actorId;
    if (actorId) typingIndicator.removeTypingUser(actorId);
    const accountId = stores.accountId;
    if (!accountId || actorId === accountId) return;
    const eventId = projectionEvent.event?.id ?? '';
    if (appState.isPresent && atLatest) {
      void unread.markAsRead(threadRootEventId ?? roomId, eventId);
      return;
    }
    const createdAt = projectionEvent.event?.createdAt;
    if (createdAt) currentReadThrough().noteUnreadArrival(createdAt.toDate().getTime());
    if (!appState.isPresent && eventId) unread.markArrivalWhileAway(eventId);
  });

  let isDraggingFiles = $state(false);
  const conversationDropZone = $derived(
    canPost && canAttach
      ? dropZone({
          onDrop: (files) => composerApi?.addFiles(files),
          onDragStateChange: (dragging) => (isDraggingFiles = dragging)
        })
      : undefined
  );
</script>

{#if highlight && !stores.realtimeSync.isRecoveringSnapshot}
  {#key highlight}
    <HighlightJump
      request={highlight}
      jump={(eventId) => jumpState.jumpToMessage(eventId)}
      onFailed={(request) => {
        if (highlight !== request || !serverScope.isCurrent()) return;
        toast.error(m('room.jump_failed'));
        onHighlightComplete?.(request);
      }}
    />
  {/key}
{/if}

<div
  class={['relative flex min-h-0 min-w-0 flex-1 flex-col', className]}
  {...rest}
  {@attach conversationDropZone}
>
  <DropZoneOverlay visible={isDraggingFiles} />
  {@render header?.()}

  {#if canReadMessages}
    <EventList
      {roomId}
      permalinkThreadRootEventId={threadRootEventId}
      {messageStore}
      {events}
      showStartMarker={!isThread && !hasLimitedMessageAccess}
      filterThreadReplies={!isThread}
      emptyMessage={m(
        isThread
          ? 'room.thread.not_found'
          : hasLimitedMessageAccess
            ? 'room.timeline.limited_empty'
            : 'room.message.empty'
      )}
      {onOpenThread}
      {onOpenCall}
      {onOpenProfile}
      unreadAfterEventId={unread.unreadMarkerEventId}
      onReachedBottom={() => unread.clearUnreadMarker()}
      onJumpToPresent={() => {
        if (highlight) onHighlightComplete?.(highlight);
      }}
      onReadPosition={handleReadPosition}
      typingUserIds={typingIndicator.userIds}
      typingMembers={members}
      pendingHighlightId={highlight?.eventId ?? null}
      highlightRequest={highlight}
      onScrollToEventComplete={(landed, eventId, request) => {
        // Completion names the request that started the scroll. A later click
        // on the same message must not be cleared by this callback.
        if (jumpState.scrollToEventId !== eventId || highlight !== request) return;
        jumpState.scrollToEventId = null;
        if (request) {
          if (landed && request.notificationId) {
            void stores.notifications.markOccurrenceRead(request.notificationId).catch((error) => {
              console.error('Failed to mark displayed notification read:', error);
            });
          }
          onHighlightComplete?.(request);
        }
        if (!landed) toast.error(m('room.jump_failed'));
      }}
      {threadingMode}
    />
  {:else}
    <EmptyState icon="icon-[uil--eye-slash]">
      {m('room.timeline.read_denied')}
    </EmptyState>
  {/if}

  <MessageComposer
    {...composer}
    {roomId}
    inThread={threadRootEventId ?? undefined}
    {mentionPriorityUserIds}
    {canPost}
    {canAttach}
    onReady={(api) => (composerApi = api)}
    onTyping={canPost ? () => typingIndicator.sendTypingIndicator() : undefined}
    onMessageSent={(event) => {
      typingIndicator.resetDebounce();
      if (!event) {
        void messageStore.refreshCurrentWindow(null);
        return;
      }
      messageStore.ingestEvent(event);
      // A thread moves its read cursor to the viewer's own reply.
      if (threadRootEventId) void unread.markAsRead(threadRootEventId, event.id);
    }}
    onThreadMessageSent={composer.onThreadMessageSent
      ? (rootEventId, event) => {
          typingIndicator.resetDebounce();
          composer.onThreadMessageSent?.(rootEventId, event);
        }
      : undefined}
  />
</div>
