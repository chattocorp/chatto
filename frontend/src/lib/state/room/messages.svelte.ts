import { tick } from 'svelte';
import { on } from 'svelte/events';
import { SvelteSet } from 'svelte/reactivity';
import type { Client } from '@urql/svelte';
import { graphql, useFragment } from '$lib/gql';
import {
  RoomEventViewFragmentDoc,
  type RoomEventViewFragment
} from '$lib/gql/graphql';
import type { FragmentType } from '$lib/gql/fragment-masking';
import type { ServerEvent } from '$lib/eventBus.svelte';
import type { GraphQLClient } from '$lib/state/server/graphqlClient.svelte';
import type { JumpToMessageState } from './composerContext.svelte';

/**
 * Minimum hidden duration before a visibility→visible transition counts as
 * a "tab resume" worth catching up for. Mirrors the eventBus visibility-
 * resubscribe threshold and the GraphQL client's suspend-detector window
 * so all three layers react on the same horizon.
 */
const TAB_RESUME_GAP_MS = 30_000;

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
 *   - **its own lifecycle wiring**: a reconnect listener on
 *     `gqlClient.reconnectCount` and a tab-resume-after-gap listener on
 *     `visibilitychange`, both feeding into {@link catchUp}
 *
 * Subclasses fill in:
 *   - the initial query (room: paginated; thread: single fetch)
 *   - the visible-events filter for {@link refetchAll}
 *   - {@link onMessagePosted} — what to do with a new MessagePostedEvent
 *   - {@link onSystemEvent} — what to do with room system events (default ignore)
 *   - {@link catchUp} — silent refetch of newer events triggered by reconnect
 *     or tab resume
 *
 * The component owns the actual subscription (via `useEvent`) and
 * forwards events here. Cross-cutting side effects (e.g. cancelling an
 * in-progress edit, removing a typing indicator) stay in the component.
 *
 * **Disposal:** callers MUST call {@link dispose} on unmount to tear down
 * the reconnect and visibility listeners.
 */
export abstract class MessageListStore {
  events = $state<RoomEventViewFragment[]>([]);
  isInitialLoading = $state(true);

  protected readonly client: Client;
  protected seenIds: SvelteSet<string> = new SvelteSet<string>();
  protected roomId = '';

  /** Increments on every load kickoff. Async callbacks compare against
   *  it via {@link isStale} to discard results from superseded loads. */
  #loadId = 0;

  #disposeLifecycle: (() => void) | null = null;

  /** Allocate a new load id; pair with {@link isStale} in async callbacks. */
  protected startLoad(): number {
    return ++this.#loadId;
  }

  /** True if a newer load has started — caller should discard its result. */
  protected isStale(thisLoad: number): boolean {
    return this.#loadId !== thisLoad;
  }

