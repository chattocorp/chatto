<script lang="ts">
  import { tick, untrack } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import { fade } from 'svelte/transition';
  import { Virtualizer, type VirtualizerHandle } from 'virtua/svelte';
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import { isMessagePostedEvent, type TimelineEventView } from '$lib/render/timelineEvents';
  import type { MessagesStore, RoomMember } from '$lib/state/room';
  import { getComposerContext, getRoomPermissions } from '$lib/state/room';
  import RoomEvent from './RoomEvent.svelte';
  import SystemEventGroup from './SystemEventGroup.svelte';
  import DaySeparator from '$lib/components/DaySeparator.svelte';
  import UnreadSeparator from './UnreadSeparator.svelte';
  import TypingIndicator from './TypingIndicator.svelte';
  import { computeEventMetadata } from './messageGrouping';
  import { buildVirtualItems, type VirtualItem } from './virtualItems';
  import { findLastEditableMessage } from './lastEditableMessage';
  import { ScrollFader } from '$lib/ui';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { INITIAL_ROOM_MESSAGE_BACKFILL_TARGET } from '$lib/state/room/messages/queries';
  import { formatDayLabel, timeFormatSettingsFor } from '$lib/utils/formatTime';
  import { useTabResumeCallback } from '$lib/hooks/useTabResumeCallback.svelte';
  import type { OpenThreadHandler, ThreadOpenOptions } from './threadOpenOptions';
  import { convergeAtBottom } from './bottomScrollConvergence';
  import { visibleTombstoneEvents, visibleUnreadMarkerEventId } from './tombstoneVisibility';
  import { TimelineViewportController } from './TimelineViewportController.svelte';
  import { RoomThreadingMode } from '$lib/roomThreading';

  let {
    roomId,
    permalinkThreadRootEventId = null,
    messageStore,
    events,
    // Scroll behavior
    alwaysScrollToBottom = false,
    showNewMessagesIndicator = true,
    // Pagination
    enablePagination = false,
    isLoadingMore = false,
    hasReachedStart = false,
    showStartMarker = true,
    onLoadMore,
    // Event updates
    updateCounter = 0,
    // Threading - only root messages can open threads
    onOpenThread,
    onOpenCall,
    onOpenProfile,
    // Filtering - whether to filter out thread replies (false for thread pane)
    filterThreadReplies = true,
    // Up-arrow-to-edit
    enableLastEditableFinder = false,
    // Loading states
    isLoading = false,
    emptyMessage = m('room.message.empty'),
    // Event ID of the first unread message (for showing the unread separator)
    unreadAfterEventId = null,
    // Typing indicator
    typingUserIds = [],
    typingMembers = [],
    // Jump to message
    scrollToEventId = null,
    onScrollToEventComplete,
    isJumpedMode = false,
    isLoadingNewer = false,
    hasReachedEnd = false,
    onLoadNewer,
    onJumpToPresent,
    onReachedPresent,
    onReachedBottom,
    pendingHighlightId = null,
    threadingMode = RoomThreadingMode.ENABLED
  }: {
    roomId: string;
    permalinkThreadRootEventId?: string | null;
    messageStore: MessagesStore;
    events: TimelineEventView[];
    // Scroll behavior
    alwaysScrollToBottom?: boolean;
    showNewMessagesIndicator?: boolean;
    // Pagination
    enablePagination?: boolean;
    isLoadingMore?: boolean;
    hasReachedStart?: boolean;
    showStartMarker?: boolean;
    onLoadMore?: () => Promise<void>;
    // Event updates
    updateCounter?: number;
    // Threading
    onOpenThread?: OpenThreadHandler;
    onOpenCall?: () => void;
    onOpenProfile?: (userId: string) => void;
    // Filtering
    filterThreadReplies?: boolean;
    // Up-arrow-to-edit
    enableLastEditableFinder?: boolean;
    // Loading states
    isLoading?: boolean;
    emptyMessage?: string;
    // Event ID of the first unread message (for showing the unread separator)
    unreadAfterEventId?: string | null;
    // Typing indicator
    typingUserIds?: string[];
    typingMembers?: RoomMember[];
    // Jump to message
    scrollToEventId?: string | null;
    onScrollToEventComplete?: (landed: boolean) => void;
    isJumpedMode?: boolean;
    isLoadingNewer?: boolean;
    hasReachedEnd?: boolean;
    onLoadNewer?: () => Promise<void>;
    onJumpToPresent?: () => Promise<boolean>;
    onReachedPresent?: () => void;
    onReachedBottom?: () => void;
    // Suppress auto-scroll while a highlight is pending (used by ThreadPane)
    pendingHighlightId?: string | null;
    threadingMode?: RoomThreadingMode;
  } = $props();

  const viewport = new TimelineViewportController();
  const expandedSystemEventIds = new SvelteSet<string>();

  function isSystemGroupExpanded(groupEvents: TimelineEventView[]): boolean {
    return groupEvents.some((event) => expandedSystemEventIds.has(event.id));
  }

  function setSystemGroupExpanded(groupEvents: TimelineEventView[], expanded: boolean): void {
    for (const event of groupEvents) {
      if (expanded) {
        expandedSystemEventIds.add(event.id);
      } else {
        expandedSystemEventIds.delete(event.id);
      }
    }
  }

  // Get composer context (scrollState may be null - ThreadPane doesn't provide it)
  const composerContext = getComposerContext();
  const scrollState = composerContext.scrollState;
  const serverScope = useServerScope();
  const stores = $derived(serverScope.store);
  const currentUser = $derived(stores.currentUser);
  const serverInfo = $derived(stores.serverInfo);
  const userSettings = $derived(timeFormatSettingsFor(currentUser.user?.settings));
  const activeLocale = $derived(getLocale());
  const firstVisibleDate = $derived(
    viewport.firstVisibleAt
      ? formatDayLabel(viewport.firstVisibleAt, userSettings, activeLocale)
      : null
  );
  const reloadsTimelineOnReturn = $derived(isJumpedMode && !!onJumpToPresent);

  // First apply structural timeline filtering. Context-free tombstones are a
  // separate stage so row removal cannot be mistaken for a newly arrived message.
  let timelineEvents = $derived(
    events.filter((e) => {
      if (!isMessagePostedEvent(e.event)) return true;

      const msg = e.event;

      // Filter out thread replies when enabled (main room view)
      // In thread pane, filterThreadReplies=false to show all messages
      if (filterThreadReplies && msg?.threadRootEventId != null) return false;

      return true;
    })
  );
  let filteredEvents = $derived(visibleTombstoneEvents(timelineEvents));
  let messageEventCount = $derived(
    filteredEvents.filter((event) => isMessagePostedEvent(event.event)).length
  );

  // Apply message grouping and day separators
  let eventsWithMeta = $derived(computeEventMetadata(filteredEvents, userSettings, activeLocale));

  // If the marker points at a hidden tombstone, move it to the next visible
  // event instead of silently dropping the unread boundary.
  let effectiveUnreadAfterEventId = $derived.by(() => {
    return visibleUnreadMarkerEventId(timelineEvents, filteredEvents, unreadAfterEventId ?? null);
  });

  // Build flat array for the virtualizer (events + interleaved separators)
  let virtualItems = $derived(
    buildVirtualItems(eventsWithMeta, effectiveUnreadAfterEventId, hasReachedStart, showStartMarker)
  );

  // Register finder for up-arrow-to-edit (computed on-demand, not reactively)
  const lastEditableMessageCtx = composerContext.lastEditableMessage;
  const roomPermissions = $derived(getRoomPermissions());

  $effect(() => {
    if (!enableLastEditableFinder) return;

    lastEditableMessageCtx?.setFinder(() => {
      return findLastEditableMessage({
        events: filteredEvents,
        currentUserId: currentUser.user?.id,
        roomPermissions,
        messageEditWindowSeconds: serverInfo.messageEditWindowSeconds,
        nowMs: Date.now()
      });
    });
  });

  // Feed projection/component inputs into the controller in one ordered
  // transition. DOM and virtualizer state are deliberately excluded.
  $effect(() => {
    const currentRoomId = roomId;
    const jumped = isJumpedMode;
    const newestId = timelineEvents.at(-1)?.id ?? null;
    const newestOptions = {
      showNewMessagesIndicator,
      alwaysScrollToBottom
    };
    untrack(() => {
      if (viewport.enterRoom(currentRoomId)) expandedSystemEventIds.clear();
      viewport.observeJumpedMode(jumped);
      // Comparing the newest ID rather than the count keeps prepended
      // pagination rows from looking like newly arrived messages.
      viewport.observeNewestEvent(newestId, newestOptions);
    });
  });

  // Watch for scroll-to-bottom requests from MessageComposer (after posting a message).
  // Posting is explicit user intent to see the bottom, so it releases the
  // controller's short virtualizer-correction lock.
  // Uses scrollContainer.scrollTop instead of scrollToIndex because the user may have
  // been scrolled up — unmeasured items at the bottom have only estimated heights,
  // causing scrollToIndex to undershoot.
  $effect(() => {
    if (!scrollState || alwaysScrollToBottom) return;
    const counter = scrollState.scrollRequestCounter;
    if (counter > 0) {
      viewport.requestComposerBottom();
      tick().then(() => {
        if (scrollContainer && viewport.shouldScrollToBottom) {
          void requestBottomScroll();
        }
      });
    }
  });

  // Scroll to a specific event by ID (for jump-to-message)
  $effect(() => {
    let cancelled = false;
    const targetId = scrollToEventId;
    if (!targetId || !virtualizerHandle || virtualItems.length === 0) return;

    // Disable auto-scroll so it doesn't race with the jump scroll.
    viewport.beginJump();

    void tick().then(async () => {
      // A replaced virtual window can take several frames to index, measure,
      // and mount its target. The initial attempt plus 60 retries preserves the
      // existing bounded wait without a separate callback state machine.
      for (let attempt = 0; attempt <= 60 && !cancelled; attempt++) {
        await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
        if (cancelled) return;

        const targetIndex = virtualItems.findIndex(
          (item) => item.type === 'event' && item.event.id === targetId
        );
        if (targetIndex !== -1) safeScrollToIndex(targetIndex, { align: 'center' });

        // Scope lookup to this EventList so the thread pane cannot highlight
        // the matching event in the main room timeline.
        const target = (scrollContainer ?? document).querySelector(eventSelector(targetId));
        if (!(target instanceof HTMLElement)) continue;

        target.classList.add('highlight-flash');
        target.addEventListener('animationend', () => target.classList.remove('highlight-flash'), {
          once: true
        });

        await new Promise((resolve) => setTimeout(resolve, 200));
        if (cancelled) return;
        const distance = distanceFromBottom();
        if (distance === null) return;
        viewport.settleJump(distance);
        onScrollToEventComplete?.(true);
        return;
      }

      if (!cancelled) onScrollToEventComplete?.(false);
    });

    return () => {
      cancelled = true;
    };
  });

  // Scroll container and virtualizer handle
  let scrollContainer = $state<HTMLDivElement>();
  let virtualizerHandle = $state<VirtualizerHandle>();
  let scrollFader = $state<{ refresh: () => void }>();

  // Safely call scrollToIndex on the virtualizer. After a {#key roomId} transition,
  // the new Virtualizer's bind:this fires immediately but its onMount → tick() →
  // assignRef hasn't run yet, so the scroller has no DOM reference. Calling
  // scrollToIndex in that window causes "Cannot read properties of null
  // (reading 'ownerDocument')". This wrapper catches that transient error.
  function safeScrollToIndex(...args: Parameters<VirtualizerHandle['scrollToIndex']>) {
    try {
      virtualizerHandle?.scrollToIndex(...args);
    } catch {
      // Virtualizer not yet initialized — scroll will self-correct on next render
    }
  }

  function requestBottomScroll(): Promise<boolean> | undefined {
    if (!scrollContainer || !virtualizerHandle || virtualItems.length === 0) return undefined;

    const token = viewport.beginBottomScroll(roomId);
    return convergeAtBottom({
      continueWhile: () =>
        viewport.canContinueBottomScroll(token, roomId, isJumpedMode, alwaysScrollToBottom) &&
        Boolean(scrollContainer && virtualizerHandle),
      waitForFrame: async () => {
        await tick();
        await new Promise((resolve) => requestAnimationFrame(resolve));
      },
      scroll: () => {
        if (!scrollContainer) return;
        safeScrollToIndex(virtualItems.length - 1, { align: 'end' });
        scrollContainer.scrollTop = scrollContainer.scrollHeight;
        scrollFader?.refresh();
      },
      measure: () => {
        if (!virtualizerHandle) return null;
        return {
          distanceFromBottom:
            virtualizerHandle.getScrollSize() -
            virtualizerHandle.getScrollOffset() -
            virtualizerHandle.getViewportSize(),
          scrollSize: virtualizerHandle.getScrollSize(),
          viewportSize: virtualizerHandle.getViewportSize()
        };
      }
    }).then((converged) => {
      viewport.completeBottomScroll(token);
      return converged;
    });
  }

  // Register the scroll container with ScrollState so sibling components
  // (MessageComposer, TypingIndicator) can synchronously scroll without waiting
  // for ResizeObserver callbacks.
  $effect(() => {
    if (scrollState && scrollContainer) {
      scrollState.setContainer(scrollContainer);
      return () => scrollState.setContainer(null);
    }
  });

  // Keep ScrollState's shouldScroll flag in sync with our local state
  $effect(() => {
    scrollState?.setShouldScroll(alwaysScrollToBottom || viewport.shouldScrollToBottom);
  });

  // Auto-scroll to bottom when new events arrive or existing events update.
  // shouldScrollToBottom is read via untrack() so toggling it doesn't re-trigger
  // this effect — it only gates whether we scroll when new data arrives.
  // Suppressed in jumped mode — we don't want to auto-scroll when viewing history.
  // Suppressed when pendingHighlightId is set — a highlight scroll is pending and
  // auto-scroll would race with it, scrolling to bottom before the highlight can fire.
  $effect(() => {
    void updateCounter;

    if (isJumpedMode) return;
    if (pendingHighlightId) return;

    if (virtualItems.length > 0 && virtualizerHandle) {
      const shouldScroll = untrack(() => alwaysScrollToBottom || viewport.shouldScrollToBottom);
      if (shouldScroll) {
        void requestBottomScroll();
      }
    }
  });

  // Scroll to bottom when clicking the new messages indicator
  function scrollToBottom() {
    viewport.followBottom();
    onReachedBottom?.();
    void requestBottomScroll();
  }

  async function handleJumpToPresentClick() {
    // The replacement latest window must perform a fresh initial-style bottom
    // scroll. Virtua otherwise preserves the historical window's offset when
    // the keyed data is replaced and can leave the user stranded mid-window.
    viewport.prepareJumpToPresent();
    onReachedBottom?.();
    const requestedRoomId = roomId;
    const intentRevision = viewport.captureIntentRevision();
    if (!(await onJumpToPresent?.())) return;
    await tick();
    if (roomId !== requestedRoomId || !viewport.hasIntentRevision(intentRevision)) return;
    void requestBottomScroll();
  }

  // Timestamp of the most recent user-driven scroll signal (wheel or touchmove).
  // The scroll-up branch in handleVirtuaScroll only fires when this is recent,
  // so virtua's internal scroll adjustments (re-measurement, $fixScrollJump),
  // composer-resize-driven scrollTop writes, and browser scroll clamping during
  // layout shifts never get misread as the user scrolling up.
  function markUserScrollIntent() {
    viewport.markUserScrollIntent();
  }

  function markKeyboardScrollIntent(event: KeyboardEvent) {
    const target = event.target;
    if (
      target instanceof HTMLInputElement ||
      target instanceof HTMLTextAreaElement ||
      (target instanceof HTMLElement && target.isContentEditable)
    ) {
      return;
    }

    if (['ArrowUp', 'ArrowDown', 'PageUp', 'PageDown', 'Home', 'End', ' '].includes(event.key)) {
      markUserScrollIntent();
    }
  }

  function distanceFromBottom(): number | null {
    if (!virtualizerHandle) return null;
    return (
      virtualizerHandle.getScrollSize() -
      virtualizerHandle.getScrollOffset() -
      virtualizerHandle.getViewportSize()
    );
  }

  function eventSelector(eventId: string): string {
    return `[data-event-id="${CSS.escape(eventId)}"]`;
  }

  // Re-evaluate "are we at the bottom?" when the tab regains visibility — the
  // browser may have throttled virtua's measurements or our auto-scroll effect
  // while hidden, leaving shouldScrollToBottom=true even though the scroll has
  // drifted off the bottom (which would suppress the Jump to Present button).
  useTabResumeCallback(() => {
    if (!virtualizerHandle) return;
    const dist =
      virtualizerHandle.getScrollSize() -
      virtualizerHandle.getScrollOffset() -
      virtualizerHandle.getViewportSize();
    viewport.reconcileAfterTabResume(dist, alwaysScrollToBottom);
  });

  let forwardLoadInFlight = false;
  let underfilledBackfillInFlight = false;

  function exitJumpedModeAtPresent(bottomDistance: number): boolean {
    if (!isJumpedMode || !hasReachedEnd || bottomDistance >= 50 || !onReachedPresent) return false;

    viewport.followBottom();
    onReachedBottom?.();
    console.debug('[room-refresh] reached present after forward pagination', {
      roomId,
      bottomDistance,
      itemCount: virtualItems.length
    });
    onReachedPresent();
    return true;
  }

  async function loadNewerAndMaybeExitAtPresent(): Promise<void> {
    if (!onLoadNewer || forwardLoadInFlight) return;

    forwardLoadInFlight = true;
    try {
      await onLoadNewer();
      await tick();
      await new Promise((resolve) => requestAnimationFrame(resolve));

      const nextBottomDistance = distanceFromBottom();
      if (nextBottomDistance !== null) {
        exitJumpedModeAtPresent(nextBottomDistance);
      }
    } finally {
      forwardLoadInFlight = false;
    }
  }

  async function loadOlderIfTimelineNeedsBackfill(): Promise<void> {
    if (
      !enablePagination ||
      !onLoadMore ||
      isLoading ||
      isLoadingMore ||
      hasReachedStart ||
      isJumpedMode ||
      underfilledBackfillInFlight
    ) {
      return;
    }

    underfilledBackfillInFlight = true;
    try {
      // A fetched page can consist entirely of context-free tombstones. There is no
      // Virtualizer in that state, but pagination still needs to walk backward
      // until it finds visible history or reaches the beginning.
      if (timelineEvents.length > 0 && filteredEvents.length === 0) {
        await onLoadMore();
        return;
      }

      await tick();
      await new Promise((resolve) => requestAnimationFrame(resolve));
      if (
        !virtualizerHandle ||
        isLoading ||
        isLoadingMore ||
        hasReachedStart ||
        isJumpedMode ||
        virtualItems.length === 0
      ) {
        return;
      }

      const scrollSize = virtualizerHandle.getScrollSize();
      const viewportSize = virtualizerHandle.getViewportSize();
      const lacksInitialRoomMessages =
        filterThreadReplies &&
        timelineEvents.length > 0 &&
        messageEventCount < INITIAL_ROOM_MESSAGE_BACKFILL_TARGET;
      if (scrollSize <= viewportSize + 50 || lacksInitialRoomMessages) {
        await onLoadMore();
      }
    } finally {
      underfilledBackfillInFlight = false;
    }
  }

  $effect(() => {
    void virtualItems.length;
    void timelineEvents.length;
    void filteredEvents.length;
    void messageEventCount;
    void enablePagination;
    void isLoading;
    void isLoadingMore;
    void hasReachedStart;
    void isJumpedMode;
    void virtualizerHandle;

    void loadOlderIfTimelineNeedsBackfill();
  });

  // Handle scroll events from virtua to detect user intent and trigger pagination.
  // virtua's shift=true handles scroll restoration during pagination automatically,
  // eliminating the need for manual scrollHeight capture/restore and overflow-anchor toggling.
  function handleVirtuaScroll(offset: number) {
    if (!virtualizerHandle) return;

    const scrollSize = virtualizerHandle.getScrollSize();
    const viewportSize = virtualizerHandle.getViewportSize();
    let firstVisibleAt: string | null = null;
    const idx = virtualizerHandle.findItemIndex(offset);
    for (let i = idx; i < virtualItems.length; i++) {
      const item = virtualItems[i];
      if (item.type === 'event') {
        firstVisibleAt = item.event.createdAt;
        break;
      }
    }
    const scrollResult = viewport.observeScroll({
      offset,
      scrollSize,
      viewportSize,
      firstVisibleAt,
      alwaysScrollToBottom,
      now: Date.now()
    });
    const { distanceFromBottom } = scrollResult;
    if (scrollResult.reachedBottom) onReachedBottom?.();

    // Trigger pagination when scrolled near the top.
    // Guard: only when content actually overflows the viewport (avoids firing in short rooms).
    if (
      enablePagination &&
      onLoadMore &&
      offset < viewportSize * 3 &&
      scrollSize > viewportSize + 50 &&
      !isLoadingMore &&
      !hasReachedStart
    ) {
      // No manual scroll restoration needed — virtua's shift=true handles it
      onLoadMore();
    }

    // Forward pagination when near bottom in jumped mode
    if (
      isJumpedMode &&
      onLoadNewer &&
      distanceFromBottom < viewportSize * 3 &&
      !isLoadingNewer &&
      !forwardLoadInFlight &&
      !hasReachedEnd
    ) {
      void loadNewerAndMaybeExitAtPresent();
    }

    // Exit jumped mode when user has scrolled to bottom and all content is loaded
    if (hasReachedEnd && exitJumpedModeAtPresent(distanceFromBottom)) {
      return;
    }
  }

  // Determine if a message can open a thread
  // Root messages open their own thread; echoes open the original thread
  function getOpenThreadHandler(event: TimelineEventView) {
    if (!onOpenThread) return undefined;

    const eventData = event.event;
    if (!eventData) return undefined;
    if (isMessagePostedEvent(eventData)) {
      // Echoes open the original thread
      if (eventData.echoOfEventId != null) {
        return (_threadRootEventId: string, options: ThreadOpenOptions = {}) =>
          onOpenThread(eventData.echoFromThreadRootEventId!, options);
      }
      // Thread replies don't open threads from the main channel
      if (eventData.threadRootEventId !== null) return undefined;
      // Root messages open their own thread
      return (_threadRootEventId?: string, options: ThreadOpenOptions = {}) =>
        onOpenThread(event.id, options);
    }

    return undefined;
  }
