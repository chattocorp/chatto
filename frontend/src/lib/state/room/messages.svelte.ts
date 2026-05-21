import { tick } from 'svelte';
import { SvelteSet } from 'svelte/reactivity';
import type { Client } from '@urql/svelte';
import { graphql, useFragment } from '$lib/gql';
import {
  RoomEventViewFragmentDoc,
  type RoomEventViewFragment
} from '$lib/gql/graphql';
import type { FragmentType } from '$lib/gql/fragment-masking';
import type { ServerEvent } from '$lib/eventBus.svelte';
import type { JumpToMessageState } from './composerContext.svelte';

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

const RoomLatestQuery = graphql(`
  query RoomMessagesLatest($roomId: ID!, $limit: Int) {
    room(roomId: $roomId) {
      events(limit: $limit) {
        events { ...RoomEventView }
        startCursor
        endCursor
        hasOlder
        hasNewer
      }
    }
  }
`);

const RoomBeforeQuery = graphql(`
  query RoomMessagesBefore($roomId: ID!, $limit: Int, $before: String) {
    room(roomId: $roomId) {
      events(limit: $limit, before: $before) {
        events { ...RoomEventView }
        startCursor
        endCursor
        hasOlder
        hasNewer
      }
    }
  }
`);

const RoomAfterQuery = graphql(`
  query RoomMessagesAfter($roomId: ID!, $limit: Int, $after: String) {
    room(roomId: $roomId) {
      events(limit: $limit, after: $after) {
        events { ...RoomEventView }
        startCursor
        endCursor
        hasOlder
        hasNewer
      }
    }
  }
`);

const RoomAroundQuery = graphql(`
  query RoomMessagesAround($roomId: ID!, $eventId: ID!, $limit: Int) {
    room(roomId: $roomId) {
      eventsAround(eventId: $eventId, limit: $limit) {
        events { ...RoomEventView }
        targetIndex
        startCursor
        endCursor
        hasOlder
        hasNewer
      }
    }
  }
`);

const RefetchOneQuery = graphql(`
  query RoomMessagesRefetchOne($roomId: ID!, $eventId: ID!) {
    room(roomId: $roomId) {
      event(eventId: $eventId) {
        ...RoomEventView
      }
    }
  }
`);

const ThreadEventsQuery = graphql(`
  query ThreadMessagesAll($roomId: ID!, $threadRootEventId: ID!) {
    room(roomId: $roomId) {
      event(eventId: $threadRootEventId) {
        ...RoomEventView
        threadReplies {
          ...RoomEventView
        }
      }
    }
  }
`);

const PAGE_SIZE = 50;

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type RawEvent = FragmentType<typeof RoomEventViewFragmentDoc>;

function unmask(raw: readonly RawEvent[]): RoomEventViewFragment[] {
  return raw
    .map((e) => useFragment(RoomEventViewFragmentDoc, e))
    .filter((e): e is RoomEventViewFragment => e !== null);
}

function getActorId(actor: RoomEventViewFragment['actor']): string | undefined {
  return actor ? (actor as { id?: string }).id : undefined;
}

// ---------------------------------------------------------------------------
// MessageListStore — base class shared by room and thread variants
// ---------------------------------------------------------------------------

/**
 * Reactive store for a list of room events. The base class owns:
 *   - the events buffer and dedup set
 *   - list mutation primitives (add, prepend, replace, reset)
 *   - per-event refetch by id or messageEventId (reactions, edits, deletes,
 *     video processing all funnel through here)
 *   - the {@link ingestServerEvent} skeleton, which routes incoming
 *     subscription events to refetches or to subclass hooks
 *
 * Subclasses fill in:
 *   - the initial query (room: paginated; thread: single fetch)
 *   - the visible-events filter for {@link refetchAll}
 *   - {@link onMessagePosted} — what to do with a new MessagePostedEvent
 *   - {@link onSystemEvent} — what to do with room system events (default ignore)
 *
 * The component owns the actual subscription (via `useEvent`) and
 * forwards events here. Cross-cutting side effects (e.g. cancelling an
 * in-progress edit, removing a typing indicator) stay in the component.
 */
export abstract class MessageListStore {
  events = $state<RoomEventViewFragment[]>([]);
  isInitialLoading = $state(true);

  protected seenIds: SvelteSet<string> = new SvelteSet<string>();
  protected roomId = '';

  constructor(
    protected readonly client: Client,
    protected readonly getCurrentUserId: () => string | null
  ) {}

