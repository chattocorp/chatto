<script lang="ts">
  import { tick, untrack } from 'svelte';
  import { fade } from 'svelte/transition';
  import { Virtualizer, type VirtualizerHandle } from 'virtua/svelte';
  import type { RoomEventViewFragment } from '$lib/gql/graphql';
  import type { RoomMember } from '$lib/state/room';
  import { getComposerContext } from '$lib/state/room';
  import RoomEvent from './RoomEvent.svelte';
  import MessageEventSkeleton from './MessageEventSkeleton.svelte';
  import DaySeparator from './DaySeparator.svelte';
  import UnreadSeparator from './UnreadSeparator.svelte';
  import TypingIndicator from './TypingIndicator.svelte';
  import { computeEventMetadata } from './messageGrouping';
  import { buildVirtualItems, type VirtualItem } from './virtualItems';
  import { getCurrentUser } from '$lib/auth/currentUser.svelte';
  import { getUserSettings } from '$lib/state/userSettings.svelte';
  import { formatDayLabel } from '$lib/utils/formatTime';

  let {
    spaceId,
    roomId,
    events,
    // Scroll behavior
    alwaysScrollToBottom = false,
    showNewMessagesIndicator = true,
    // Pagination
    enablePagination = false,
    isLoadingMore = false,
    hasReachedStart = false,
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
    emptyMessage = 'No messages yet',
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
    pendingHighlightId = null
  }: {
    spaceId: string;
    roomId: string;
    events: RoomEventViewFragment[];
    // Scroll behavior
    alwaysScrollToBottom?: boolean;
    showNewMessagesIndicator?: boolean;
    // Pagination
    enablePagination?: boolean;
    isLoadingMore?: boolean;
    hasReachedStart?: boolean;
    onLoadMore?: () => Promise<void>;
    // Event updates
    updateCounter?: number;
    // Threading
    onOpenThread?: (threadRootEventId: string, highlightEventId?: string) => void;
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
    // Suppress auto-scroll while a highlight is pending (used by ThreadPane)
    pendingHighlightId?: string | null;
  } = $props();

  // Hide content until the first scroll-to-bottom completes, preventing
  // the flash where content renders at the top before jumping to the bottom.
  let initialScrollDone = $state(false);

  // State for smart scroll behavior (when not alwaysScrollToBottom)
  let shouldScrollToBottom = $state(true);
  let hasNewMessages = $state(false);
  let lastSeenNewestId = $state<string | null>(null);
  let firstVisibleDate = $state<string | null>(null);

  // Track previous scroll offset for direction detection
  let previousOffset = $state<number | null>(null);

  // Get composer context (scrollState may be null - ThreadPane doesn't provide it)
  const composerContext = getComposerContext();
  const scrollState = composerContext.scrollState;
  const userSettings = getUserSettings();

  // Sort events chronologically. Uses createdAt with event ID tiebreaker for
  // the rare case of sub-millisecond concurrent posts. Events from JetStream
  // are already ordered; this is a safety net for merged live + historical data.
  let sortedEvents = $derived(
    [...events].sort((a, b) => {
      const timeA = new Date(a.createdAt).getTime();
      const timeB = new Date(b.createdAt).getTime();
      if (timeA !== timeB) return timeA - timeB;
      return a.id < b.id ? -1 : a.id > b.id ? 1 : 0;
    })
  );

  // Filter events based on configuration
  let filteredEvents = $derived(
    sortedEvents.filter((e) => {
      if (e.event?.__typename !== 'MessagePostedEvent') return true;

      const msg = e.event;

      // Filter out thread replies when enabled (main room view)
      // In thread pane, filterThreadReplies=false to show all messages
      if (filterThreadReplies && msg?.inThread != null) return false;

      // Deleted messages (body === null) are always shown with placeholder
      return true;
    })
  );

  // Apply message grouping and day separators
  let eventsWithMeta = $derived(computeEventMetadata(filteredEvents, userSettings));

  // The unread separator event ID is computed by the parent component
  // (RoomEventsPane for sequence-based, ThreadPane for time-based)
  // and passed in directly as unreadAfterEventId.

  // Build flat array for the virtualizer (events + interleaved separators)
  let virtualItems = $derived(
    buildVirtualItems(eventsWithMeta, unreadAfterEventId ?? null, hasReachedStart)
  );

  // Register finder for up-arrow-to-edit (computed on-demand, not reactively)
  const lastEditableMessageCtx = composerContext.lastEditableMessage;
  const currentUser = getCurrentUser();

  $effect(() => {
    if (!enableLastEditableFinder) return;

    lastEditableMessageCtx?.setFinder(() => {
      const userId = currentUser.user?.id;
      if (!userId) return null;

      for (let i = filteredEvents.length - 1; i >= 0; i--) {
        const e = filteredEvents[i];
        if (e.actorId !== userId) continue;
        if (e.event?.__typename !== 'MessagePostedEvent') continue;
        if (e.event?.body == null) continue;
        return { eventId: e.id, body: e.event.body };
      }
      return null;
    });
  });

  // Reset scroll state when room changes
  $effect(() => {
    void roomId;

    initialScrollDone = false;
    shouldScrollToBottom = true;
    hasNewMessages = false;
    lastSeenNewestId = null;
    previousOffset = null;
  });

  // When exiting jumped mode (returning to present), re-enable auto-scroll
  // so the latest messages are visible at the bottom.
  let prevJumpedMode: boolean | undefined;
  $effect(() => {
    if (prevJumpedMode && !isJumpedMode) {
      shouldScrollToBottom = true;
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

    if (
      lastSeenNewestId !== null &&
      newestId !== lastSeenNewestId &&
      !shouldScrollToBottom
    ) {
      hasNewMessages = true;
    }

    lastSeenNewestId = newestId;
  });

  // Clear new messages indicator when scrolling back to bottom
  $effect(() => {
    if (shouldScrollToBottom) {
      hasNewMessages = false;
    }
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
      shouldScrollToBottom = true;
      scrollUpLock = false;
      if (scrollUpLockTimer) {
        clearTimeout(scrollUpLockTimer);
        scrollUpLockTimer = null;
      }
      tick().then(() => {
        if (scrollContainer && shouldScrollToBottom) {
          startScrollCorrection();
          scrollContainer.scrollTop = scrollContainer.scrollHeight;
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
    shouldScrollToBottom = false;
    // Mark initial scroll as done so the skeleton overlay is removed.
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
        const dist = virtualizerHandle.getScrollSize() - virtualizerHandle.getScrollOffset() - virtualizerHandle.getViewportSize();
        if (dist < 50) {
          shouldScrollToBottom = true;
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
          if (!untrack(() => initialScrollDone)) {
            // Give virtua time to measure actual item heights and settle the
            // scroll position. The skeleton overlay hides the content during
            // this window, so there's no visual cost to waiting.
            setTimeout(() => {
              safeScrollToIndex(virtualItems.length - 1, { align: 'end' });
              initialScrollDone = true;
            }, 80);
          }
        });
      }
    }
  });

  // Scroll to bottom when clicking the new messages indicator
  function scrollToBottom() {
    shouldScrollToBottom = true;
    hasNewMessages = false;
    if (virtualizerHandle) {
      safeScrollToIndex(virtualItems.length - 1, { align: 'end' });
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
      if (distanceFromBottom < 50 && !scrollUpLock) {
        shouldScrollToBottom = true;
      }
      // Disable auto-scroll if user scrolled up (and clearly not near the bottom).
      // The distanceFromBottom guard prevents virtua's internal scroll corrections
      // ($fixScrollJump) from being misinterpreted as user scrolling up.
      // The lock prevents the correction from immediately re-enabling auto-scroll.
      else if (
        previousOffset !== null &&
        offset < previousOffset - 10 &&
        distanceFromBottom > 100
      ) {
        shouldScrollToBottom = false;
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
    // the bottom. Re-scroll to the true bottom. Only fires when a programmatic
    // scroll set the flag — never during user-initiated scrolling.
    // Self-correcting scroll: after a programmatic scroll, the virtualizer may
    // re-measure items (changing scrollHeight), leaving the position short of
    // the bottom. Re-scroll to the true bottom. Only fires during the short
    // window after a programmatic scroll set the flag — never during user scrolling.
    if (pendingScrollCorrection && distanceFromBottom > 50 && scrollContainer) {
      scrollContainer.scrollTop = scrollContainer.scrollHeight;
    }

    previousOffset = offset;

    // Track the date of the first visible event for the "Jump to Present" button
    if (!shouldScrollToBottom && virtualizerHandle) {
      const idx = virtualizerHandle.findItemIndex(offset);
      // Walk forward from the found index to find the first event-type item
      for (let i = idx; i < virtualItems.length; i++) {
        const item = virtualItems[i];
        if (item.type === 'event') {
          firstVisibleDate = formatDayLabel(item.event.createdAt, userSettings);
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
      !hasReachedEnd
    ) {
      onLoadNewer();
    }

    // Exit jumped mode when user has scrolled to bottom and all content is loaded
    if (isJumpedMode && hasReachedEnd && distanceFromBottom < 50 && onJumpToPresent) {
      onJumpToPresent();
    }
  }

  // Determine if a message can open a thread
  // Root messages open their own thread; echoes open the original thread
  function getOpenThreadHandler(event: RoomEventViewFragment) {
    if (!onOpenThread) return undefined;

    const eventData = event.event;
    if (!eventData) return undefined;
    if (eventData.__typename === 'MessagePostedEvent') {
      // Echoes open the original thread
      if (eventData.echoOfEventId != null) {
        return (_threadRootEventId: string, highlightEventId?: string) =>
          onOpenThread(eventData.echoFromThreadRootEventId!, highlightEventId);
      }
      // Thread replies don't open threads from the main channel
      if (eventData.inThread !== null) return undefined;
      // Root messages open their own thread
      return () => onOpenThread(event.id);
    }

    return undefined;
  }
</script>

<div class="relative flex min-h-0 min-w-0 flex-1 flex-col">
  <!-- Gradient fade overlay at top -->
  <div
    class="pointer-events-none absolute inset-x-0 top-0 z-10 h-8 bg-linear-to-b from-background/60 to-transparent"
  ></div>

  <!-- Skeleton overlay: stays visible during loading AND while the virtualizer
       settles its initial scroll position, preventing the flash where content
       renders at the wrong position before jumping to the bottom. -->
  {#if isLoading || (!initialScrollDone && virtualItems.length > 0)}
    <div class="pointer-events-none absolute inset-0 z-[5] flex flex-col justify-end gap-2 overflow-hidden bg-background">
      <MessageEventSkeleton />
      {#each Array(15) as _, i (i)}
        <MessageEventSkeleton
          compact={i % 5 > 0}
          lines={(i % 3) + 1}
        />
      {/each}
    </div>
  {/if}

  <div
    bind:this={scrollContainer}
    class="flex min-h-0 min-w-0 flex-1 flex-col overflow-x-hidden overflow-y-auto overscroll-y-contain [&>div]:mt-auto"
    data-testid="messages-container"
  >
    {#if !isLoading && virtualItems.length === 0}
      <div class="flex flex-1 items-center justify-center">
        <div class="py-4 text-sm text-muted/40">{emptyMessage}</div>
      </div>
    {:else if !isLoading}
      {#if isLoadingMore}
        <div class="px-4 py-2">
          <MessageEventSkeleton />
          <MessageEventSkeleton compact />
          <MessageEventSkeleton compact />
        </div>
      {/if}
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
            <div class="py-2 text-center text-sm text-muted/40">
              You've reached the very beginning of this conversation.
            </div>
          {:else if item.type === 'day-separator'}
            <DaySeparator label={item.label} />
          {:else if item.type === 'unread-separator'}
            <UnreadSeparator />
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
                {spaceId}
                {roomId}
                onOpenThread={getOpenThreadHandler(eventData)}
              />
            {/if}
          {/if}
        {/snippet}
      </Virtualizer>
    {/if}
  </div>

  <TypingIndicator {typingUserIds} members={typingMembers} />

  {#if isJumpedMode && !shouldScrollToBottom && onJumpToPresent}
    <button
      transition:fade={{ duration: 150 }}
      onclick={onJumpToPresent}
      data-testid="jump-to-present"
      class="absolute bottom-4 left-1/2 -translate-x-1/2 cursor-pointer whitespace-nowrap rounded-lg bg-primary px-4 py-2 text-sm font-medium text-white shadow-lg hover:bg-primary/90"
    >
      {#if firstVisibleDate}
        <span class="opacity-70">{firstVisibleDate}</span>
        <span class="mx-1.5 opacity-40">|</span>
      {/if}
      Jump to Present
      <span class="ml-1 iconify inline-block align-middle uil--arrow-down"></span>
    </button>
  {:else if !alwaysScrollToBottom && !shouldScrollToBottom}
    <button
      transition:fade={{ duration: 150 }}
      onclick={scrollToBottom}
      data-testid="jump-to-present"
      class="absolute bottom-4 left-1/2 -translate-x-1/2 cursor-pointer whitespace-nowrap rounded-lg bg-primary px-4 py-2 text-sm font-medium text-white shadow-lg hover:bg-primary/90"
    >
      {#if firstVisibleDate}
        <span class="opacity-70">{firstVisibleDate}</span>
        <span class="mx-1.5 opacity-40">|</span>
      {/if}
      {hasNewMessages ? 'New messages' : 'Jump to Present'}
      <span class="ml-1 iconify inline-block align-middle uil--arrow-down"></span>
    </button>
  {/if}
</div>
