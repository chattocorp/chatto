<script lang="ts">
  import { tick, untrack } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';
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
  import { INITIAL_ROOM_MESSAGE_BACKFILL_TARGET } from '$lib/state/room/messages/queries';
  import { formatDayLabel } from '$lib/utils/formatTime';
  import { useTabResumeCallback } from '$lib/hooks/useTabResumeCallback.svelte';
  import { useMayHaveMissedMessagesCallback } from '$lib/hooks/useMayHaveMissedMessagesCallback.svelte';
  import type { ResumeSignal } from '$lib/hooks/resumeCoordinator.svelte';
  import type { OpenThreadHandler, ThreadOpenOptions } from './threadOpenOptions';
  import { convergeAtBottom } from './bottomScrollConvergence';
  import {
    scheduleNextTombstoneExpiry,
    shouldHideTombstone,
    visibleTombstoneEvents,
    visibleUnreadMarkerEventId
  } from './tombstoneVisibility';

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
    onReachedBottom,
    onSoftRefresh,
    pendingHighlightId = null
  }: {
    roomId: string;
    permalinkThreadRootEventId?: string | null;
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
    onScrollToEventComplete?: (landed: boolean) => void;
    isJumpedMode?: boolean;
    isLoadingNewer?: boolean;
    hasReachedEnd?: boolean;
    onLoadNewer?: () => Promise<void>;
    onJumpToPresent?: () => Promise<boolean>;
    onReachedPresent?: () => void;
    onReachedBottom?: () => void;
    onSoftRefresh?: (result: RefreshCurrentWindowResult, anchored: boolean) => void;
    // Suppress auto-scroll while a highlight is pending (used by ThreadPane)
    pendingHighlightId?: string | null;
  } = $props();

  type RefreshAnchor = {
    eventId: string;
    top: number;
  };

  let initialScrollDone = $state(false);
  let bottomScrollOperation = 0;
  let userScrollIntentAt = 0;
  const USER_SCROLL_INTENT_MS = 250;

  // State for smart scroll behavior (when not alwaysScrollToBottom)
  let shouldScrollToBottom = $state(true);
  let hasNewMessages = $state(false);
  let lastSeenNewestId = $state<string | null>(null);
  let firstVisibleAt = $state<string | null>(null);
  const expandedSystemEventIds = new SvelteSet<string>();
  let expandedStateRoomId: string | null = null;

  function isSystemGroupExpanded(groupEvents: RoomEventView[]): boolean {
    return groupEvents.some((event) => expandedSystemEventIds.has(event.id));
  }

  function setSystemGroupExpanded(groupEvents: RoomEventView[], expanded: boolean): void {
    for (const event of groupEvents) {
      if (expanded) {
        expandedSystemEventIds.add(event.id);
      } else {
        expandedSystemEventIds.delete(event.id);
      }
    }
  }

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

  // First apply structural timeline filtering. Tombstone expiry is a separate
  // stage so row removal cannot be mistaken for a newly arrived message.
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
  let tombstoneClockVersion = $state(0);
  let filteredEvents = $derived.by(() => {
    void tombstoneClockVersion;
    const nowMs = Date.now();
    return visibleTombstoneEvents(timelineEvents, nowMs);
  });
  let messageEventCount = $derived(
    filteredEvents.filter((event) => isMessagePostedEvent(event.event)).length
  );

  // Apply message grouping and day separators
  let eventsWithMeta = $derived(computeEventMetadata(filteredEvents, userSettings, activeLocale));

  // If the marker points at an expired tombstone, move it to the next visible
  // event instead of silently dropping the unread boundary.
  let effectiveUnreadAfterEventId = $derived.by(() => {
    return visibleUnreadMarkerEventId(timelineEvents, filteredEvents, unreadAfterEventId ?? null);
  });

  // Build flat array for the virtualizer (events + interleaved separators)
  let virtualItems = $derived(
    buildVirtualItems(eventsWithMeta, effectiveUnreadAfterEventId, hasReachedStart, showStartMarker)
  );

  async function expireTombstones(atMs: number) {
    const bottomDistance = distanceFromBottom();
    const wasAtBottom =
      alwaysScrollToBottom ||
      (bottomDistance === null ? shouldScrollToBottom : bottomDistance < 50);
    const anchor = wasAtBottom ? null : captureRefreshAnchor(atMs);

    tombstoneClockVersion += 1;
    await tick();

    if (wasAtBottom && scrollContainer) {
      await new Promise((resolve) => requestAnimationFrame(resolve));
      scrollContainer.scrollTop = scrollContainer.scrollHeight;
      scrollFader?.refresh();
      return;
    }
    if (!anchor || !scrollContainer) return;

    // Virtua can measure and correct the keyed list over several frames. Keep
    // restoring the same event anchor while those measurements settle.
    for (let frame = 0; frame < 4; frame++) {
      await new Promise((resolve) => requestAnimationFrame(resolve));
      const target = scrollContainer.querySelector<HTMLElement>(eventSelector(anchor.eventId));
      if (!target) return;
      scrollContainer.scrollTop += target.getBoundingClientRect().top - anchor.top;
    }
    scrollFader?.refresh();
  }

  $effect(() => {
    void tombstoneClockVersion;
    const nowMs = Date.now();
    return scheduleNextTombstoneExpiry(timelineEvents, nowMs, (expiresAt) => {
      void expireTombstones(expiresAt);
    });
  });

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

    cancelBottomScroll();
    initialScrollDone = false;
    setShouldScrollToBottom(true);
    lastSeenNewestId = null;
    firstVisibleAt = null;
    previousOffset = null;
  });

  $effect(() => {
    const currentRoomId = roomId;
    if (expandedStateRoomId === null) {
      expandedStateRoomId = currentRoomId;
      return;
    }
    if (currentRoomId === expandedStateRoomId) return;
    expandedStateRoomId = currentRoomId;
    // This reset belongs exclusively to room navigation. Reading the reactive
    // set here would also subscribe the effect to expansion changes.
    untrack(() => expandedSystemEventIds.clear());
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
    if (timelineEvents.length === 0) return;
    const newestId = timelineEvents[timelineEvents.length - 1].id;

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
          void requestBottomScroll();
        }
      });
    }
  });

  // Scroll to a specific event by ID (for jump-to-message)
  let scrollAttemptId = 0;
  $effect(() => {
    const attemptId = ++scrollAttemptId;
    const targetId = scrollToEventId;
    if (!targetId || !virtualizerHandle || virtualItems.length === 0) return;
    const targetEventId = targetId;

    // Disable auto-scroll so it doesn't race with the jump scroll.
    setShouldScrollToBottom(false);
    // Mark initial scroll as done so pending initial loading state cannot obscure the jump.
    initialScrollDone = true;

    // After a cache replacement, virtua can need several frames before the
    // target item is indexed, measured, and mounted. Retry the full lookup +
    // scroll path instead of giving up before the target is renderable.
    tick().then(() => {
      let attempts = 0;
      const maxAttempts = 60;
      let completed = false;

      function complete(landed: boolean) {
        if (completed || scrollAttemptId !== attemptId) return;
        if (!landed) {
          completed = true;
          onScrollToEventComplete?.(false);
          return;
        }

        // Check after the successful target scroll has settled. Starting this
        // timer before the virtual row mounts can re-enable bottom scrolling
        // based on the previous window's offset.
        setTimeout(() => {
          if (completed || !virtualizerHandle || scrollAttemptId !== attemptId) return;
          const dist =
            virtualizerHandle.getScrollSize() -
            virtualizerHandle.getScrollOffset() -
            virtualizerHandle.getViewportSize();
          if (dist < 50) setShouldScrollToBottom(true);
          completed = true;
          onScrollToEventComplete?.(true);
        }, 200);
      }

      function tryScrollAndHighlight() {
        if (scrollAttemptId !== attemptId) return;

        const targetIdx = virtualItems.findIndex(
          (item) => item.type === 'event' && item.event.id === targetEventId
        );
        if (targetIdx !== -1) {
          safeScrollToIndex(targetIdx, { align: 'center' });
        }

        // Scope to this EventList's scroll container so the thread pane
        // highlights within the thread, not in the main room view.
        const scope = scrollContainer ?? document;
        const target = scope.querySelector(eventSelector(targetEventId));
        if (target instanceof HTMLElement) {
          target.classList.add('highlight-flash');
          target.addEventListener(
            'animationend',
            () => target.classList.remove('highlight-flash'),
            { once: true }
          );
          complete(true);
          return;
        }

        if (attempts >= maxAttempts) {
          complete(false);
          return;
        }
        attempts++;
        requestAnimationFrame(tryScrollAndHighlight);
      }

      requestAnimationFrame(tryScrollAndHighlight);
    });

    return () => {
      if (scrollAttemptId === attemptId) scrollAttemptId++;
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

  function cancelBottomScroll() {
    bottomScrollOperation += 1;
  }

  function requestBottomScroll(): Promise<boolean> | undefined {
    if (!scrollContainer || !virtualizerHandle || virtualItems.length === 0) return undefined;

    const operation = ++bottomScrollOperation;
    const requestedRoomId = roomId;
    const intentAtStart = userScrollIntentAt;
    return convergeAtBottom({
      continueWhile: () =>
        operation === bottomScrollOperation &&
        roomId === requestedRoomId &&
        userScrollIntentAt === intentAtStart &&
        !isJumpedMode &&
        (alwaysScrollToBottom || shouldScrollToBottom) &&
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
      if (operation === bottomScrollOperation) initialScrollDone = true;
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
        void requestBottomScroll();
      }
    }
  });

  // Scroll to bottom when clicking the new messages indicator
  function scrollToBottom() {
    setShouldScrollToBottom(true);
    onReachedBottom?.();
    void requestBottomScroll();
  }

  async function handleJumpToPresentClick() {
    // The replacement latest window must perform a fresh initial-style bottom
    // scroll. Virtua otherwise preserves the historical window's offset when
    // the keyed data is replaced and can leave the user stranded mid-window.
    setShouldScrollToBottom(true);
    initialScrollDone = false;
    scrollUpLock = false;
    onReachedBottom?.();
    const requestedRoomId = roomId;
    const intentAtStart = userScrollIntentAt;
    if (!(await onJumpToPresent?.())) return;
    await tick();
    if (roomId !== requestedRoomId || userScrollIntentAt !== intentAtStart) return;
    void requestBottomScroll();
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
  function markUserScrollIntent() {
    userScrollIntentAt = Date.now();
    cancelBottomScroll();
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

  function eventIdForVirtualItem(item: VirtualItem): string | null {
    if (item.type === 'event') return item.event.id;
    if (item.type === 'system-group') return item.events[0]?.id ?? null;
    return null;
  }

  function eventSelector(eventId: string): string {
    return `[data-event-id="${CSS.escape(eventId)}"]`;
  }

  function captureRefreshAnchor(visibleAtMs?: number): RefreshAnchor | null {
    if (!scrollContainer || !virtualizerHandle || virtualItems.length === 0) return null;

    const viewportTop = scrollContainer.getBoundingClientRect().top;
    let partiallyVisibleAnchor: RefreshAnchor | null = null;
    const startIdx = Math.max(
      0,
      virtualizerHandle.findItemIndex(virtualizerHandle.getScrollOffset())
    );
    for (let i = startIdx; i < virtualItems.length; i++) {
      const item = virtualItems[i];
      if (
        visibleAtMs !== undefined &&
        item.type === 'event' &&
        shouldHideTombstone(item.event, visibleAtMs)
      ) {
        continue;
      }
      const eventId = eventIdForVirtualItem(item);
      if (!eventId) continue;

      const el = scrollContainer.querySelector<HTMLElement>(eventSelector(eventId));
      if (!el) continue;
      const rect = el.getBoundingClientRect();
      if (rect.bottom <= viewportTop) continue;
      const candidate = {
        eventId,
        top: rect.top
      };
      if (rect.top >= viewportTop) return candidate;
      partiallyVisibleAnchor ??= candidate;
    }

    if (partiallyVisibleAnchor) return partiallyVisibleAnchor;
    console.debug('[room-refresh] no visible anchor found', { roomId });
    return null;
  }

  let softRefreshInFlight = false;
  const MIN_BROWSER_WAKE_REFRESH_HIDDEN_MS = 5_000;

  function isShortBrowserWake(signal: ResumeSignal): boolean {
    if (signal.source !== 'browser') return false;
    if (signal.reason !== 'visibility' && signal.reason !== 'pageshow') return false;
    return (
      signal.hiddenDurationMs !== null &&
      signal.hiddenDurationMs < MIN_BROWSER_WAKE_REFRESH_HIDDEN_MS
    );
  }

  async function refreshAfterPossibleMiss(signal: ResumeSignal): Promise<boolean> {
    if (softRefreshInFlight) return false;
    if (isLoading && virtualItems.length === 0) return false;
    if (isShortBrowserWake(signal)) {
      console.debug('[room-refresh] skipped short browser wake refresh', {
        roomId,
        reason: signal.reason,
        hiddenDurationMs: signal.hiddenDurationMs,
        epoch: signal.epoch
      });
      return false;
    }

    const bottomDistance = distanceFromBottom();
    const wasAtBottom =
      alwaysScrollToBottom ||
      (bottomDistance === null ? shouldScrollToBottom : bottomDistance < 50);
    const anchor = wasAtBottom ? null : captureRefreshAnchor();

    softRefreshInFlight = true;
    try {
      console.debug('[room-refresh] event list refresh started', {
        roomId,
        reason: signal.reason,
        source: signal.source,
        phase: signal.phase,
        hiddenDurationMs: signal.hiddenDurationMs,
        epoch: signal.epoch,
        mode: wasAtBottom ? 'latest' : 'anchored',
        wasAtBottom,
        bottomDistance,
        anchorEventId: anchor?.eventId ?? null,
        itemCount: virtualItems.length
      });
      const result = await messageStore.refreshCurrentWindow(
        wasAtBottom ? null : (anchor?.eventId ?? null)
      );
      if (!result.refreshed) {
        console.debug('[room-refresh] event list refresh skipped after store refresh failed', {
          roomId,
          reason: signal.reason,
          source: signal.source,
          phase: signal.phase,
          wasAtBottom,
          result
        });
        return false;
      }
      onSoftRefresh?.(result, anchor !== null);
      if (!result.changed) {
        console.debug('[room-refresh] event list refresh completed unchanged', {
          roomId,
          result,
          itemCount: virtualItems.length
        });
        return true;
      }
      await tick();
      await new Promise((resolve) => requestAnimationFrame(resolve));

      if (wasAtBottom) {
        setShouldScrollToBottom(true);
        await requestBottomScroll();
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

  useMayHaveMissedMessagesCallback((signal) => refreshAfterPossibleMiss(signal));

  // Re-evaluate "are we at the bottom?" when the tab regains visibility — the
  // browser may have throttled virtua's measurements or our auto-scroll effect
  // while hidden, leaving shouldScrollToBottom=true even though the scroll has
  // drifted off the bottom (which would suppress the Jump to Present button).
  useTabResumeCallback(() => {
    tombstoneClockVersion += 1;
    if (alwaysScrollToBottom || !shouldScrollToBottom || !initialScrollDone) return;
    if (!virtualizerHandle) return;
    const dist =
      virtualizerHandle.getScrollSize() -
      virtualizerHandle.getScrollOffset() -
      virtualizerHandle.getViewportSize();
    if (dist > 50) setShouldScrollToBottom(false);
  });

  let forwardLoadInFlight = false;
  let underfilledBackfillInFlight = false;

  function exitJumpedModeAtPresent(bottomDistance: number): boolean {
    if (!isJumpedMode || !hasReachedEnd || bottomDistance >= 50 || !onReachedPresent) return false;

    setShouldScrollToBottom(true);
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
      // A fetched page can consist entirely of expired tombstones. There is no
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
    const distanceFromBottom = scrollSize - offset - viewportSize;

    // Smart scroll: detect user scroll direction
    if (!alwaysScrollToBottom) {
      // Re-enable auto-scroll if we're at the bottom (and not locked)
      if (distanceFromBottom < 10 && !scrollUpLock) {
        const wasScrolledUp = !shouldScrollToBottom;
        setShouldScrollToBottom(true);
        if (wasScrolledUp && Date.now() - userScrollIntentAt < USER_SCROLL_INTENT_MS) {
          onReachedBottom?.();
        }
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
        cancelBottomScroll();
        scrollUpLock = true;
        if (scrollUpLockTimer) clearTimeout(scrollUpLockTimer);
        scrollUpLockTimer = setTimeout(() => {
          scrollUpLock = false;
        }, 150);
      }
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

<svelte:window onkeydown={markKeyboardScrollIntent} />

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
                {m['room.timeline.beginning']()}
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
      onclick={handleJumpToPresentClick}
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