</script>

<svelte:window onkeydown={markKeyboardScrollIntent} />

<div class="relative flex min-h-0 min-w-0 flex-1 flex-col pb-2">
  <ScrollFader
    top
    bottom
    bind:this={scrollFader}
    bind:scrollEl={scrollContainer}
    scrollClass="overscroll-y-contain"
    data-testid="messages-container"
    onwheel={markUserScrollIntent}
    ontouchmove={markUserScrollIntent}
    onpointerdown={markUserScrollIntent}
  >
    <div class="mt-auto">
      {#if !isLoading && virtualItems.length === 0}
        <div class="flex flex-1 items-center justify-center">
          <div class="py-4 text-sm text-muted">{emptyMessage}</div>
        </div>
      {:else if !isLoading}
        <Virtualizer
          bind:this={virtualizerHandle}
          data={virtualItems}
          getKey={(item, index) => item?.key ?? `__ix_${index}`}
          scrollRef={scrollContainer}
          shift={isLoadingMore}
          itemSize={60}
          onscroll={handleVirtuaScroll}
        >
          {#snippet children(item: VirtualItem)}
            {#if !item}
              <!-- Stale virtualizer index during data transition, skip -->
            {:else if item.type === 'start-marker'}
              <div class="pt-10 pb-2 text-center text-sm text-muted">
                {m('room.timeline.beginning')}
              </div>
            {:else if item.type === 'day-separator'}
              <DaySeparator label={item.label} />
            {:else if item.type === 'unread-separator'}
              <UnreadSeparator />
            {:else if item.type === 'system-group'}
              <!-- Same guard pattern as the event branch below — virtua may re-invoke
                   the snippet with a stale item reference during data transitions
                   (e.g. switching rooms or servers). -->
              {@const groupEvents = item?.events}
              {@const groupKind = item?.kind}
              {#if groupEvents && groupKind && groupEvents.length > 0}
                <SystemEventGroup
                  events={groupEvents}
                  kind={groupKind}
                  expanded={isSystemGroupExpanded(groupEvents)}
                  onExpandedChange={(expanded) => setSystemGroupExpanded(groupEvents, expanded)}
                />
              {/if}
            {:else}
              <!--
                Use {@const} with optional chaining to snapshot the event and guard
                against the virtualizer's item getter returning undefined during data
                transitions. Svelte 5's reactive prop getters can re-evaluate before
                the outer {#if !item} branch switches, so we need this inner guard.
              -->
              {@const eventData = item?.event}
              {#if eventData}
                <RoomEvent
                  event={eventData}
                  compact={!item.isFirstInGroup}
                  {roomId}
                  {permalinkThreadRootEventId}
                  {messageStore}
                  onOpenThread={getOpenThreadHandler(eventData)}
                  activeCallId={stores.activeCallRooms.getCallId(roomId)}
                  {onOpenCall}
                  {onOpenProfile}
                  {threadingMode}
                />
              {/if}
            {/if}
          {/snippet}
        </Virtualizer>
      {/if}
    </div>
  </ScrollFader>

  <TypingIndicator {typingUserIds} members={typingMembers} />

  {#if !viewport.shouldScrollToBottom && (reloadsTimelineOnReturn || !alwaysScrollToBottom)}
    <button
      transition:fade={{ duration: 150 }}
      onclick={reloadsTimelineOnReturn ? handleJumpToPresentClick : scrollToBottom}
      data-testid="jump-to-present"
      class="absolute bottom-4 left-1/2 z-40 -translate-x-1/2 cursor-pointer menu whitespace-nowrap"
    >
      <div class="flex items-center gap-2 menu-section px-3 py-1">
        {#if firstVisibleDate}
          <span class="text-muted">{firstVisibleDate}</span>
          <span class="text-muted/40">|</span>
        {/if}
        <span>
          {!reloadsTimelineOnReturn && viewport.hasNewMessages
            ? m('room.unread_separator')
            : m('room.jump_to_present')}
        </span>
        <span class="iconify icon-[uil--arrow-down]"></span>
      </div>
    </button>
  {/if}
</div>
