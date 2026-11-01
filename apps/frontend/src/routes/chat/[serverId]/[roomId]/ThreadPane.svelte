<script lang="ts">
  import { untrack } from 'svelte';
  import { serverUi } from '$lib/state/server/serverUi';
  import { fly } from 'svelte/transition';
  import { createPanelReveal } from '$lib/dom/panelReveal';
  import { fromInlineEndOffset } from '$lib/i18n/direction';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { isMessagePostedEvent } from '@chatto/client/timeline/timelineEvents';
  import { m } from '$lib/i18n/messages';
  import type { ThreadPanePresentation } from '$lib/state/userPreferences.svelte';
  import { threadPaneWidth } from '$lib/state/threadPaneWidth.svelte';
  import { THREAD_PANE_MAX_WIDTH, THREAD_PANE_MIN_WIDTH } from '$lib/storage/threadPaneWidth';
  import { PaneHeader, HeaderIconButton, FloatingPopover } from '$lib/ui';
  import { expoOutTransition } from '$lib/ui/motion';
  import ResizeHandle from '$lib/components/ResizeHandle.svelte';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import UserMenu from '$lib/components/users/UserMenu.svelte';
  import { UserMenuState } from '$lib/components/users/UserMenuState.svelte';
  import { startDMWith } from '$lib/dm/startDM';
  import { getLiveDisplayName, getLiveLogin } from '$lib/state/userProfiles.svelte';
  import ConversationPane from './ConversationPane.svelte';
  import type { PendingComposerInput, PendingHighlight } from './roomNavigationState.svelte';
  import { ThreadFollowState } from './threadFollowState.svelte';
  import { RoomThreadingMode } from '@chatto/client/util/roomThreading';

  const paneId = $props.id();

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
    independentSlowMode = false,
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
    independentSlowMode?: boolean;
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
  const stores = serverScope.store;
  const revealThread = createPanelReveal(() => ({
    scope: roomId,
    panel: threadRootEventId
  }));

  $effect(() => {
    if (!stores.currentUser.user) return;
    return serverUi(stores).readViews.register({ roomId, threadRootId: threadRootEventId });
  });

  const store = $derived(stores.rooms.thread(roomId, threadRootEventId));

  // Restore on entry, including after hydration mounts the pane. Departure
  // cancels late jumps without fetching replies for a hidden thread.
  $effect(() => {
    const mountedStore = store;
    untrack(() => void mountedStore.restoreLatestWindow());
    return () => mountedStore.cancelPendingHistoricalJump();
  });

  const threadMessage = $derived(
    store.threadEvents.find((entry) => isMessagePostedEvent(entry.event))?.event
  );
  const rootMessage = $derived(
    store.threadEvents.find((entry) => entry.id === threadRootEventId)?.event
  );
  const participants = $derived(
    isMessagePostedEvent(rootMessage) ? (rootMessage.threadParticipants ?? []) : []
  );
  // The summary includes replies outside the loaded message window.
  const participantCount = $derived(
    isMessagePostedEvent(rootMessage)
      ? Math.max(rootMessage.threadParticipantCount ?? 0, participants.length)
      : 0
  );
  let paneWidth = $state(0);
  // Use the actual pane width: split panes can stay narrow on a wide screen.
  const participantLimit = $derived(
    paneWidth >= 640 ? 12 : paneWidth >= 480 ? 8 : paneWidth >= 360 ? 5 : 3
  );
  // Profile changes must update labels and open hints without refreshing the thread summary.
  const visibleParticipants = $derived(
    participants.slice(0, participantLimit).map((user) =>
      user.deleted
        ? user
        : {
            ...user,
            displayName: getLiveDisplayName(user.id, user.displayName ?? ''),
            login: getLiveLogin(user.id, user.login ?? '')
          }
    )
  );
  let participantHint = $state<{
    userId: string;
    roomId: string;
    threadRootEventId: string;
    anchor: DOMRect;
  } | null>(null);
  const hintedParticipant = $derived(
    participantHint?.roomId === roomId && participantHint.threadRootEventId === threadRootEventId
      ? visibleParticipants.find((user) => user.id === participantHint?.userId)
      : undefined
  );
  function showParticipantHint(event: MouseEvent | FocusEvent, userId: string) {
    participantHint = {
      userId,
      roomId,
      threadRootEventId,
      anchor: (event.currentTarget as HTMLElement).getBoundingClientRect()
    };
  }
  function hideParticipantHint() {
    participantHint = null;
  }
  // Resolve menu data from the current thread so removed participants cannot stay visible.
  const participantMenu = new UserMenuState<{
    roomId: string;
    threadRootEventId: string;
    userId: string;
  }>();
  const menuParticipant = $derived(
    participantMenu.target?.roomId === roomId &&
      participantMenu.target.threadRootEventId === threadRootEventId
      ? participants.find((user) => user.id === participantMenu.target?.userId && !user.deleted)
      : undefined
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
  id={paneId}
  class={[
    'flex min-h-0 min-w-0 flex-col overflow-hidden border-s border-border bg-background',
    presentation === 'split'
      ? 'relative w-[var(--thread-pane-width)] shrink-0'
      : 'absolute inset-y-0 end-0 z-10 w-full inline-end-overlay-shadow lg:w-[90%]'
  ]}
  data-testid="thread-pane"
  bind:clientWidth={paneWidth}
  {@attach revealThread}
  style:--thread-pane-width={`${threadPaneWidth.value}px`}
  transition:fly|global={{
    x: fromInlineEndOffset(presentation === 'overlay' ? 48 : 24),
    ...expoOutTransition(180)
  }}
>
  {#if presentation === 'split'}
    <ResizeHandle
      controls={paneId}
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
    data-testid="thread-conversation-pane"
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
      slowModeNextPostAt: independentSlowMode
        ? (store.threadSlowModeNextPostAt ?? null)
        : slowModeNextPostAt,
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
          {#if participantCount > 0}
            <div
              class="flex items-center"
              role="group"
              aria-label={m('room.thread.participants', { count: participantCount })}
              data-testid="thread-header-participants"
            >
              {#each visibleParticipants as participant, index (participant.id)}
                {@const menuTarget = { roomId, threadRootEventId, userId: participant.id }}
                {@const name = participant.deleted
                  ? m('common.deleted_user')
                  : participant.displayName || participant.login || m('common.unknown_user')}
                <button
                  type="button"
                  class={[
                    'relative pane-header-icon-button rounded-full not-disabled:hover:bg-transparent focus-visible:z-10 not-disabled:active:bg-transparent',
                    index > 0 && '-ms-4'
                  ]}
                  aria-label={name}
                  aria-haspopup="dialog"
                  aria-expanded={menuParticipant?.id === participant.id}
                  disabled={participant.deleted}
                  onmouseenter={(event) => showParticipantHint(event, participant.id)}
                  onmouseleave={hideParticipantHint}
                  onfocus={(event) => showParticipantHint(event, participant.id)}
                  onblur={hideParticipantHint}
                  onpointerdown={hideParticipantHint}
                  onclick={(event) => participantMenu.open(menuTarget, event)}
                  {@attach participantMenu.trigger(() => (participant.deleted ? null : menuTarget))}
                >
                  <UserAvatar user={participant} size="sm" class="ring-2 ring-background" />
                </button>
              {/each}
              {#if participantCount > visibleParticipants.length}
                <span class="px-1 text-xs text-muted" aria-hidden="true"
                  >+{participantCount - visibleParticipants.length}</span
                >
              {/if}
            </div>
          {/if}
          <HeaderIconButton
            icon={threadFollow.following ? 'icon-[uil--bell]' : 'icon-[uil--bell-slash]'}
            label={threadFollow.following ? m('room.thread.unfollow') : m('room.thread.follow')}
            onclick={() => void threadFollow.toggle()}
            disabled={threadFollow.pending}
          />
          <div class="hidden lg:contents">
            <HeaderIconButton
              icon="icon-[uil--times]"
              label={m('room.thread.close')}
              onclick={onClose}
            />
          </div>
        {/snippet}
      </PaneHeader>
    {/snippet}
  </ConversationPane>
</div>

<svelte:window
  onresize={hideParticipantHint}
  onkeydowncapture={(event) => {
    if (event.key === 'Escape' && participantHint && !menuParticipant) {
      hideParticipantHint();
      event.preventDefault();
      event.stopPropagation();
    }
  }}
/>

{#if participantHint && hintedParticipant && !menuParticipant}
  <FloatingPopover
    anchor={participantHint.anchor}
    role="tooltip"
    class="pointer-events-none max-w-xs menu"
    onclose={hideParticipantHint}
  >
    <div class="menu-section px-3 py-2 text-xs whitespace-normal">
      {hintedParticipant.deleted
        ? m('common.deleted_user')
        : hintedParticipant.displayName || hintedParticipant.login || m('common.unknown_user')}
    </div>
  </FloatingPopover>
{/if}

{#if menuParticipant}
  <UserMenu
    state={participantMenu}
    user={menuParticipant}
    canSendMessage={stores.permissions.canStartDMs}
    viewerSettings={stores.currentUser.user?.settings}
    onSendMessage={() => startDMWith(serverScope.serverId, menuParticipant!.id)}
    {onOpenProfile}
  />
{/if}