  // -------------------------------------------------------------------------
  // Subscription event ingestion
  // -------------------------------------------------------------------------

  /**
   * Route a space event into the store. Handles all common message-list
   * mutations: refetch on edit/delete/reaction/video, full reset on
   * RoomDeletedEvent, full refetch on ServerMemberDeletedEvent. Delegates
   * MessagePostedEvent and room system events to subclass hooks.
   */
  ingestServerEvent(serverEvent: ServerEvent): void {
    const eventData = serverEvent.event;
    if (!eventData) return;
    // The subscription envelope (ServerEvent) and the historical-query
    // envelope (RoomEventView) describe identical fields for the room-
    // event union members the store cares about — the only TypeScript
    // difference is the wrapper's broader union of event types. Cast
    // once at the boundary so the downstream code can keep using the
    // RoomEventViewFragment shape it was written against.
    const spaceEvent = serverEvent as unknown as RoomEventViewFragment;

    if (eventData.__typename === 'ServerMemberDeletedEvent') {
      this.refetchAll();
      return;
    }

    if (eventData.__typename === 'RoomDeletedEvent') {
      if (eventData.roomId === this.roomId) this.resetState();
      return;
    }

    // From here on, only events scoped to this room are interesting.
    if ('roomId' in eventData && eventData.roomId !== this.roomId) return;

    // Apply deletions locally — the post-deletion state is deterministic
    // (body=null, attachments=[]) and we already know the affected event id,
    // so a server round-trip is wasteful and a refetch failure would leave
    // the original message visible on screen.
    if (eventData.__typename === 'MessageDeletedEvent') {
      this.applyDeletion(eventData.messageEventId);
      return;
    }

    if (
      eventData.__typename === 'MessageUpdatedEvent' ||
      eventData.__typename === 'ReactionAddedEvent' ||
      eventData.__typename === 'ReactionRemovedEvent' ||
      eventData.__typename === 'VideoProcessingCompletedEvent'
    ) {
      this.refetchByMessageEventId(eventData.messageEventId);
      return;
    }

    if (eventData.__typename === 'MessagePostedEvent') {
      this.onMessagePosted(spaceEvent, eventData);
      return;
    }

    if (
      eventData.__typename === 'UserJoinedRoomEvent' ||
      eventData.__typename === 'UserLeftRoomEvent' ||
      eventData.__typename === 'RoomUpdatedEvent' ||
      eventData.__typename === 'RoomArchivedEvent' ||
      eventData.__typename === 'RoomUnarchivedEvent'
    ) {
      this.onSystemEvent(spaceEvent);
    }
  }

  /** Refetch every visible event. Used when a space member is deleted. */
  abstract refetchAll(): Promise<void>;

  // -------------------------------------------------------------------------
  // Subclass hooks
  // -------------------------------------------------------------------------

  protected abstract onMessagePosted(
    spaceEvent: RoomEventViewFragment,
    eventData: Extract<RoomEventViewFragment['event'], { __typename: 'MessagePostedEvent' }>
  ): void;

  /** Default: ignore room system events. RoomMessagesStore overrides to add. */
  protected onSystemEvent(_spaceEvent: RoomEventViewFragment): void {
    // intentionally empty
  }

  // -------------------------------------------------------------------------
  // Refetch primitives
  // -------------------------------------------------------------------------

  protected async refetchOne(eventId: string): Promise<void> {
    const result = await this.client
      .query(
        RefetchOneQuery,
        { roomId: this.roomId, eventId },
        { requestPolicy: 'network-only' }
      )
      .toPromise();

    const fetched = result.data?.room?.event;
    if (!fetched) return;
    const updated = useFragment(RoomEventViewFragmentDoc, fetched);
    if (!updated) return;
    const idx = this.events.findIndex((e) => e.id === eventId);
    if (idx !== -1) this.events[idx] = updated;
  }

  protected async refetchByMessageEventId(messageEventId: string): Promise<void> {
    // Match either the direct event id or an echo whose original points here.
    for (const e of this.events) {
      const evt = e.event;
      if (
        e.id === messageEventId ||
        (evt?.__typename === 'MessagePostedEvent' && evt.echoOfEventId === messageEventId)
      ) {
        await this.refetchOne(e.id);
      }
    }
  }

