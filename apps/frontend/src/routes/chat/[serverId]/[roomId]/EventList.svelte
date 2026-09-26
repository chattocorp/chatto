<script lang="ts">
  import { onDestroy, tick, untrack } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import { fade } from 'svelte/transition';
  import { Virtualizer, type VirtualizerHandle } from 'virtua/svelte';
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import { isMessagePostedEvent, type TimelineEventView } from '$lib/render/timelineEvents';
  import type { MessagesStore, RoomMember } from '$lib/state/room';
  import { getComposerContext, getRoomMembers, getRoomPermissions } from '$lib/state/room';
  import type { UserAvatarUserView } from '$lib/render/users';
  import RoomEvent from './RoomEvent.svelte';
  import MessageUserOverlays from './MessageUserOverlays.svelte';
  import { MessageUserInteractionState } from './messageUserInteractions.svelte';
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
    showStartMarker = true,
    // Threading - only root messages can open threads
    onOpenThread,
    onOpenCall,
    onOpenProfile,
    // Filtering - whether to filter out thread replies (false for thread pane)
    filterThreadReplies = true,
    emptyMessage = m('room.message.empty'),
    // Event ID of the first unread message (for showing the unread separator)
    unreadAfterEventId = null,
    // Typing indicator
    typingUserIds = [],
    typingMembers = [],
    onScrollToEventComplete,
    onReachedBottom,
    pendingHighlightId = null,
    threadingMode = RoomThreadingMode.ENABLED
  }: {
    roomId: string;
    permalinkThreadRootEventId?: string | null;
    /** Owns loading, pagination, and jump requests for this timeline. */
    messageStore: MessagesStore;
    events: TimelineEventView[];
    showStartMarker?: boolean;
    // Threading
    onOpenThread?: OpenThreadHandler;
    onOpenCall?: () => void;
    onOpenProfile?: (userId: string) => void;
    // Filtering
    filterThreadReplies?: boolean;
    emptyMessage?: string;
    // Event ID of the first unread message (for showing the unread separator).
    // The timeline lands on it once per entry unless the user moves first.
    unreadAfterEventId?: string | null;
    // Typing indicator
    typingUserIds?: string[];
    typingMembers?: RoomMember[];
    /** Reports whether a jump-to-message request found and highlighted its target. */
    onScrollToEventComplete?: (landed: boolean) => void;
    onReachedBottom?: () => void;
    // Suppress auto-scroll while a highlight is pending (used by ThreadPane)
    pendingHighlightId?: string | null;
    threadingMode?: RoomThreadingMode;
  } = $props();

  const viewport = new TimelineViewportController();
  let destroyed = false;
  onDestroy(() => {
    destroyed = true;
    viewport.cancelBottomScroll();
  });
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

  // The room and thread panes each provide their own composer context. Its jump
  // state and the message store together drive loading and jump-to-message.
  const composerContext = getComposerContext();
  const scrollState = composerContext.scrollState;
  const jumpState = composerContext.jumpState;
  const isLoading = $derived(messageStore.isInitialLoading);
  const isLoadingMore = $derived(messageStore.isLoadingMore);
  const hasReachedStart = $derived(messageStore.hasReachedStart);
  const isJumpedMode = $derived(jumpState.isJumpedMode);
  const isLoadingNewer = $derived(jumpState.isLoadingNewer);
  const hasReachedEnd = $derived(jumpState.hasReachedEnd);
  const scrollToEventId = $derived(jumpState.scrollToEventId);
  const serverScope = useServerScope();
  const stores = $derived(serverScope.store);
  const currentUser = $derived(stores.currentUser);
  const serverInfo = $derived(stores.serverInfo);
  const roomMembers = $derived(getRoomMembers());
  const userInteractions = new MessageUserInteractionState(() => roomMembers);
  const isUniversal = $derived(stores.projection?.rooms?.get(roomId)?.room?.universal ?? false);
  const canStartDMs = $derived(stores.permissions?.canStartDMs ?? false);
  let overlayScope = untrack(() => `${serverScope.serverId}:${roomId}`);
  $effect(() => {
    const nextScope = `${serverScope.serverId}:${roomId}`;
    if (nextScope !== overlayScope) {
      userInteractions.close();
      overlayScope = nextScope;
    }
  });

  function openUserMenu(user: UserAvatarUserView | RoomMember, anchorRect: DOMRect | null) {
    userInteractions.showUser(user, anchorRect);
  }
  const userSettings = $derived(timeFormatSettingsFor(currentUser.user?.settings));
  const activeLocale = $derived(getLocale());
  const firstVisibleDate = $derived(
    viewport.firstVisibleAt
      ? formatDayLabel(viewport.firstVisibleAt, userSettings, activeLocale)
      : null
  );

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
    lastEditableMessageCtx.setFinder(() => {
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
  // A thread pane keeps this component when the user opens another thread.
  const timelineKey = $derived(
    permalinkThreadRootEventId ? `${roomId}:${permalinkThreadRootEventId}` : roomId
  );

  $effect(() => {
    const currentTimelineKey = timelineKey;
    const jumped = isJumpedMode;
    const newestId = timelineEvents.at(-1)?.id ?? null;
    untrack(() => {
      if (viewport.enterRoom(currentTimelineKey)) expandedSystemEventIds.clear();
      viewport.observeJumpedMode(jumped);
      // Comparing the newest ID rather than the count keeps prepended
      // pagination rows from looking like newly arrived messages.
      viewport.observeNewestEvent(newestId);
    });
  });

  // Watch for scroll-to-bottom requests from MessageComposer (after posting a message).
  // Posting is explicit user intent to see the bottom, so it releases the
  // controller's short virtualizer-correction lock.
  // Uses scrollContainer.scrollTop instead of scrollToIndex because the user may have
  // been scrolled up — unmeasured items at the bottom have only estimated heights,
  // causing scrollToIndex to undershoot.
  $effect(() => {
    const counter = scrollState.scrollRequestCounter;
    if (counter > 0) {
      viewport.requestBottom();
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

  async function landOnUnreadSeparator(requestedTimelineKey: string) {
    const current = () =>
      !destroyed && timelineKey === requestedTimelineKey && viewport.isUnreadEntryLandingRunning;

    await tick();
    // Virtua measures estimated rows after each scroll. Repeat until the
    // offset is stable so the separator ends up at the top of the viewport.
    let previousOffset: number | null = null;
    for (let attempt = 0; attempt < 10; attempt++) {
      if (!current()) return;
      const index = virtualItems.findIndex((item) => item.type === 'unread-separator');
      if (index === -1) break;
      safeScrollToIndex(index, { align: 'start' });
      await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
      const offset = virtualizerHandle?.getScrollOffset() ?? null;
      if (offset === null || offset === previousOffset) break;
      previousOffset = offset;
    }
    if (!current()) return;

    // If all unread messages fit on screen, the scroll clamps to the bottom
    // and the timeline keeps following new messages.
    viewport.finishUnreadEntryLanding(distanceFromBottom());
  }

  // Scroll container and virtualizer handle
  let scrollContainer = $state<HTMLDivElement>();
  let virtualizerHandle = $state<VirtualizerHandle>();

  // Build a DOM command only after fresh authority and the virtualizer are ready.
  const recoveryTarget = $derived.by(() => {
    const position = messageStore.recoveryViewport;
    if (!position || isLoading || stores.realtimeSync.isRecoveringSnapshot) return null;
    const items = virtualItems;
    if (items.length > 0 && !virtualizerHandle) return null;
    const index = items.findIndex((item) =>
      item.type === 'event'
        ? item.event.id === position.eventId
        : item.type === 'system-group' && item.events.some((event) => event.id === position.eventId)
    );
    return { position, index, store: messageStore };
  });

  // Land on the unread separator once per room or thread entry. The marker resolves
  // after the entry read request, so the initial bottom scroll may already
  // have run; any explicit viewport action before then wins (see
  // TimelineViewportController.beginUnreadEntryLanding). An entry that
  // targets a specific message skips the landing.
  const unreadEntryLanding = $derived.by(() => {
    if (scrollToEventId || pendingHighlightId) return { timelineKey, skip: true };
    if (!effectiveUnreadAfterEventId || isJumpedMode) return null;
    if (!virtualizerHandle || virtualItems.length === 0) return null;
    if (messageStore.recoveryViewport || stores.realtimeSync.isRecoveringSnapshot) return null;
    return { timelineKey, skip: false };
  });

  /** Apply the derived landing request; the controller makes it one-shot. */
  function landOnUnreadEntry(request: typeof unreadEntryLanding) {
    return () =>
      untrack(() => {
        if (!request) return;
        if (request.skip) viewport.skipUnreadEntryLanding(request.timelineKey);
        else if (viewport.beginUnreadEntryLanding(request.timelineKey)) {
          void landOnUnreadSeparator(request.timelineKey);
        }
      });
  }

  /** Coordinates belong to this mounted timeline, not to its cached store. */
  function ownViewport(store: MessagesStore) {
    return () => () => store.clearViewport();
  }

  /** Apply the derived scroll command after layout; detach cancels pending work. */
  function restoreViewport(target: typeof recoveryTarget) {
    return () => {
      if (!target) return;
      const frame = requestAnimationFrame(() => {
        const { position, index, store } = target;
        if (index >= 0) {
          viewport.beginJump();
          // Requests for the discarded window cannot release this flag.
          jumpState.isLoadingNewer = false;
          jumpState.isJumpedMode = position.hasNewer ?? false;
          jumpState.hasReachedEnd = !position.hasNewer;
          safeScrollToIndex(index, { align: 'start', offset: position.offset });
          store.recoveryViewport = null;
        } else {
          store.clearViewport();
          jumpState.reset();
          viewport.followBottom();
          void requestBottomScroll();
        }
      });
      return () => cancelAnimationFrame(frame);
    };
  }
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
    if (stores.realtimeSync.isRecoveringSnapshot || messageStore.recoveryViewport) return undefined;
    if (!scrollContainer || !virtualizerHandle || virtualItems.length === 0) return undefined;

    const token = viewport.beginBottomScroll(roomId);
    return convergeAtBottom({
      continueWhile: () =>
        // Check lifetime before reading derived props from the old room.
        !destroyed &&
        !stores.realtimeSync.isRecoveringSnapshot &&
        !messageStore.recoveryViewport &&
        viewport.canContinueBottomScroll(token, roomId, isJumpedMode) &&
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
    if (scrollContainer) {
      scrollState.setContainer(scrollContainer);
      return () => scrollState.setContainer(null);
    }
  });

  // Keep ScrollState's shouldScroll flag in sync with our local state
  $effect(() => {
    scrollState.setShouldScroll(viewport.shouldScrollToBottom);
  });

  // Auto-scroll to bottom when new events arrive or existing events update.
  // shouldScrollToBottom is read via untrack() so toggling it doesn't re-trigger
  // this effect — it only gates whether we scroll when new data arrives.
  // Suppressed in jumped mode — we don't want to auto-scroll when viewing history.
  // Suppressed when pendingHighlightId is set — a highlight scroll is pending and
  // auto-scroll would race with it, scrolling to bottom before the highlight can fire.
  $effect(() => {
    void events.length;

    if (isJumpedMode) return;
    if (pendingHighlightId) return;

    if (virtualItems.length > 0 && virtualizerHandle) {
      const shouldScroll = untrack(() => viewport.shouldScrollToBottom);
      if (shouldScroll) {
        void requestBottomScroll();
      }
    }
  });

  // Scroll to bottom when clicking the new messages indicator
  function scrollToBottom() {
    viewport.requestBottom();
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
    if (!(await messageStore.jumpToPresent(jumpState))) return;
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
    viewport.reconcileAfterTabResume(dist);
  });

  let forwardLoadInFlight = false;
  let underfilledBackfillInFlight = false;

  function exitJumpedModeAtPresent(bottomDistance: number): boolean {
    if (!isJumpedMode || !hasReachedEnd || bottomDistance >= 50) return false;

    viewport.followBottom();
    onReachedBottom?.();
    console.debug('[room-refresh] reached present after forward pagination', {
      roomId,
      bottomDistance,
      itemCount: virtualItems.length
    });
    jumpState.reset();
    return true;
  }

  async function loadNewerAndMaybeExitAtPresent(): Promise<void> {
    if (forwardLoadInFlight) return;

    forwardLoadInFlight = true;
    try {
      await messageStore.loadNewer(jumpState);
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
    if (stores.realtimeSync.isRecoveringSnapshot || messageStore.recoveryViewport) return;
    if (
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
        await messageStore.loadMore();
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
        await messageStore.loadMore();
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
    if (
      !virtualizerHandle ||
      isLoading ||
      stores.realtimeSync.isRecoveringSnapshot ||
      messageStore.recoveryViewport
    )
      return;

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
      now: Date.now()
    });
    const { distanceFromBottom } = scrollResult;
    // A separator can be the first visible item. Anchor to the next event so
    // dates and unread markers do not discard the reading position.
    let anchorIndex = idx;
    while (
      anchorIndex < virtualItems.length &&
      virtualItems[anchorIndex].type !== 'event' &&
      virtualItems[anchorIndex].type !== 'system-group'
    )
      anchorIndex++;
    const anchor = virtualItems[anchorIndex];
    const anchorEvent =
      anchor?.type === 'event'
        ? anchor.event
        : anchor?.type === 'system-group'
          ? anchor.events[0]
          : undefined;
    messageStore.setViewport(
      !viewport.shouldScrollToBottom && anchorEvent
        ? { eventId: anchorEvent.id, offset: offset - virtualizerHandle.getItemOffset(anchorIndex) }
        : null
    );
    if (scrollResult.reachedBottom) onReachedBottom?.();

    // Trigger pagination when scrolled near the top.
    // Guard: only when content actually overflows the viewport (avoids firing in short rooms).
    if (
      offset < viewportSize * 3 &&
      scrollSize > viewportSize + 50 &&
      !isLoadingMore &&
      !hasReachedStart
    ) {
      // No manual scroll restoration needed — virtua's shift=true handles it
      void messageStore.loadMore();
    }

    // Forward pagination when near bottom in jumped mode
    if (
      isJumpedMode &&
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

<div class="relative flex min-h-0 min-w-0 flex-1 flex-col pb-2" {@attach ownViewport(messageStore)}>
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
    <div
      class="mt-auto mobile-presentation:px-1"
      {@attach restoreViewport(recoveryTarget)}
      {@attach landOnUnreadEntry(unreadEntryLanding)}
    >
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
                  onOpenUser={openUserMenu}
                  {threadingMode}
                />
              {/if}
            {/if}
          {/snippet}
        </Virtualizer>
      {/if}
    </div>
  </ScrollFader>

  <TypingIndicator {typingUserIds} members={typingMembers} profiles={stores.projection.users} />

  {#if !viewport.shouldScrollToBottom}
    <button
      transition:fade={{ duration: 150 }}
      onclick={isJumpedMode ? handleJumpToPresentClick : scrollToBottom}
      data-testid="jump-to-present"
      class="absolute bottom-4 left-1/2 z-40 -translate-x-1/2 cursor-pointer menu whitespace-nowrap"
    >
      <div class="flex items-center gap-2 menu-section px-3 py-1">
        {#if firstVisibleDate}
          <span class="text-muted">{firstVisibleDate}</span>
          <span class="text-muted/40">|</span>
        {/if}
        <span>
          {!isJumpedMode && viewport.hasNewMessages
            ? m('room.unread_separator')
            : m('room.jump_to_present')}
        </span>
        <span class="iconify icon-[uil--arrow-down]"></span>
      </div>
    </button>
  {/if}

  {#key `${serverScope.serverId}:${roomId}`}
    <MessageUserOverlays
      interactions={userInteractions}
      serverId={serverScope.serverId}
      {roomId}
      currentUserId={currentUser.user?.id}
      {canStartDMs}
      canBanRoomMembers={roomPermissions.canBanRoomMembers}
      {isUniversal}
      {onOpenProfile}
    />
  {/key}
</div>
