<script lang="ts">
  import { tick, untrack } from 'svelte';
  import { fade } from 'svelte/transition';
  import { Virtualizer, type VirtualizerHandle } from 'virtua/svelte';
  import * as m from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import type { RoomEventView } from '$lib/render/types';
  import { isMessagePostedEvent } from '$lib/render/eventKinds';
  import type { MessagesStore, RefreshCurrentWindowResult, RoomMember } from '$lib/state/room';
  import { getComposerContext, getRoomPermissions } from '$lib/state/room';
  import RoomEvent from './RoomEvent.svelte';
  import SystemEventGroup from './SystemEventGroup.svelte';
  import DaySeparator from './DaySeparator.svelte';
  import UnreadSeparator from './UnreadSeparator.svelte';
  import TypingIndicator from './TypingIndicator.svelte';
  import { computeEventMetadata } from './messageGrouping';
  import { buildVirtualItems, type VirtualItem } from './virtualItems';
  import { findLastEditableMessage } from './lastEditableMessage';
  import ScrollFader from '$lib/ui/ScrollFader.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getUserSettings } from '$lib/state/userSettings.svelte';
  import { formatDayLabel } from '$lib/utils/formatTime';
  import { useTabResumeCallback } from '$lib/hooks/useTabResumeCallback.svelte';
  import {
    useMayHaveMissedMessagesCallback,
    type MayHaveMissedMessagesReason
  } from '$lib/hooks/useMayHaveMissedMessagesCallback.svelte';
  import type { OpenThreadHandler, ThreadOpenOptions } from './threadOpenOptions';

  let {
    roomId,
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
    // Filtering - whether to filter out thread replies (false for thread pane)
    filterThreadReplies = true,
    // Up-arrow-to-edit
    enableLastEditableFinder = false,
    // Loading states
    isLoading = false,
    emptyMessage = m['room.message.empty'](),
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
    onSoftRefresh,
    pendingHighlightId = null
  }: {
    roomId: string;
    messageStore: MessagesStore;
    events: RoomEventView[];
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
    onScrollToEventComplete?: () => void;
    isJumpedMode?: boolean;
    isLoadingNewer?: boolean;
    hasReachedEnd?: boolean;
    onLoadNewer?: () => Promise<void>;
    onJumpToPresent?: () => void;
    onReachedPresent?: () => void;
    onSoftRefresh?: (result: RefreshCurrentWindowResult, anchored: boolean) => void;
    // Suppress auto-scroll while a highlight is pending (used by ThreadPane)
    pendingHighlightId?: string | null;
  } = $props();

  type RefreshAnchor = {
    eventId: string;
    top: number;
  };

  let initialScrollDone = $state(false);

  // State for smart scroll behavior (when not alwaysScrollToBottom)
  let shouldScrollToBottom = $state(true);
  let hasNewMessages = $state(false);
  let lastSeenNewestId = $state<string | null>(null);
  let firstVisibleAt = $state<string | null>(null);

  function setShouldScrollToBottom(value: boolean) {
    shouldScrollToBottom = value;
    if (value) {
      hasNewMessages = false;
      firstVisibleAt = null;
    }
  }

  // Track previous scroll offset for direction detection
  let previousOffset = $state<number | null>(null);

  // Get composer context (scrollState may be null - ThreadPane doesn't provide it)
  const composerContext = getComposerContext();
  const scrollState = composerContext.scrollState;
  const userSettings = getUserSettings();
  const activeLocale = $derived(getLocale());
  const firstVisibleDate = $derived(
    firstVisibleAt ? formatDayLabel(firstVisibleAt, userSettings, activeLocale) : null
  );

  // Filter events based on configuration
  let filteredEvents = $derived(
    events.filter((e) => {
      if (!isMessagePostedEvent(e.event)) return true;

      const msg = e.event;

      // Filter out thread replies when enabled (main room view)
      // In thread pane, filterThreadReplies=false to show all messages
      if (filterThreadReplies && msg?.threadRootEventId != null) return false;

      // Deleted messages (body === null) are always shown with placeholder
      return true;
    })
  );

  // Apply message grouping and day separators
  let eventsWithMeta = $derived(computeEventMetadata(filteredEvents, userSettings, activeLocale));

  // The unread separator event ID is computed by the parent component
  // (RoomEventsPane for sequence-based, ThreadPane for time-based)
  // and passed in directly as unreadAfterEventId.

  // Build flat array for the virtualizer (events + interleaved separators)
  let virtualItems = $derived(
    buildVirtualItems(eventsWithMeta, unreadAfterEventId ?? null, hasReachedStart, showStartMarker)
  );

  // Register finder for up-arrow-to-edit (computed on-demand, not reactively)
  const lastEditableMessageCtx = composerContext.lastEditableMessage;
  const stores = serverRegistry.getStore(getActiveServer());
  const currentUser = $derived(stores.currentUser);
  const serverInfo = stores.serverInfo;
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

  // Reset scroll state when room changes
  $effect(() => {
    void roomId;

    initialScrollDone = false;
    setShouldScrollToBottom(true);
    lastSeenNewestId = null;
    firstVisibleAt = null;
    previousOffset = null;
  });

  // When exiting jumped mode (returning to present), re-enable auto-scroll
  // so the latest messages are visible at the bottom.
  let prevJumpedMode: boolean | undefined;
  $effect(() => {
    if (prevJumpedMode && !isJumpedMode) {
      setShouldScrollToBottom(true);
    }
    prevJumpedMode = isJumpedMode;
  });

  // Track new messages arriving while scrolled up (only when indicator is enabled).
  // Compares the newest event's ID rather than the count, so that loading older
  // messages via pagination (which prepends to the array) doesn't falsely trigger.
  $effect(() => {
    if (!showNewMessagesIndicator || alwaysScrollToBottom) return;
    if (filteredEvents.length === 0) return;

    const newestId = filteredEvents[filteredEvents.length - 1].id;

    if (lastSeenNewestId !== null && newestId !== lastSeenNewestId && !shouldScrollToBottom) {
      hasNewMessages = true;
    }

    lastSeenNewestId = newestId;
  });

  // Watch for scroll-to-bottom requests from MessageComposer (after posting a message).
  // Clears scrollUpLock since posting a message is explicit user intent to see the bottom.
  // Uses scrollContainer.scrollTop instead of scrollToIndex because the user may have
  // been scrolled up — unmeasured items at the bottom have only estimated heights,
  // causing scrollToIndex to undershoot.
  $effect(() => {
    if (!scrollState || alwaysScrollToBottom) return;
    const counter = scrollState.scrollRequestCounter;
    if (counter > 0) {
      setShouldScrollToBottom(true);
      scrollUpLock = false;
      if (scrollUpLockTimer) {
        clearTimeout(scrollUpLockTimer);
        scrollUpLockTimer = null;
      }
      tick().then(() => {
        if (scrollContainer && shouldScrollToBottom) {
          startScrollCorrection();
          scrollContainer.scrollTop = scrollContainer.scrollHeight;
          scrollFader?.refresh();
        }
      });
    }
  });

  // Scroll to a specific event by ID (for jump-to-message)
  $effect(() => {
    const targetId = scrollToEventId;
    if (!targetId || !virtualizerHandle || virtualItems.length === 0) return;

    // Find the target event's index in virtualItems
    const targetIdx = virtualItems.findIndex(
      (item) => item.type === 'event' && item.event.id === targetId
    );
    if (targetIdx === -1) return;

    // Disable auto-scroll so it doesn't race with the jump scroll.
    setShouldScrollToBottom(false);
    // Mark initial scroll as done so pending initial loading state cannot obscure the jump.
    initialScrollDone = true;

    // Wait for render, then scroll and highlight.
    // After a cache replacement (jump-to-message), virtua needs multiple frames
    // to measure items and render the target element. Retry the highlight
    // a few times to handle this latency.
    tick().then(() => {
      safeScrollToIndex(targetIdx, { align: 'center' });

      // After the scroll and virtualizer measurement settle, restore
      // shouldScrollToBottom if we landed at the bottom (e.g., linking to a
      // recent message, or content doesn't overflow the viewport). Without this,
      // the "Jump to Present" button appears spuriously because no scroll event
      // fires when content is shorter than the viewport.
      setTimeout(() => {
        if (!virtualizerHandle) return;
        const dist =
          virtualizerHandle.getScrollSize() -
          virtualizerHandle.getScrollOffset() -
          virtualizerHandle.getViewportSize();
        if (dist < 50) {
          setShouldScrollToBottom(true);
        }
      }, 200);

      let attempts = 0;
      function tryHighlight() {
        // Scope to this EventList's scroll container so the thread pane
        // highlights within the thread, not in the main room view.
        const scope = scrollContainer ?? document;
        const target = scope.querySelector(`[data-event-id="${targetId}"]`);
        if (target instanceof HTMLElement) {
          target.classList.add('highlight-flash');
          target.addEventListener(
            'animationend',
            () => target.classList.remove('highlight-flash'),
            { once: true }
          );
          onScrollToEventComplete?.();
        } else if (attempts < 15) {
          attempts++;
          requestAnimationFrame(tryHighlight);
        } else {
          // Give up — element never appeared, still signal completion
          onScrollToEventComplete?.();
        }
      }
      requestAnimationFrame(tryHighlight);
    });
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
    scrollState?.setShouldScroll(alwaysScrollToBottom || shouldScrollToBottom);
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
      const shouldScroll = untrack(() => alwaysScrollToBottom || shouldScrollToBottom);
      if (shouldScroll) {
        // Wait for Svelte to flush DOM updates so the virtualizer has
        // accurate measurements for the new items before scrolling.
        tick().then(() => {
          if (!virtualizerHandle) return;
          if (!untrack(() => alwaysScrollToBottom || shouldScrollToBottom)) return;
          startScrollCorrection();
          safeScrollToIndex(virtualItems.length - 1, { align: 'end' });
          scrollFader?.refresh();
          if (!untrack(() => initialScrollDone)) {
            // Give virtua time to measure actual item heights and settle the
            // scroll position.
            setTimeout(() => {
              safeScrollToIndex(virtualItems.length - 1, { align: 'end' });
              scrollFader?.refresh();
              initialScrollDone = true;
            }, 80);
          }
        });
      }
    }
  });

  // Scroll to bottom when clicking the new messages indicator
  function scrollToBottom() {
    setShouldScrollToBottom(true);
    if (virtualizerHandle) {
      safeScrollToIndex(virtualItems.length - 1, { align: 'end' });
      scrollFader?.refresh();
    }
  }

  // Timer-based flag set by programmatic scrolls (auto-scroll effect, scroll-request
  // effect). During the window, handleVirtuaScroll will self-correct if the virtualizer
  // re-measures items (changing scrollHeight) and leaves position short of bottom.
  // Uses a timeout because the virtualizer may settle over multiple frames — a simple
  // distance check clears too early (first scroll reaches bottom, flag clears, then
  // virtualizer re-renders and grows scrollHeight).
  let pendingScrollCorrection = false;
  let scrollCorrectionTimer: ReturnType<typeof setTimeout> | null = null;

  function startScrollCorrection() {
    pendingScrollCorrection = true;
    if (scrollCorrectionTimer) clearTimeout(scrollCorrectionTimer);
    scrollCorrectionTimer = setTimeout(() => {
      pendingScrollCorrection = false;
      scrollCorrectionTimer = null;
    }, 500);
  }

  // Lock to prevent virtua's scroll corrections from immediately re-enabling
  // auto-scroll after we detect a user scroll-up. Without this, $fixScrollJump
  // can adjust the scroll position back near the bottom within the same frame,
  // causing handleVirtuaScroll to see distanceFromBottom < 50 and re-enable.
  let scrollUpLock = false;
  let scrollUpLockTimer: ReturnType<typeof setTimeout> | null = null;

  // Timestamp of the most recent user-driven scroll signal (wheel or touchmove).
  // The scroll-up branch in handleVirtuaScroll only fires when this is recent,
  // so virtua's internal scroll adjustments (re-measurement, $fixScrollJump),
  // composer-resize-driven scrollTop writes, and browser scroll clamping during
  // layout shifts never get misread as the user scrolling up.
  let userScrollIntentAt = 0;
  const USER_SCROLL_INTENT_MS = 250;

  function markUserScrollIntent() {
    userScrollIntentAt = Date.now();
  }

  function distanceFromBottom(): number | null {
    if (!virtualizerHandle) return null;
    return (
      virtualizerHandle.getScrollSize() -
      virtualizerHandle.getScrollOffset() -
      virtualizerHandle.getViewportSize()
    );
  }

  function eventIdForVirtualItem(item: VirtualItem): string | null {
    if (item.type === 'event') return item.event.id;
    if (item.type === 'system-group') return item.events[0]?.id ?? null;
    return null;
  }

  function eventSelector(eventId: string): string {
    return `[data-event-id="${CSS.escape(eventId)}"]`;
  }

  function captureRefreshAnchor(): RefreshAnchor | null {
    if (!scrollContainer || !virtualizerHandle || virtualItems.length === 0) return null;

    const startIdx = Math.max(
      0,
      virtualizerHandle.findItemIndex(virtualizerHandle.getScrollOffset())
    );
    for (let i = startIdx; i < virtualItems.length; i++) {
      const eventId = eventIdForVirtualItem(virtualItems[i]);
      if (!eventId) continue;

      const el = scrollContainer.querySelector<HTMLElement>(eventSelector(eventId));
      if (!el) continue;
      return {
        eventId,
        top: el.getBoundingClientRect().top
      };
    }

    console.debug('[room-refresh] no visible anchor found', { roomId });
    return null;
  }

  let softRefreshInFlight = false;

  async function refreshAfterPossibleMiss(reason: MayHaveMissedMessagesReason): Promise<boolean> {
    if (softRefreshInFlight) return false;
    if (isLoading && virtualItems.length === 0) return false;

    const bottomDistance = distanceFromBottom();
    const wasAtBottom =
      alwaysScrollToBottom ||
      (bottomDistance === null ? shouldScrollToBottom : bottomDistance < 50);
    const anchor = wasAtBottom ? null : captureRefreshAnchor();

    softRefreshInFlight = true;
    try {
      console.debug('[room-refresh] event list refresh started', {
        roomId,
        reason,
        mode: wasAtBottom ? 'latest' : 'anchored',
        bottomDistance,
        anchorEventId: anchor?.eventId ?? null,
        itemCount: virtualItems.length
      });
      const result = await messageStore.refreshCurrentWindow(anchor?.eventId ?? null);
      if (!result.refreshed) {
        console.debug('[room-refresh] event list refresh skipped after store refresh failed', {
          roomId,
          reason,
          result
        });
        return false;
      }
      onSoftRefresh?.(result, anchor !== null);
      await tick();
      await new Promise((resolve) => requestAnimationFrame(resolve));

      if (wasAtBottom) {
        setShouldScrollToBottom(true);
        initialScrollDone = true;
        startScrollCorrection();
        scrollContainer?.scrollTo({ top: scrollContainer.scrollHeight });
        if (virtualItems.length > 0) {
          safeScrollToIndex(virtualItems.length - 1, { align: 'end' });
        }
        scrollFader?.refresh();
        console.debug('[room-refresh] event list refresh completed at bottom', {
          roomId,
          result,
          itemCount: virtualItems.length
        });
        return true;
      }

      if (anchor && scrollContainer) {
        const target = scrollContainer.querySelector<HTMLElement>(eventSelector(anchor.eventId));
        if (target) {
          const nextTop = target.getBoundingClientRect().top;
          scrollContainer.scrollTop += nextTop - anchor.top;
          scrollFader?.refresh();
          console.debug('[room-refresh] anchor restored', {
            roomId,
            anchorEventId: anchor.eventId,
            deltaPx: nextTop - anchor.top,
            result,
            itemCount: virtualItems.length
          });
        } else {
          console.debug('[room-refresh] anchor disappeared after refresh', {
            roomId,
            anchorEventId: anchor.eventId,
            result,
            itemCount: virtualItems.length
          });
        }
      }
      return true;
    } finally {
      softRefreshInFlight = false;
    }
  }

  useMayHaveMissedMessagesCallback((reason) => refreshAfterPossibleMiss(reason));

  // Re-evaluate "are we at the bottom?" when the tab regains visibility — the
  // browser may have throttled virtua's measurements or our auto-scroll effect
  // while hidden, leaving shouldScrollToBottom=true even though the scroll has
  // drifted off the bottom (which would suppress the Jump to Present button).
  useTabResumeCallback(() => {
    if (alwaysScrollToBottom || !shouldScrollToBottom || !initialScrollDone) return;
    if (!virtualizerHandle) return;
    const dist =
      virtualizerHandle.getScrollSize() -
      virtualizerHandle.getScrollOffset() -
      virtualizerHandle.getViewportSize();
    if (dist > 50) setShouldScrollToBottom(false);
  });

  let forwardLoadInFlight = false;

  function exitJumpedModeAtPresent(bottomDistance: number): boolean {
    if (!isJumpedMode || !hasReachedEnd || bottomDistance >= 50 || !onReachedPresent) return false;

    setShouldScrollToBottom(true);
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

  // Handle scroll events from virtua to detect user intent and trigger pagination.
  // virtua's shift=true handles scroll restoration during pagination automatically,
  // eliminating the need for manual scrollHeight capture/restore and overflow-anchor toggling.
  function handleVirtuaScroll(offset: number) {
    if (!virtualizerHandle) return;

    const scrollSize = virtualizerHandle.getScrollSize();
    const viewportSize = virtualizerHandle.getViewportSize();
    const distanceFromBottom = scrollSize - offset - viewportSize;

    // Smart scroll: detect user scroll direction
    if (!alwaysScrollToBottom) {
      // Re-enable auto-scroll if we're at the bottom (and not locked)
      if (distanceFromBottom < 10 && !scrollUpLock) {
        setShouldScrollToBottom(true);
      }
      // Disable auto-scroll if user scrolled up (and clearly not near the bottom).
      // Gated on a recent wheel/touchmove signal so virtua's internal scroll
      // corrections ($fixScrollJump after re-measuring items), composer-resize
      // scrollTop writes, and browser scroll-clamping during layout shifts can't
      // be misread as the user scrolling up. The distanceFromBottom guard is
      // kept as a second line of defense for the brief window where intent is
      // still armed from a fling that already settled near the bottom.
      else if (
        Date.now() - userScrollIntentAt < USER_SCROLL_INTENT_MS &&
        previousOffset !== null &&
        offset < previousOffset - 10 &&
        distanceFromBottom > 20
      ) {
        setShouldScrollToBottom(false);
        pendingScrollCorrection = false;
        if (scrollCorrectionTimer) {
          clearTimeout(scrollCorrectionTimer);
          scrollCorrectionTimer = null;
        }
        scrollUpLock = true;
        if (scrollUpLockTimer) clearTimeout(scrollUpLockTimer);
        scrollUpLockTimer = setTimeout(() => {
          scrollUpLock = false;
        }, 150);
      }
    }

    // Self-correcting scroll: after a programmatic scroll, the virtualizer may
    // re-measure items (changing scrollHeight), leaving the position short of
    // the bottom. Re-scroll to the true bottom. Only fires during the short
    // window after a programmatic scroll set the flag — never during user scrolling.
    // Also gated on shouldScrollToBottom so a stale correction window from the
    // initial auto-scroll doesn't yank the user back to the bottom after a
    // jump-to-message takes them to an old event within those 500ms.
    if (
      pendingScrollCorrection &&
      (alwaysScrollToBottom || shouldScrollToBottom) &&
      distanceFromBottom > 50 &&
      scrollContainer
    ) {
      scrollContainer.scrollTop = scrollContainer.scrollHeight;
      scrollFader?.refresh();
    }

    previousOffset = offset;

    // Track the date of the first visible event for the "Jump to Present" button
    if (!shouldScrollToBottom && virtualizerHandle) {
      const idx = virtualizerHandle.findItemIndex(offset);
      // Walk forward from the found index to find the first event-type item
      for (let i = idx; i < virtualItems.length; i++) {
        const item = virtualItems[i];
        if (item.type === 'event') {
          firstVisibleAt = item.event.createdAt;
          break;
        }
      }
    }

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
  function getOpenThreadHandler(event: RoomEventView) {
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

<div class="relative flex min-h-0 min-w-0 flex-1 flex-col pb-2">
  <!-- Gradient fade overlay at top -->
  <div
    class="pointer-events-none absolute inset-x-0 top-0 z-10 h-8 bg-linear-to-b from-background/60 to-transparent"
  ></div>

  <ScrollFader
    top
    bottom
    bind:this={scrollFader}
    bind:scrollEl={scrollContainer}
    scrollClass="overscroll-y-contain"
    data-testid="messages-container"
    onwheel={markUserScrollIntent}
    ontouchmove={markUserScrollIntent}
  >
    <div class="mt-auto">
      {#if !isLoading && virtualItems.length === 0}
        <div class="flex flex-1 items-center justify-center">
          <div class="py-4 text-sm text-muted/40">{emptyMessage}</div>
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
              <div class="pt-10 pb-2 text-center text-sm text-muted/40">
                This is the beginning of this conversation.
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
                <SystemEventGroup events={groupEvents} kind={groupKind} />
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
                  {messageStore}
                  onOpenThread={getOpenThreadHandler(eventData)}
                />
              {/if}
            {/if}
          {/snippet}
        </Virtualizer>
      {/if}
    </div>
  </ScrollFader>

  <TypingIndicator {typingUserIds} members={typingMembers} />

  {#if isJumpedMode && !shouldScrollToBottom && onJumpToPresent}
    <button
      transition:fade={{ duration: 150 }}
      onclick={onJumpToPresent}
      data-testid="jump-to-present"
      class="absolute bottom-4 left-1/2 -translate-x-1/2 cursor-pointer menu whitespace-nowrap"
    >
      <div class="flex items-center gap-2 menu-section px-3 py-1">
        {#if firstVisibleDate}
          <span class="text-muted">{firstVisibleDate}</span>
          <span class="text-muted/40">|</span>
        {/if}
        <span>{m['room.jump_to_present']()}</span>
        <span class="iconify uil--arrow-down"></span>
      </div>
    </button>
  {:else if !alwaysScrollToBottom && !shouldScrollToBottom}
    <button
      transition:fade={{ duration: 150 }}
      onclick={scrollToBottom}
      data-testid="jump-to-present"
      class="absolute bottom-4 left-1/2 -translate-x-1/2 cursor-pointer menu whitespace-nowrap"
    >
      <div class="flex items-center gap-2 menu-section px-3 py-1">
        {#if firstVisibleDate}
          <span class="text-muted">{firstVisibleDate}</span>
          <span class="text-muted/40">|</span>
        {/if}
        <span>{hasNewMessages ? m['room.unread_separator']() : m['room.jump_to_present']()}</span>
        <span class="iconify uil--arrow-down"></span>
      </div>
    </button>
  {/if}
</div>