  /**
   * Apply a deletion locally to any matching MessagePostedEvent in the buffer
   * (the original by id, plus any echo whose echoOfEventId points at it).
   * Mirrors the server's post-deletion state: body=null, attachments=[].
   * Reactions and reply metadata are left intact so the tombstone row keeps
   * its existing engagement visible alongside the placeholder.
   */
  protected applyDeletion(messageEventId: string): void {
    for (let i = 0; i < this.events.length; i++) {
      const e = this.events[i];
      const evt = e.event;
      if (evt?.__typename !== 'MessagePostedEvent') continue;
      if (e.id !== messageEventId && evt.echoOfEventId !== messageEventId) continue;

      this.events[i] = {
        ...e,
        event: { ...evt, body: null, attachments: [] }
      };
    }
  }

  // -------------------------------------------------------------------------
  // List mutation primitives
  // -------------------------------------------------------------------------

  protected addEvent(event: RoomEventViewFragment): boolean {
    if (this.seenIds.has(event.id)) return false;
    this.seenIds.add(event.id);
    this.events.push(event);
    return true;
  }

  protected appendMany(events: RoomEventViewFragment[]): void {
    for (const e of events) this.addEvent(e);
  }

  protected prependEvents(olderEvents: RoomEventViewFragment[]): number {
    const newOnes = olderEvents.filter((e) => !this.seenIds.has(e.id));
    for (const e of newOnes) this.seenIds.add(e.id);
    this.events.unshift(...newOnes);
    return newOnes.length;
  }

  /**
   * Replace the buffer with fetched events but preserve any subscription
   * events that arrived during the in-flight query. Always the right
   * choice when a paginated query result replaces the timeline — the
   * eventBus subscription has been live since layout mount, so any
   * MessagePostedEvent for this room that lands while the query is in
   * flight has already been added to {@link events} via
   * {@link ingestServerEvent} and must not be wiped by the result.
   */
  protected replaceMergingExisting(rawEvents: readonly RawEvent[]): void {
    const fetched = unmask(rawEvents);
    const newSeen = new SvelteSet<string>();
    const merged: RoomEventViewFragment[] = [];
    for (const e of fetched) {
      if (newSeen.has(e.id)) continue;
      newSeen.add(e.id);
      merged.push(e);
    }
    for (const e of this.events) {
      if (newSeen.has(e.id)) continue;
      newSeen.add(e.id);
      merged.push(e);
    }
    this.events = merged;
    this.seenIds = newSeen;
  }

  protected resetState(): void {
    this.events = [];
    this.seenIds = new SvelteSet();
  }
}

// ---------------------------------------------------------------------------
// RoomMessagesStore — the room view (paginated, supports jumped mode)
// ---------------------------------------------------------------------------

/**
 * Message store for a room's main timeline. Adds pagination, jumped-mode
 * navigation (jump-to-message + load-newer + jump-to-present), reconnect
 * catch-up, root-event filtering, and thread-reply metadata fan-out.
 */
export class RoomMessagesStore extends MessageListStore {
  isLoadingMore = $state(false);
  hasReachedStart = $state(false);

  /**
   * Opaque pagination cursors returned by the GraphQL `Room.events` query.
   * `oldestCursor` anchors backward pagination; `newestCursor` anchors
   * forward pagination and reconnect catch-up. Subscription-delivered
   * events do not carry a cursor and therefore do not update `newestCursor`
   * — that's fine, because catch-up's worst case is "fetch some events
   * we've already seen" which `appendMany` dedupes by ID.
   */
  private oldestCursor: string | undefined;
  private newestCursor: string | undefined;
  /** Increments on every setRoom or jumpToPresent — guards async callbacks. */
  private loadId = 0;

  /** Root-level events only (excludes thread replies). */
  get rootEvents(): RoomEventViewFragment[] {
    return this.events.filter(isRootRoomEvent);
  }

  // -------------------------------------------------------------------------
  // Lifecycle
  // -------------------------------------------------------------------------

  /**
   * Switch to a different room (or refetch the current one).
   *
   * @param mode 'reset' clears state and shows skeleton; 'catchUp' keeps
   *   stale events visible and quietly fetches forward (use on reconnect).
   */
  setRoom(roomId: string, mode: 'reset' | 'catchUp'): void {
    const isSameRoom = this.roomId === roomId;
    this.roomId = roomId;

    const thisLoad = ++this.loadId;

    if (mode === 'reset' || !isSameRoom) {
      this.resetState();
      this.isInitialLoading = true;
      this.fetchLatest(thisLoad);
      return;
    }

    if (this.events.length === 0) {
      this.fetchLatest(thisLoad);
      return;
    }
    this.catchUpForward(thisLoad);
  }