  constructor(
    protected readonly gqlClient: GraphQLClient,
    protected readonly getCurrentUserId: () => string | null
  ) {
    this.client = gqlClient.client;
    this.#disposeLifecycle = $effect.root(() => {
      // Reactive: re-run when reconnectCount changes, fire catchUp on
      // genuine increments.
      let lastSeen = this.gqlClient.reconnectCount;
      $effect(() => {
        const n = this.gqlClient.reconnectCount;
        if (n <= lastSeen) return;
        const prev = lastSeen;
        lastSeen = n;
        console.debug(
          '[MessageListStore] reconnectCount %d → %d, catching up',
          prev,
          n
        );
        this.catchUp();
      });

      // Non-reactive: register a document visibilitychange listener and
      // let $effect.root tear it down via the returned cleanup.
      if (typeof document === 'undefined') return;
      let lastVisibleAt = Date.now();
      return on(document, 'visibilitychange', () => {
        if (document.visibilityState !== 'visible') {
          lastVisibleAt = Date.now();
          return;
        }
        const gap = Date.now() - lastVisibleAt;
        lastVisibleAt = Date.now();
        if (gap > TAB_RESUME_GAP_MS) {
          console.debug(
            '[MessageListStore] visible after %ds hidden → catching up',
            Math.round(gap / 1000)
          );
          this.catchUp();
        }
      });
    });
  }

  /** Tear down lifecycle listeners. Idempotent. */
  dispose(): void {
    this.#disposeLifecycle?.();
    this.#disposeLifecycle = null;
  }

  // -------------------------------------------------------------------------
  // Catch-up hook (called by reconnect / tab-resume listeners)
  // -------------------------------------------------------------------------

  /**
   * Silent refetch of newer events. Called by the reconnect / tab-resume
   * listeners wired in the constructor (the trigger reason is logged by
   * the base class before this is invoked). Subclasses implement the
   * actual fetch strategy (paginated forward query for rooms, single-
   * shot thread fetch for threads). Must NOT toggle
   * {@link isInitialLoading} — catch-up should keep the stale view on
   * screen until the new data lands.
   */
  protected abstract catchUp(): void;

  // -------------------------------------------------------------------------
  // Subscription event ingestion
  // -------------------------------------------------------------------------

  /**
   * Route a space event into the store. Handles all common message-list
   * mutations: inline edit/retract, refetch on reaction/video, full reset on
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
    if (
      eventData.__typename === 'MessageRetractedEvent' ||
      eventData.__typename === 'MessageDeletedEvent'
    ) {
      this.applyDeletion(eventData.messageEventId);
      return;
    }

    if (eventData.__typename === 'MessageEditedEvent') {
      this.applyEdit(eventData.messageEventId, eventData);
      return;
    }

    if (
      eventData.__typename === 'MessageUpdatedEvent' ||
      eventData.__typename === 'ReactionAddedEvent' ||
      eventData.__typename === 'ReactionRemovedEvent' ||
      eventData.__typename === 'VideoProcessingCompletedEvent' ||
      eventData.__typename === 'AssetProcessingStartedEvent' ||
      eventData.__typename === 'AssetProcessingSucceededEvent' ||
      eventData.__typename === 'AssetProcessingFailedEvent'
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

  /**
   * Apply an edit payload directly to the matching MessagePostedEvent. The
   * backend emits one canonical edit event per linked post/echo, so we only
   * patch the direct event ID here; the linked event will arrive separately.
   */
  protected applyEdit(
    messageEventId: string,
    edit: Extract<ServerEvent['event'], { __typename: 'MessageEditedEvent' }>
  ): void {
    for (let i = 0; i < this.events.length; i++) {
      const e = this.events[i];
      const evt = e.event;
      if (evt?.__typename !== 'MessagePostedEvent') continue;
      if (e.id !== messageEventId) continue;

      this.events[i] = {
        ...e,
        event: {
          ...evt,
          body: edit.body,
          attachments: edit.attachments,
          linkPreview: edit.linkPreview,
          updatedAt: edit.updatedAt
        }
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

  /** Root-level events only (excludes thread replies). */
  get rootEvents(): RoomEventViewFragment[] {
    return this.events.filter(isRootRoomEvent);
  }

  // -------------------------------------------------------------------------
  // Lifecycle
  // -------------------------------------------------------------------------

  /**
   * Switch to a room (or force-refetch the current one). Always shows the
   * skeleton and clears state. Silent reconnect / tab-resume catch-ups go
   * through {@link catchUp} (driven internally by the base class), not
   * through this method.
   */
  setRoom(roomId: string): void {
    this.roomId = roomId;
    this.resetAndFetchLatest();
  }

  // -------------------------------------------------------------------------
  // Catch-up (called by base class on reconnect / tab-resume)
  // -------------------------------------------------------------------------

  protected catchUp(): void {
    if (!this.roomId) return;
    const thisLoad = this.startLoad();
    if (this.events.length === 0) {
      this.fetchLatest(thisLoad);
    } else {
      this.catchUpForward(thisLoad);
    }
  }

  /** Shared by {@link setRoom} and {@link jumpToPresent}: clear state, show
   *  the skeleton, kick off a fresh fetchLatest under a new load id. */
  private resetAndFetchLatest(): void {
    const thisLoad = this.startLoad();
    this.resetState();
    this.isInitialLoading = true;
    this.fetchLatest(thisLoad);
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
    this.resetAndFetchLatest();
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
        if (this.isStale(thisLoad)) return;
        if (result.error) console.error('RoomMessagesStore: fetchLatest error:', result.error);
        const page = result.data?.room?.events;
        if (page) {
          this.replaceWithFetchedAndUpdateCursors(page);
          this.hasReachedStart = !page.hasOlder;
        }
        this.isInitialLoading = false;
      })
      .catch((error: unknown) => {
        if (this.isStale(thisLoad)) return;
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

    const after = this.newestCursor;
    this.client
      .query(RoomAfterQuery, {
        roomId: this.roomId,
        limit: PAGE_SIZE,
        after
      })
      .toPromise()
      .then((result) => {
        if (this.isStale(thisLoad)) return;
        if (result.error) {
          console.error('RoomMessagesStore: catchUp error:', result.error);
          return;
        }
        const page = result.data?.room?.events;
        if (!page) return;

        const fetched = unmask(page.events);
        const strategy = page.hasNewer ? 'replace' : 'append';
        console.debug(
          '[RoomMessagesStore] catchUpForward: roomId=%s after=%s fetched=%d hasNewer=%s strategy=%s',
          this.roomId,
          after,
          fetched.length,
          page.hasNewer,
          strategy
        );
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
        if (this.isStale(thisLoad)) return;
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

  /** Events that belong to this thread (root + replies). */
  get threadEvents(): RoomEventViewFragment[] {
    return this.events.filter((e) => isThreadEvent(e, this.roomId, this.threadRootEventId));
  }

  /** Switch to a thread (or force-refetch the current one). Always shows
   *  the skeleton. Silent reconnect / tab-resume catch-ups go through
   *  {@link catchUp}, not through this method.
   */
  setThread(roomId: string, threadRootEventId: string): void {
    this.roomId = roomId;
    this.threadRootEventId = threadRootEventId;

    const thisLoad = this.startLoad();
    this.resetState();
    this.isInitialLoading = true;
    this.fetchThread(thisLoad);
  }

  protected catchUp(): void {
    if (!this.threadRootEventId) return;
    // Silent: do NOT flip isInitialLoading. fetchThread merges results
    // with existing events via replaceMergingExisting, so the stale view
    // stays on screen until the new data lands.
    this.fetchThread(this.startLoad());
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
        if (this.isStale(thisLoad)) return;
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
        if (this.isStale(thisLoad)) return;
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
    case 'MessageEditedEvent':
    case 'MessageRetractedEvent':
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
