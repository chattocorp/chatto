<!--
@component

One conversation timeline with its composer: the room timeline, or one thread.
The room view and the thread pane both render it.

The pane owns the per-conversation state: the composer context, the typing
indicator, unread handling, realtime read updates, timeline activation,
jump-to-message, pending highlights and composer input, and file drops. The
owner supplies the header and the posting policy.

`threadRootEventId` selects the timeline kind when the pane mounts, so an
owner must keep it null or non-null for the pane's lifetime. The room and
thread IDs can change while the pane stays mounted.
-->
<script lang="ts" module>
  import type { MessageComposerProps } from '$lib/components/composer/messageComposerState.svelte';

  /** Composer options that the owner decides. The pane supplies the rest. */
  export type ConversationComposerOptions = Omit<
    MessageComposerProps,
    'roomId' | 'inThread' | 'canPost' | 'canAttach' | 'onReady' | 'onTyping' | 'onMessageSent'
  >;
</script>

<script lang="ts">
  import { tick, untrack, type Snippet } from 'svelte';
  import type { ClassValue, HTMLAttributes } from 'svelte/elements';
  import { createReadStateAPI, type MarkThreadAsReadResult } from '$lib/api-client/readState';
  import { dropZone } from '$lib/attachments/dropZone.svelte';
  import DropZoneOverlay from '$lib/attachments/DropZoneOverlay.svelte';
  import MessageComposer, {
    type MessageComposerApi
  } from '$lib/components/composer/MessageComposer.svelte';
  import {
    createTypingIndicator,
    useProjectionEvent,
    useRoomUnread,
    useUnreadMarker
  } from '$lib/hooks';
  import { useTimelineMutations } from '$lib/hooks/useTimelineMutations.svelte';
  import { m } from '$lib/i18n/messages';
  import { RoomThreadingMode } from '$lib/roomThreading';
  import { appState } from '$lib/state/globals.svelte';
  import { createComposerContext, getRoomMembers, type MessagesStore } from '$lib/state/room';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import { toast } from '$lib/ui/toast';
  import EventList from './EventList.svelte';
  import type { PendingComposerInput, PendingHighlight } from './roomNavigationState.svelte';
  import type { OpenThreadHandler } from './threadOpenOptions';

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
  const stores = $derived(serverScope.store);
  const members = $derived(getRoomMembers());
  const isThread = untrack(() => threadRootEventId !== null);

  // The composer and the timeline below share this pane's context. A thread
  // pane shadows the room's context, so its jumps, replies, and scroll
  // requests stay in the thread.
  const composerContext = createComposerContext();
  const { editState, replyState, jumpState, quoteInsertionState } = composerContext;

  const events = $derived(isThread ? messageStore.threadEvents : messageStore.rootEvents);
  const targetKey = $derived(threadRootEventId ? `${roomId}:${threadRootEventId}` : roomId);

  useTimelineMutations(() => ({
    serverId: serverScope.serverId,
    roomId,
    timeline: messageStore
  }));

  const typingIndicator = createTypingIndicator(() => ({
    roomId,
    threadRootEventId,
    currentUserId: stores.viewerId
  }));

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
    }
    return result;
  }

  const unread = isThread
    ? useUnreadMarker(() => threadRootEventId!, {
        markAsRead: markThreadAsRead,
        // A saved view can show before the server accepts commands. The read
        // starts when the viewer is verified, as in the room timeline.
        canMarkAsRead: () => stores.isAuthenticated,
        markerWindowFromReadResult: (result, markedAtMs) =>
          result.previousLastReadAt
            ? { afterTime: result.previousLastReadAt, beforeTime: markedAtMs }
            : null,
        getMarkerEvents: () => events,
        getMarkerSkipActorId: () => stores.currentUser.user?.id ?? null,
        onMarkAsReadError: (error) => console.error('Failed to mark thread as read:', error)
      })
    : useRoomUnread(() => ({ roomId, events, canReadMessages }));

  /** The read-state target: the thread root in a thread, otherwise the room. */
  const readTargetId = $derived(threadRootEventId ?? roomId);

  // A reply or a jump belongs to the conversation that started it.
  let replyTargetKey = untrack(() => targetKey);
  $effect(() => {
    const key = targetKey;
    if (key === replyTargetKey) return;
    replyTargetKey = key;
    replyState.cancelReply();
  });

  // Load the timeline only when the viewer can read it; the server rejects
  // other reads. Reconnect convergence belongs to the resumable server
  // projection and does not trigger a parallel read here.
  $effect(() => {
    if (!canReadMessages) return;
    const store = messageStore;
    const targetRoomId = roomId;
    const targetThreadRootEventId = threadRootEventId;
    untrack(() => {
      if (targetThreadRootEventId) store.setThread(targetRoomId, targetThreadRootEventId);
      else store.setRoom(targetRoomId);
      jumpState.reset();
    });
  });

  // Register before any child can request a jump. The room store loads a
  // window around the target. The store cannot do that for a thread, so a
  // thread only scrolls; the highlight flow below loads a missing target first.
  jumpState.setJumpHandler(async (eventId: string) => {
    if (!canReadMessages) return false;
    if (!isThread) return messageStore.jumpToMessage(eventId, jumpState);
    jumpState.scrollToEventId = eventId;
    return true;
  });

  // Projection v2 folds retractions and crypto-erasure into the authoritative
  // message row, so an edit of a deleted message ends here.
  $effect(() => {
    const editingEventId = editState.eventId;
    if (!editingEventId) return;
    const payload = events.find((event) => event.id === editingEventId)?.event;
    if (payload && 'deletedAt' in payload && payload.deletedAt) editState.cancelEdit();
  });

  // Jump to each highlight request once. A thread first loads a target outside
  // its latest page, because its jump handler only scrolls.
  let handledHighlight: PendingHighlight | null = null;
  let highlightRequest = 0;
  $effect(() => {
    const target = highlight;
    if (!target) {
      handledHighlight = null;
      highlightRequest += 1;
      return;
    }
    if (isThread && messageStore.isInitialLoading) return;
    if (handledHighlight === target) return;
    handledHighlight = target;
    const request = ++highlightRequest;
    const current = () =>
      request === highlightRequest && highlight === target && serverScope.isCurrent();

    void (async () => {
      await tick();
      if (isThread && !events.some((event) => event.id === target.eventId)) {
        await messageStore.refreshCurrentWindow(target.eventId);
      }
      if (!current()) return;
      const jumped = await jumpState.jumpToMessage(target.eventId);
      if (!current()) return;
      if (!jumped) {
        toast.error(m('room.jump_failed'));
        onHighlightComplete?.(target);
        return;
      }
      if (target.notificationId) {
        void stores.notifications.markOccurrenceRead(target.notificationId).catch((error) => {
          console.error('Failed to mark displayed notification read:', error);
        });
      }
    })();
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
  // that arrive while the viewer is present.
  useProjectionEvent((projectionEvent) => {
    const semantic = projectionEvent.event?.event;
    if (semantic?.case !== 'messagePosted' || semantic.value.roomId !== roomId) return;
    if ((semantic.value.threadRootEventId || null) !== threadRootEventId) return;

    const actorId = projectionEvent.event?.actorId;
    if (actorId) typingIndicator.removeTypingUser(actorId);
    const viewer = stores.currentUser.user;
    if (viewer && actorId !== viewer.id && appState.isPresent) {
      void unread.markAsRead(readTargetId, projectionEvent.event?.id ?? '');
    }
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
      typingUserIds={typingIndicator.userIds}
      typingMembers={members}
      pendingHighlightId={highlight?.eventId ?? null}
      onScrollToEventComplete={(landed) => {
        jumpState.scrollToEventId = null;
        if (highlight) onHighlightComplete?.(highlight);
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