  // -------------------------------------------------------------------------
  // Pagination
  // -------------------------------------------------------------------------

  async loadMore(): Promise<void> {
    if (this.isLoadingMore || this.hasReachedStart || !this.oldestCursor) return;

    const cursor = this.oldestCursor;
    this.isLoadingMore = true;

    try {
      const result = await this.client
        .query(RoomBeforeQuery, {
          roomId: this.roomId,
          limit: PAGE_SIZE,
          before: cursor
        })
        .toPromise();

      const page = result.data?.room?.events;
      if (!page) return;

      const olderEvents = unmask(page.events);

      if (olderEvents.length === 0) {
        this.hasReachedStart = true;
      } else {
        // Advance the backward cursor to the start of this page.
        if (page.startCursor) {
          this.oldestCursor = page.startCursor;
        }
        const added = this.prependEvents(olderEvents);
        if (added === 0) this.hasReachedStart = true;
      }

      if (!page.hasOlder) this.hasReachedStart = true;
    } catch (error) {
      console.error('RoomMessagesStore: loadMore failed:', error);
    } finally {
      // Yield a frame so the virtualizer can settle before another loadMore
      await tick();
      await new Promise((r) => requestAnimationFrame(r));
      this.isLoadingMore = false;
    }
  }

  /**
   * Forward pagination — only meaningful in jumped mode (i.e. when the local
   * timeline doesn't include the latest events). Updates {@link jumpState} to
   * reflect end-of-history.
   */
  async loadNewer(jumpState: JumpToMessageState): Promise<void> {
    if (jumpState.isLoadingNewer || jumpState.hasReachedEnd) return;
    if (!this.newestCursor) return;

    jumpState.isLoadingNewer = true;
    try {
      const result = await this.client
        .query(RoomAfterQuery, {
          roomId: this.roomId,
          limit: PAGE_SIZE,
          after: this.newestCursor
        })
        .toPromise();

      // User left jumped mode while in flight — abandon the result.
      if (!jumpState.isJumpedMode) return;

      const page = result.data?.room?.events;
      if (!page) return;

      const newer = unmask(page.events);
      if (newer.length === 0) {
        jumpState.hasReachedEnd = true;
      } else {
        if (page.endCursor) {
          this.newestCursor = page.endCursor;
        }
        this.appendMany(newer);
      }

      if (!page.hasNewer) jumpState.hasReachedEnd = true;
    } catch (error) {
      console.error('RoomMessagesStore: loadNewer failed:', error);
    } finally {
      jumpState.isLoadingNewer = false;
    }
  }

  // -------------------------------------------------------------------------
  // Jump to message
  // -------------------------------------------------------------------------

  async jumpToMessage(eventId: string, jumpState: JumpToMessageState): Promise<void> {
    if (this.events.some((e) => e.id === eventId)) {
      jumpState.scrollToEventId = eventId;
      return;
    }

    this.isInitialLoading = true;
    try {
      const result = await this.client
        .query(RoomAroundQuery, {
          roomId: this.roomId,
          eventId,
          limit: PAGE_SIZE
        })
        .toPromise();

      const around = result.data?.room?.eventsAround;
      if (result.error || !around) {
        if (result.error) console.error('RoomMessagesStore: jumpToMessage failed:', result.error);
        return;
      }

      const { events: rawEvents, hasOlder, hasNewer, startCursor, endCursor } = around;
      const parsed = unmask(rawEvents);

      this.events = [...parsed];
      this.seenIds = new SvelteSet(parsed.map((e) => e.id));
      this.oldestCursor = startCursor ?? undefined;
      this.newestCursor = endCursor ?? undefined;
      this.hasReachedStart = !hasOlder;

      // Only enter jumped mode when newer messages exist beyond this window.
      jumpState.isJumpedMode = hasNewer;
      jumpState.hasReachedEnd = !hasNewer;
      jumpState.hasOlderMessages = hasOlder;
      jumpState.scrollToEventId = eventId;
    } finally {
      this.isInitialLoading = false;
    }
  }

  /** Exit jumped mode and refetch the latest events. */
  jumpToPresent(jumpState: JumpToMessageState): void {
    jumpState.reset();
    const thisLoad = ++this.loadId;
    this.resetState();
    this.isInitialLoading = true;
    this.fetchLatest(thisLoad);
  }

  // -------------------------------------------------------------------------
  // Refetch all visible events (override)
  // -------------------------------------------------------------------------

