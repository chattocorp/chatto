<script lang="ts">
  import { tick } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import { graphql, useFragment } from '$lib/gql';
  import type { FragmentType } from '$lib/gql/fragment-masking';
  import { RoomEventViewFragmentDoc, type RoomEventViewFragment } from '$lib/gql/graphql';
  import { useSpaceEvent, useReconnectTrigger } from '$lib/hooks';
  import { useConnection } from '$lib/state/instance/connection.svelte';
  import { getComposerContext, type RoomMember } from '$lib/state/room';
  import { getCurrentUser } from '$lib/auth/currentUser.svelte';
  import EventList from './EventList.svelte';

  let {
    spaceId,
    roomId,
    unreadAfterTime = null,
    unreadBeforeTime = null,
    onOpenThread,
    typingUserIds = [],
    typingMembers = []
  }: {
    spaceId: string;
    roomId: string;
    unreadAfterTime?: string | null;
    unreadBeforeTime?: string | null;
    onOpenThread?: (threadRootEventId: string, highlightEventId?: string) => void;
    typingUserIds?: string[];
    typingMembers?: RoomMember[];
  } = $props();

  // Resolve time-based unread boundary to an event ID for EventList
  let unreadAfterEventId = $derived.by(() => {
    if (unreadAfterTime === null) return null;
    const afterMs = new Date(unreadAfterTime).getTime();
    const beforeMs = unreadBeforeTime ? new Date(unreadBeforeTime).getTime() : Infinity;
    for (const event of roomEvents) {
      const eventMs = new Date(event.createdAt).getTime();
      if (eventMs > afterMs && eventMs <= beforeMs) {
        return event.id;
      }
    }
    return null;
  });

  const connection = useConnection();
  const composerContext = getComposerContext();
  const editState = composerContext.editState;
  const jumpState = composerContext.jumpState;
  const currentUser = getCurrentUser();

  // --- Local state (replaces SpaceMessageCache) ---
  let events = $state<RoomEventViewFragment[]>([]);
  let seenIds = new SvelteSet<string>();
  let oldestTime = $state<Date | undefined>(undefined);
  let newestTime = $state<Date | undefined>(undefined); // for jumped mode

  function getEventKey(event: RoomEventViewFragment): string {
    return event.id;
  }

  function resetLocalState() {
    events = [];
    seenIds = new SvelteSet();
    oldestTime = undefined;
    newestTime = undefined;
    hasReachedStart = false;
  }

  // Filter: only root messages and room system events (not thread replies)
  function isRootRoomEvent(event: RoomEventViewFragment): boolean {
    const eventData = event.event;
    if (!eventData) return false;

    switch (eventData.__typename) {
      case 'MessagePostedEvent':
        // Echoes (echoOfEventId set) are root-level; thread replies (inThread set) are not
        return !!eventData.echoOfEventId || !eventData.inThread;
      case 'MessageUpdatedEvent':
      case 'MessageDeletedEvent':
      case 'UserJoinedRoomEvent':
      case 'UserLeftRoomEvent':
      case 'RoomUpdatedEvent':
      case 'RoomDeletedEvent':
      case 'RoomArchivedEvent':
      case 'RoomUnarchivedEvent':
        return true;
      default:
        return false;
    }
  }

  let roomEvents = $derived(events.filter(isRootRoomEvent));

  // Track updates for scroll triggering
  let updateCounter = $derived(roomEvents.length);

  let isLoadingMore = $state(false);
  let isInitialLoading = $state(true);
  let hasReachedStart = $state(false);
  let refetchTrigger = $state(0);

  // Track current load to handle race conditions when room changes mid-load
  let loadId = { current: 0 };

  let earliestCreatedAt = $derived(oldestTime ?? null);

  /** Add an event with dedup, appending to the end. Returns true if added. */
  function addEvent(event: RoomEventViewFragment): boolean {
    const key = getEventKey(event);
    if (seenIds.has(key)) return false;
    seenIds.add(key);
    events.push(event);
    return true;
  }

  /** Prepend older events (for pagination). Returns count of new events. */
  function prependEvents(olderEvents: RoomEventViewFragment[]): number {
    const newEvents = olderEvents.filter((e) => !seenIds.has(getEventKey(e)));
    for (const e of newEvents) seenIds.add(getEventKey(e));
    events.unshift(...newEvents);
    return newEvents.length;
  }

  async function loadMoreMessages() {
    if (isLoadingMore || hasReachedStart || earliestCreatedAt === null) {
      return;
    }

    const cursor = earliestCreatedAt.toISOString();
    isLoadingMore = true;

    try {
      const result = await connection().client
        .query(
          graphql(`
            query RoomEventsQueryPaginated(
              $spaceId: ID!
              $roomId: ID!
              $limit: Int
              $before: Time
            ) {
              roomEvents(spaceId: $spaceId, roomId: $roomId, limit: $limit, before: $before) {
                events {
                  ...RoomEventView
                }
                hasOlder
                hasNewer
              }
            }
          `),
          { spaceId, roomId, limit: 50, before: cursor }
        )
        .toPromise();

      if (result.data?.roomEvents) {
        const olderEvents = result.data.roomEvents.events
          .map((event) => useFragment(RoomEventViewFragmentDoc, event))
          .filter((e): e is RoomEventViewFragment => e !== null);

        if (olderEvents.length === 0) {
          hasReachedStart = true;
        } else {
          // Update oldest time
          for (const e of olderEvents) {
            const t = e.createdAt ? new Date(e.createdAt) : null;
            if (t && (!oldestTime || t < oldestTime)) oldestTime = t;
          }

          const newEventsAdded = prependEvents(olderEvents);

          if (newEventsAdded === 0) {
            hasReachedStart = true;
          }
        }

        if (!result.data.roomEvents.hasOlder) {
          hasReachedStart = true;
        }
      }
    } catch (error) {
      console.error('Failed to load more messages:', error);
    } finally {
      await tick();
      await new Promise((resolve) => requestAnimationFrame(resolve));
      isLoadingMore = false;
    }
  }

  // --- Jump to message ---

  /** Jump to a specific message by event ID. */
  async function jumpToMessage(eventId: string) {
    // Check if the event is already in the local timeline
    if (events.some((e) => e.id === eventId)) {
      if (jumpState) {
        jumpState.scrollToEventId = eventId;
      }
      return;
    }

    // Fetch events around the target
    isInitialLoading = true;
    try {
      const result = await connection().client
        .query(
          graphql(`
            query RoomEventsAroundQuery($spaceId: ID!, $roomId: ID!, $eventId: ID!, $limit: Int) {
              roomEventsAround(
                spaceId: $spaceId
                roomId: $roomId
                eventId: $eventId
                limit: $limit
              ) {
                events {
                  ...RoomEventView
                }
                targetIndex
                hasOlder
                hasNewer
              }
            }
          `),
          { spaceId, roomId, eventId, limit: 50 }
        )
        .toPromise();

      if (result.error) {
        console.error('Failed to jump to message:', result.error);
        return;
      }

      if (result.data?.roomEventsAround) {
        const { events: rawEvents, hasOlder, hasNewer } = result.data.roomEventsAround;
        const parsed = rawEvents
          .map((e) => useFragment(RoomEventViewFragmentDoc, e))
          .filter((e): e is RoomEventViewFragment => e !== null);

        // Compute time bounds
        let oldest: Date | undefined;
        let newest: Date | undefined;
        for (const e of parsed) {
          const t = e.createdAt ? new Date(e.createdAt) : null;
          if (t) {
            if (!oldest || t < oldest) oldest = t;
            if (!newest || t > newest) newest = t;
          }
        }

        // Replace local events with the jumped window
        events = [...parsed];
        seenIds = new SvelteSet(parsed.map(getEventKey));
        oldestTime = oldest;
        newestTime = newest;

        // Update state
        hasReachedStart = !hasOlder;

        if (jumpState) {
          // Only enter jumped mode if there are newer messages beyond the window.
          // If we're already at the end of the conversation, there's no need for
          // the "Jump to Present" overlay — the user IS at the present.
          jumpState.isJumpedMode = hasNewer;
          jumpState.hasReachedEnd = !hasNewer;
          jumpState.hasOlderMessages = hasOlder;
          jumpState.scrollToEventId = eventId;
        }
      }
    } catch (error) {
      console.error('Failed to jump to message:', error);
    } finally {
      isInitialLoading = false;
    }
  }

  /** Load newer messages (forward pagination in jumped mode). */
  async function loadNewerMessages() {
    if (!jumpState || jumpState.isLoadingNewer || jumpState.hasReachedEnd) return;
    if (!newestTime) return;

    jumpState.isLoadingNewer = true;
    try {
      const result = await connection().client
        .query(
          graphql(`
            query RoomEventsQueryForward($spaceId: ID!, $roomId: ID!, $limit: Int, $after: Time) {
              roomEvents(spaceId: $spaceId, roomId: $roomId, limit: $limit, after: $after) {
                events {
                  ...RoomEventView
                }
                hasOlder
                hasNewer
              }
            }
          `),
          { spaceId, roomId, limit: 50, after: newestTime.toISOString() }
        )
        .toPromise();

      // Bail if we left jumped mode while the request was in-flight
      if (!jumpState?.isJumpedMode) return;

      if (result.data?.roomEvents) {
        const newerEvents = result.data.roomEvents.events
          .map((e) => useFragment(RoomEventViewFragmentDoc, e))
          .filter((e): e is RoomEventViewFragment => e !== null);

        if (newerEvents.length === 0) {
          jumpState.hasReachedEnd = true;
        } else {
          // Compute newest time
          let newest: Date | undefined;
          for (const e of newerEvents) {
            const t = e.createdAt ? new Date(e.createdAt) : null;
            if (t && (!newest || t > newest)) newest = t;
          }

          // Append with dedup
          const newEvents = newerEvents.filter((e) => !seenIds.has(getEventKey(e)));
          for (const e of newEvents) seenIds.add(getEventKey(e));
          events.push(...newEvents);
          if (newest) newestTime = newest;
        }

        if (!result.data.roomEvents.hasNewer) {
          jumpState.hasReachedEnd = true;
        }
      }

    } catch (error) {
      console.error('Failed to load newer messages:', error);
    } finally {
      if (jumpState) {
        jumpState.isLoadingNewer = false;
      }
    }
  }

  /** Return to the latest messages (exit jumped mode). */
  function jumpToPresent() {
    if (jumpState) {
      jumpState.reset();
    }
    resetLocalState();
    refetchTrigger++;
  }

  // Register handlers with the jump context so child components can trigger jumps
  if (jumpState) {
    jumpState.setJumpHandler(jumpToMessage);
    jumpState.setJumpToPresentHandler(jumpToPresent);
    jumpState.setLoadNewerHandler(loadNewerMessages);
  }

  // Reset jump state when room changes
  $effect(() => {
    void roomId;
    if (jumpState) {
      jumpState.reset();
    }
  });


  const reconnect = useReconnectTrigger();

  // Track previous values to distinguish room changes from reconnects
  let prevRoomId: string | undefined;
  let prevRefetchTrigger: number | undefined;

  function fetchRoomEvents(showLoading: boolean) {
    const thisLoadId = ++loadId.current;
    const currentRoomId = roomId;

    if (showLoading) {
      resetLocalState();
      isLoadingMore = false;
      isInitialLoading = true;
    }

    // On reconnect with existing events, try to catch up incrementally
    // instead of replacing everything.
    if (!showLoading && events.length > 0) {
      catchUpMissedEvents(thisLoadId, currentRoomId);
      return;
    }

    fetchLatestEvents(thisLoadId, currentRoomId);
  }

  /** Fetch the latest events for the room (initial load or full reset). */
  function fetchLatestEvents(thisLoadId: number, currentRoomId: string) {
    connection().client
      .query(
        graphql(`
          query RoomEventsQuery($spaceId: ID!, $roomId: ID!, $limit: Int) {
            roomEvents(spaceId: $spaceId, roomId: $roomId, limit: $limit) {
              events {
                ...RoomEventView
              }
              hasOlder
              hasNewer
            }
          }
        `),
        { spaceId, roomId: currentRoomId, limit: 50 }
      )
      .toPromise()
      .then((result) => {
        if (loadId.current !== thisLoadId) return;

        if (result.error) {
          console.error('Failed to load room events:', result.error);
        }

        if (result.data?.roomEvents) {
          replaceEvents(result.data.roomEvents.events);
          hasReachedStart = !result.data.roomEvents.hasOlder;
        }
        isInitialLoading = false;
      })
      .catch((error: unknown) => {
        if (loadId.current !== thisLoadId) return;
        console.error('Room events query failed:', error);
        isInitialLoading = false;
      });
  }

  /**
   * On reconnect, fetch only the events we missed since the last known event.
   * If the gap is small (hasNewer is false), append seamlessly. If the server
   * reports more events exist beyond the page, replace the timeline to avoid holes.
   */
  function catchUpMissedEvents(thisLoadId: number, currentRoomId: string) {
    // Find the newest createdAt from our current events
    let newest: Date | undefined;
    for (const e of events) {
      const t = e.createdAt ? new Date(e.createdAt) : null;
      if (t && (!newest || t > newest)) newest = t;
    }

    // No timestamp to anchor from — fall back to full fetch
    if (!newest) {
      fetchLatestEvents(thisLoadId, currentRoomId);
      return;
    }

    connection().client
      .query(
        graphql(`
          query RoomEventsCatchUp($spaceId: ID!, $roomId: ID!, $limit: Int, $after: Time) {
            roomEvents(spaceId: $spaceId, roomId: $roomId, limit: $limit, after: $after) {
              events {
                ...RoomEventView
              }
              hasOlder
              hasNewer
            }
          }
        `),
        { spaceId, roomId: currentRoomId, limit: 50, after: newest.toISOString() }
      )
      .toPromise()
      .then((result) => {
        if (loadId.current !== thisLoadId) return;

        if (result.error) {
          console.error('Failed to catch up room events:', result.error);
          return;
        }

        if (!result.data?.roomEvents) return;

        const { events: rawEvents, hasNewer } = result.data.roomEvents;
        const fetched = rawEvents
          .map((event) => useFragment(RoomEventViewFragmentDoc, event))
          .filter((e): e is RoomEventViewFragment => e !== null);

        if (hasNewer) {
          // Gap is too large — server says more events exist beyond this page.
          // Replace timeline to avoid event holes.
          replaceEvents(rawEvents);
        } else {
          // Small gap — append missed events seamlessly
          const newEvents = fetched.filter((e) => !seenIds.has(getEventKey(e)));
          for (const e of newEvents) seenIds.add(getEventKey(e));
          events.push(...newEvents);
        }
      })
      .catch((error: unknown) => {
        if (loadId.current !== thisLoadId) return;
        console.error('Room events catch-up failed:', error);
      });
  }

  /** Replace all local events with a fresh set from the server. */
  function replaceEvents(rawEvents: readonly FragmentType<typeof RoomEventViewFragmentDoc>[]) {
    const fetched = rawEvents
      .map((event) => useFragment(RoomEventViewFragmentDoc, event))
      .filter((e): e is RoomEventViewFragment => e !== null);

    const newSeenIds = new SvelteSet<string>();
    let newOldestTime: Date | undefined;
    for (const e of fetched) {
      const key = getEventKey(e);
      newSeenIds.add(key);
      const t = e.createdAt ? new Date(e.createdAt) : null;
      if (t && (!newOldestTime || t < newOldestTime)) newOldestTime = t;
    }
    events = fetched;
    seenIds = newSeenIds;
    oldestTime = newOldestTime;
    newestTime = undefined;
    hasReachedStart = false;
  }

  // Load historical events when room changes, WebSocket reconnects, or jumpToPresent
  $effect(() => {
    void reconnect.count;
    void refetchTrigger;

    const isRoomChange = prevRoomId !== undefined && prevRoomId !== roomId;
    const isRefetchTrigger = prevRefetchTrigger !== undefined && prevRefetchTrigger !== refetchTrigger;
    const isFirstLoad = prevRoomId === undefined;

    prevRoomId = roomId;
    prevRefetchTrigger = refetchTrigger;

    // Show skeletons on first load, room change, or jumpToPresent.
    // On reconnect, keep stale messages visible and refetch silently.
    const showLoading = isFirstLoad || isRoomChange || isRefetchTrigger;
    fetchRoomEvents(showLoading);
  });

  // Refetch a message by event ID and replace it in the local array
  async function refetchMessage(targetSpaceId: string, targetRoomId: string, eventId: string) {
    const result = await connection().client
      .query(
        graphql(`
          query RefetchMessage($spaceId: ID!, $roomId: ID!, $eventId: ID!) {
            roomEventByEventId(spaceId: $spaceId, roomId: $roomId, eventId: $eventId) {
              ...RoomEventView
            }
          }
        `),
        { spaceId: targetSpaceId, roomId: targetRoomId, eventId },
        { requestPolicy: 'network-only' }
      )
      .toPromise();

    if (result.data?.roomEventByEventId) {
      const updatedEvent = useFragment(RoomEventViewFragmentDoc, result.data.roomEventByEventId);
      if (updatedEvent) {
        const idx = events.findIndex((e) => e.id === eventId);
        if (idx !== -1) {
          events[idx] = updatedEvent;
        }
      }
    }
  }

  // Refetch all visible events (used when a user is deleted to update message actors)
  async function refetchAllEvents() {
    const currentSpaceId = spaceId;
    const currentRoomId = roomId;
    const eventsSnapshot = [...roomEvents];

    for (const event of eventsSnapshot) {
      await refetchMessage(currentSpaceId, currentRoomId, event.id);
    }
  }

  // Handle live events for this room
  useSpaceEvent((spaceEvent) => {
    const eventData = spaceEvent.event;
    if (!eventData) return;

    // When a space member's account is deleted, refetch all visible messages
    if (eventData.__typename === 'SpaceMemberDeletedEvent') {
      refetchAllEvents();
      return;
    }

    // Clear edit state if the message being edited is deleted
    if (eventData.__typename === 'MessageDeletedEvent') {
      if (eventData.roomId !== roomId) return;
      if (editState.eventId === eventData.messageEventId) {
        editState.cancelEdit();
      }
    }

    // Handle room deletion - clear local state
    if (eventData.__typename === 'RoomDeletedEvent') {
      if (eventData.roomId === roomId) {
        resetLocalState();
      }
      return;
    }

    // Handle message edits/deletes - refetch the affected event(s)
    if (
      eventData.__typename === 'MessageUpdatedEvent' ||
      eventData.__typename === 'MessageDeletedEvent'
    ) {
      if (eventData.roomId !== roomId) return;
      for (const e of events) {
        const evt = e.event;
        if (
          e.id === eventData.messageEventId ||
          (evt?.__typename === 'MessagePostedEvent' &&
            evt.echoOfEventId === eventData.messageEventId)
        ) {
          refetchMessage(spaceId, roomId, e.id);
        }
      }
      return;
    }

    // Handle reaction events - refetch the reacted message.
    // Reactions are stored against the original message's event ID, so we also
    // match echoes (whose echoOfEventId points to the original).
    if (
      eventData.__typename === 'ReactionAddedEvent' ||
      eventData.__typename === 'ReactionRemovedEvent'
    ) {
      if (eventData.roomId !== roomId) return;
      for (const e of events) {
        const evt = e.event;
        if (
          e.id === eventData.messageEventId ||
          (evt?.__typename === 'MessagePostedEvent' &&
            evt.echoOfEventId === eventData.messageEventId)
        ) {
          refetchMessage(spaceId, roomId, e.id);
        }
      }
      return;
    }

    // Handle video processing completed - refetch affected events
    if (eventData.__typename === 'VideoProcessingCompletedEvent') {
      if (eventData.roomId !== roomId) return;
      for (const e of events) {
        const evt = e.event;
        if (
          e.id === eventData.messageEventId ||
          (evt?.__typename === 'MessagePostedEvent' &&
            evt.echoOfEventId === eventData.messageEventId)
        ) {
          refetchMessage(spaceId, roomId, e.id);
        }
      }
      return;
    }

    // Route displayable events to local state
    if (eventData.__typename === 'MessagePostedEvent') {
      if (eventData.roomId !== roomId) return;

      // Update thread metadata on root message when a reply arrives, but
      // don't add thread replies to the main room events — they belong in the thread pane.
      if (eventData.inThread) {
        const rootIdx = events.findIndex((e) => e.id === eventData.inThread);
        if (rootIdx !== -1) {
          const rootEvent = events[rootIdx];
          if (rootEvent.event?.__typename === 'MessagePostedEvent') {
            const getActorId = (actor: RoomEventViewFragment['actor']): string | undefined =>
              actor ? (actor as { id?: string }).id : undefined;

            const actorId = getActorId(spaceEvent.actor);
            const existingParticipants = rootEvent.event.threadParticipants;
            const isNewParticipant =
              actorId && !existingParticipants.some((p) => getActorId(p) === actorId);

            const isFirstReply = rootEvent.event.replyCount === 0;
            const viewerIsRootAuthor =
              currentUser.user?.id != null && rootEvent.actorId === currentUser.user.id;
            // The backend auto-follows both the replier (always) and the root
            // author (on first reply). Mirror that here for instant UI feedback.
            const viewerIsReplier =
              currentUser.user?.id != null && actorId === currentUser.user.id;
            const viewerIsFollowingThread =
              viewerIsReplier || (isFirstReply && viewerIsRootAuthor)
                ? true
                : rootEvent.event.viewerIsFollowingThread;

            events[rootIdx] = {
              ...rootEvent,
              event: {
                ...rootEvent.event,
                replyCount: rootEvent.event.replyCount + 1,
                lastReplyAt: spaceEvent.createdAt,
                viewerIsFollowingThread,
                threadParticipants:
                  isNewParticipant && spaceEvent.actor
                    ? [...existingParticipants, spaceEvent.actor]
                    : existingParticipants
              }
            };
          }
        }
        return;
      }

      addEvent(spaceEvent);
      return;
    }

    if (
      eventData.__typename === 'UserJoinedRoomEvent' ||
      eventData.__typename === 'UserLeftRoomEvent' ||
      eventData.__typename === 'RoomUpdatedEvent' ||
      eventData.__typename === 'RoomArchivedEvent' ||
      eventData.__typename === 'RoomUnarchivedEvent'
    ) {
      if (eventData.roomId !== roomId) return;
      addEvent(spaceEvent);
    }
  });
</script>

<EventList
  {spaceId}
  {roomId}
  events={roomEvents}
  alwaysScrollToBottom={false}
  showNewMessagesIndicator={true}
  enablePagination={true}
  {isLoadingMore}
  {hasReachedStart}
  onLoadMore={loadMoreMessages}
  {updateCounter}
  {onOpenThread}
  enableLastEditableFinder={true}
  isLoading={isInitialLoading}
  {unreadAfterEventId}
  {typingUserIds}
  {typingMembers}
  scrollToEventId={jumpState?.scrollToEventId ?? null}
  onScrollToEventComplete={() => {
    if (jumpState) jumpState.scrollToEventId = null;
  }}
  isJumpedMode={jumpState?.isJumpedMode ?? false}
  isLoadingNewer={jumpState?.isLoadingNewer ?? false}
  hasReachedEnd={jumpState?.hasReachedEnd ?? false}
  onLoadNewer={loadNewerMessages}
  onJumpToPresent={jumpToPresent}
/>
