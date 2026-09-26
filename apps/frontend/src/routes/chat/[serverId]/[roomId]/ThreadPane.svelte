<script lang="ts">
  import { fly } from 'svelte/transition';
  import { fromInlineEndOffset } from '$lib/i18n/direction';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { isMessagePostedEvent } from '$lib/render/timelineEvents';
  import { m } from '$lib/i18n/messages';
  import type { ThreadPanePresentation } from '$lib/state/userPreferences.svelte';
  import { threadPaneWidth } from '$lib/state/threadPaneWidth.svelte';
  import { THREAD_PANE_MAX_WIDTH, THREAD_PANE_MIN_WIDTH } from '$lib/storage/threadPaneWidth';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import HeaderIconButton from '$lib/ui/HeaderIconButton.svelte';
  import { expoOutTransition } from '$lib/ui/motion';
  import ResizeHandle from '$lib/components/ResizeHandle.svelte';
  import ConversationPane from './ConversationPane.svelte';
  import type { PendingComposerInput, PendingHighlight } from './roomNavigationState.svelte';
  import { ThreadFollowState } from './threadFollowState.svelte';
  import { RoomThreadingMode } from '$lib/roomThreading';

  let {
    roomId,
    roomName,
    isDirectMessage = false,
    threadRootEventId,
    onClose,
    canPostInThread = true,
    canAttach = true,
    canEchoMessage = false,
    slowModeSeconds = 0,
    slowModeNextPostAt = null,
    slowModeBypassed = false,
    highlight = null,
    composerInput = null,
    presentation = 'overlay',
    threadingMode = RoomThreadingMode.ENABLED,
    onOpenProfile,
    onHighlightComplete,
    onComposerInputConsumed
  }: {
    roomId: string;
    roomName: string;
    isDirectMessage?: boolean;
    threadRootEventId: string;
    onClose: () => void;
    canPostInThread?: boolean;
    canAttach?: boolean;
    canEchoMessage?: boolean;
    slowModeSeconds?: number;
    slowModeNextPostAt?: string | null;
    slowModeBypassed?: boolean;
    highlight?: PendingHighlight | null;
    composerInput?: PendingComposerInput | null;
    /** Resolved layout after the preferred mode and available width are applied. */
    presentation?: ThreadPanePresentation;
    threadingMode?: RoomThreadingMode;
    onOpenProfile?: (userId: string) => void;
    onHighlightComplete?: (highlight: PendingHighlight) => void;
    onComposerInputConsumed?: (input: PendingComposerInput) => void;
  } = $props();

  const serverScope = useServerScope();
  const stores = $derived(serverScope.store);

  $effect(() => {
    if (!stores.currentUser.user) return;
    return stores.readViews.register({ roomId, threadRootId: threadRootEventId });
  });

  const store = $derived(stores.messagesForThread(roomId, threadRootEventId));

  // Track mounted consumers while the server retains the canonical timeline
  // for replay and persistence. Release viewport state when the last pane closes.
  $effect(() => {
    const mountedStores = stores;
    const mountedStore = store;
    const mountedRoomId = roomId;
    const mountedThreadRootEventId = threadRootEventId;
    mountedStores.retainMessagesForThread(mountedRoomId, mountedThreadRootEventId, mountedStore);
    return () =>
      mountedStores.releaseMessagesForThread(mountedRoomId, mountedThreadRootEventId, mountedStore);
  });

  const threadMessage = $derived(
    store.threadEvents.find((entry) => isMessagePostedEvent(entry.event))?.event
  );
  let canPost = $derived(
    threadingMode !== RoomThreadingMode.DISABLED &&
      (isMessagePostedEvent(threadMessage)
        ? (threadMessage.canReplyInThread ?? canPostInThread)
        : canPostInThread)
  );

  let threadTitle = $derived(
    isDirectMessage ? roomName : m('room.thread.title', { room: roomName })
  );

  const threadFollow = new ThreadFollowState({
    getConnection: () => serverScope.connection,
    getSnapshot: () => {
      const rootEvent = store.threadEvents.find((event) => event.id === threadRootEventId);
      const following =
        !store.isInitialLoading && isMessagePostedEvent(rootEvent?.event)
          ? (rootEvent.event.viewerIsFollowingThread ?? false)
          : null;
      return { roomId, threadRootEventId, following };
    }
  });
</script>

<div
  class={[
    'flex min-h-0 min-w-0 flex-col overflow-hidden border-s border-border bg-background',
    presentation === 'split'
      ? 'relative w-[var(--thread-pane-width)] shrink-0'
      : 'absolute inset-y-0 end-0 z-10 w-full inline-end-overlay-shadow lg:w-[90%]'
  ]}
  data-testid="thread-pane"
  style:--thread-pane-width={`${threadPaneWidth.value}px`}
  transition:fly|global={{ x: fromInlineEndOffset(300), ...expoOutTransition() }}
>
  {#if presentation === 'split'}
    <ResizeHandle
      width={threadPaneWidth.value}
      min={THREAD_PANE_MIN_WIDTH}
      max={THREAD_PANE_MAX_WIDTH}
      onResize={(width) => threadPaneWidth.set(width)}
      onReset={() => threadPaneWidth.reset()}
      edge="start"
      label={`${m('ui.resize_handle.resize')}: ${threadTitle}`}
    />
  {/if}
  <ConversationPane
    {roomId}
    {threadRootEventId}
    messageStore={store}
    {threadingMode}
    {canPost}
    {canAttach}
    composer={{
      echoToConversation: isDirectMessage,
      placeholder: m('room.thread.reply_placeholder'),
      slowModeSeconds,
      slowModeNextPostAt,
      slowModeBypassed,
      showAlsoSendToChannel: canEchoMessage,
      onEscape: onClose
    }}
    {highlight}
    {onHighlightComplete}
    {composerInput}
    {onComposerInputConsumed}
    {onOpenProfile}
  >
    {#snippet header()}
      <PaneHeader title={threadTitle} onBack={onClose} backLabel={m('room.thread.back_to_room')}>
        {#snippet actions()}
          <HeaderIconButton
            icon={threadFollow.following ? 'icon-[uil--bell]' : 'icon-[uil--bell-slash]'}
            label={threadFollow.following ? m('room.thread.unfollow') : m('room.thread.follow')}
            onclick={() => void threadFollow.toggle()}
            disabled={threadFollow.pending}
          />
          <HeaderIconButton
            icon="icon-[uil--times]"
            label={m('room.thread.close')}
            onclick={onClose}
          />
        {/snippet}
      </PaneHeader>
    {/snippet}
  </ConversationPane>
</div>