  async refetchAll(): Promise<void> {
    const snapshot = [...this.rootEvents];
    for (const event of snapshot) {
      await this.refetchOne(event.id);
    }
  }

  // -------------------------------------------------------------------------
  // Subscription hooks (override)
  // -------------------------------------------------------------------------

  protected onMessagePosted(
    spaceEvent: RoomEventViewFragment,
    eventData: Extract<RoomEventViewFragment['event'], { __typename: 'MessagePostedEvent' }>
  ): void {
    // Thread replies don't enter the room timeline; instead, update
    // metadata on the root message (replyCount, lastReplyAt, participants,
    // viewerIsFollowingThread auto-follow).
    if (eventData.threadRootEventId) {
      this.applyThreadReplyToRoot(spaceEvent, eventData);
      return;
    }
    this.addEvent(spaceEvent);
  }

  protected onSystemEvent(spaceEvent: RoomEventViewFragment): void {
    this.addEvent(spaceEvent);
  }

  // -------------------------------------------------------------------------
  // Private — fetch
  // -------------------------------------------------------------------------

  protected resetState(): void {
    super.resetState();
    this.oldestCursor = undefined;
    this.newestCursor = undefined;
    this.hasReachedStart = false;
  }

  private fetchLatest(thisLoad: number): void {
    this.client
      .query(RoomLatestQuery, {
        roomId: this.roomId,
        limit: PAGE_SIZE
      })
      .toPromise()
      .then((result) => {
        if (this.loadId !== thisLoad) return;
        if (result.error) console.error('RoomMessagesStore: fetchLatest error:', result.error);
        const page = result.data?.room?.events;
        if (page) {
          this.replaceWithFetchedAndUpdateCursors(page);
          this.hasReachedStart = !page.hasOlder;
        }
        this.isInitialLoading = false;
      })
      .catch((error: unknown) => {
        if (this.loadId !== thisLoad) return;
        console.error('RoomMessagesStore: fetchLatest failed:', error);
        this.isInitialLoading = false;
      });
  }

  /**
   * Reconnect catch-up: fetch only events newer than what we already have.
   * If the gap is larger than a page (server reports hasNewer), replace the
   * timeline to avoid holes.
   *
   * Uses `newestCursor` (last cursor returned by a query) rather than
   * scanning local events for a max timestamp. Subscription-delivered events
   * arrived after `newestCursor` was set, so this re-fetches them — but
   * `appendMany` dedupes by ID so the cost is duplicate network bytes, not
   * duplicate UI items.
   */
  private catchUpForward(thisLoad: number): void {
    if (!this.newestCursor) {
      this.fetchLatest(thisLoad);
      return;
    }

    this.client
      .query(RoomAfterQuery, {
        roomId: this.roomId,
        limit: PAGE_SIZE,
        after: this.newestCursor
      })
      .toPromise()
      .then((result) => {
        if (this.loadId !== thisLoad) return;
        if (result.error) {
          console.error('RoomMessagesStore: catchUp error:', result.error);
          return;
        }
        const page = result.data?.room?.events;
        if (!page) return;

        const fetched = unmask(page.events);
        if (page.hasNewer) {
          this.replaceWithFetchedAndUpdateCursors(page);
        } else {
          if (page.endCursor) {
            this.newestCursor = page.endCursor;
          }
          this.appendMany(fetched);
        }
      })
      .catch((error: unknown) => {
        if (this.loadId !== thisLoad) return;
        console.error('RoomMessagesStore: catchUp failed:', error);
      });
  }

  /** Replace via merge (preserves mid-query subscription events) and update cursors. */
  private replaceWithFetchedAndUpdateCursors(connection: {
    events: readonly RawEvent[];
    startCursor?: string | null;
    endCursor?: string | null;
  }): void {
    this.replaceMergingExisting(connection.events);
    this.oldestCursor = connection.startCursor ?? undefined;
    this.newestCursor = connection.endCursor ?? undefined;
    this.hasReachedStart = false;
  }

  /**
   * Mirror the backend's auto-follow behavior on the root message when a
   * thread reply arrives, so the UI updates instantly without refetching.
   */
  private applyThreadReplyToRoot(
    spaceEvent: RoomEventViewFragment,
    eventData: Extract<RoomEventViewFragment['event'], { __typename: 'MessagePostedEvent' }>
  ): void {
    const rootIdx = this.events.findIndex((e) => e.id === eventData.threadRootEventId);
    if (rootIdx === -1) return;

    const rootEvent = this.events[rootIdx];
    if (rootEvent.event?.__typename !== 'MessagePostedEvent') return;

    const actorId = getActorId(spaceEvent.actor);
    const existingParticipants = rootEvent.event.threadParticipants;
    const isNewParticipant =
      !!actorId && !existingParticipants.some((p) => getActorId(p) === actorId);

    const isFirstReply = rootEvent.event.replyCount === 0;
    const currentUserId = this.getCurrentUserId();
    const viewerIsRootAuthor = currentUserId !== null && rootEvent.actorId === currentUserId;
    const viewerIsReplier = currentUserId !== null && actorId === currentUserId;
    const viewerIsFollowingThread =
      viewerIsReplier || (isFirstReply && viewerIsRootAuthor)
        ? true
        : rootEvent.event.viewerIsFollowingThread;

    this.events[rootIdx] = {
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

// ---------------------------------------------------------------------------
// ThreadMessagesStore — a single thread's view
// ---------------------------------------------------------------------------

/**
 * Message store for a single thread. Loads root + replies in one query,
 * doesn't paginate, and only accepts MessagePostedEvents that target this
 * thread. System events (joined/left/etc.) are ignored.
 */
export class ThreadMessagesStore extends MessageListStore {
  private threadRootEventId = '';
  /** Increments on every setThread — guards async callbacks. */
  private loadId = 0;

  /** Events that belong to this thread (root + replies). */
  get threadEvents(): RoomEventViewFragment[] {
    return this.events.filter((e) => isThreadEvent(e, this.roomId, this.threadRootEventId));
  }

  /** Switch to a different thread and (re)fetch its events. */
  setThread(roomId: string, threadRootEventId: string): void {
    this.roomId = roomId;
    this.threadRootEventId = threadRootEventId;

    const thisLoad = ++this.loadId;
    this.resetState();
    this.isInitialLoading = true;
    this.fetchThread(thisLoad);
  }

  async refetchAll(): Promise<void> {
    const snapshot = [...this.threadEvents];
    for (const event of snapshot) {
      await this.refetchOne(event.id);
    }
  }

  // -------------------------------------------------------------------------
  // Subscription hooks (override)
  // -------------------------------------------------------------------------

  protected onMessagePosted(
    spaceEvent: RoomEventViewFragment,
    eventData: Extract<RoomEventViewFragment['event'], { __typename: 'MessagePostedEvent' }>
  ): void {
    if (eventData.threadRootEventId === this.threadRootEventId) {
      this.addEvent(spaceEvent);
    }
  }

  // System events are ignored — base class default is fine.

  // -------------------------------------------------------------------------
  // Private — fetch
  // -------------------------------------------------------------------------

  private fetchThread(thisLoad: number): void {
    this.client
      .query(ThreadEventsQuery, {
        roomId: this.roomId,
        threadRootEventId: this.threadRootEventId
      })
      .toPromise()
      .then((result) => {
        if (this.loadId !== thisLoad) return;
        if (result.error) console.error('ThreadMessagesStore: fetch error:', result.error);
        const root = result.data?.room?.event;
        if (root) {
          // Merge with any subscription events that arrived during the
          // in-flight query (e.g. the user's own reply or a fast cross-user
          // reply). Overwriting would drop them.
          this.replaceMergingExisting([root, ...root.threadReplies]);
        }
        this.isInitialLoading = false;
      })
      .catch((error: unknown) => {
        if (this.loadId !== thisLoad) return;
        console.error('ThreadMessagesStore: fetch failed:', error);
        this.isInitialLoading = false;
      });
  }
}

// ---------------------------------------------------------------------------
// Filtering helpers — exported so callers can match the store's view.
// ---------------------------------------------------------------------------

export function isRootRoomEvent(event: RoomEventViewFragment): boolean {
  const eventData = event.event;
  if (!eventData) return false;
  switch (eventData.__typename) {
    case 'MessagePostedEvent':
      // Echoes are root-level; thread replies (threadRootEventId set) are not.
      return !!eventData.echoOfEventId || !eventData.threadRootEventId;
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

export function isThreadEvent(
  event: RoomEventViewFragment,
  roomId: string,
  threadRootEventId: string
): boolean {
  const eventData = event.event;
  if (!eventData || !('roomId' in eventData) || eventData.roomId !== roomId) return false;
  // Thread view only shows messages, not system events.
  if (eventData.__typename !== 'MessagePostedEvent') return false;
  if (event.id === threadRootEventId) return true;
  return eventData.threadRootEventId === threadRootEventId;
}
