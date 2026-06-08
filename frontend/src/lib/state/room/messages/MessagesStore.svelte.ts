import { tick } from 'svelte';
import { on } from 'svelte/events';
import { SvelteSet } from 'svelte/reactivity';
import type { Client } from '@urql/svelte';
import { useFragment } from '$lib/gql';
import {
  RoomEventViewFragmentDoc,
  type RoomEventViewFragment
} from '$lib/gql/graphql';
import type { EventEnvelope } from '$lib/eventBus.svelte';
import type { GraphQLClient } from '$lib/state/server/graphqlClient.svelte';
import type { JumpToMessageState } from '../composerContext.svelte';
import {
  PAGE_SIZE,
  RefetchOneQuery,
  RoomAfterQuery,
  RoomAroundQuery,
  RoomBeforeQuery,
  RoomLatestQuery,
  ThreadEventsQuery
} from './queries';
import { isRootRoomEvent, isThreadEvent } from './filters';
import {
  type EventConnectionPage,
  type RawEvent,
  getActorId,
  threadRepliesConnection,
  unmask
} from './helpers';

type MessageScope = 'room' | 'thread';

/**
 * Minimum hidden duration before a visibility->visible transition counts as
 * a "tab resume" worth catching up for. Mirrors the eventBus visibility-
 * resubscribe threshold and the GraphQL client's suspend-detector window
 * so all three layers react on the same horizon.
 */
const TAB_RESUME_GAP_MS = 30_000;

/**
 * Message store for both the main room timeline and a single thread pane.
 * The scope-specific methods (`setRoom` / `setThread`) choose which Core
 * GraphQL connection backs the list while the lifecycle, pagination, refetch,
 * and subscription ingestion behavior stays shared.
 */
export class MessagesStore {
  events = $state<RoomEventViewFragment[]>([]);
  isInitialLoading = $state(true);
  isLoadingMore = $state(false);
  hasReachedStart = $state(false);

  private readonly client: Client;
  private scope: MessageScope | null = null;
  private threadRootEventId = '';
  private seenIds: SvelteSet<string> = new SvelteSet<string>();
  private roomId = '';
  private oldestCursor: string | undefined;
  private newestCursor: string | undefined;

  /** Increments on every load kickoff. Async callbacks compare against
   *  it via {@link isStale} to discard results from superseded loads. */
  #loadId = 0;

  #disposeLifecycle: (() => void) | null = null;

  constructor(
    private readonly gqlClient: GraphQLClient,
    private readonly getCurrentUserId: () => string | null
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
        console.debug('[MessagesStore] reconnectCount %d -> %d, catching up', prev, n);
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
            '[MessagesStore] visible after %ds hidden -> catching up',
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

  /** Root-level events only (excludes thread replies). */
  get rootEvents(): RoomEventViewFragment[] {
    return this.events.filter(isRootRoomEvent);
  }

  /** Events that belong to this thread (root + replies). */
  get threadEvents(): RoomEventViewFragment[] {
    return this.events.filter((e) => isThreadEvent(e, this.roomId, this.threadRootEventId));
  }

  /** Allocate a new load id; pair with {@link isStale} in async callbacks. */
  private startLoad(): number {
    return ++this.#loadId;
  }

  /** True if a newer load has started; caller should discard its result. */
  private isStale(thisLoad: number): boolean {
    return this.#loadId !== thisLoad;
  }

  setRoom(roomId: string): void {
    this.scope = 'room';
    this.roomId = roomId;
    this.threadRootEventId = '';
    this.resetAndFetchLatest();
  }

  setThread(roomId: string, threadRootEventId: string): void {
    this.scope = 'thread';
    this.roomId = roomId;
    this.threadRootEventId = threadRootEventId;

    const thisLoad = this.startLoad();
    this.resetState();
    this.isInitialLoading = true;
    this.fetchThread(thisLoad);
  }

  /**
   * Route a space event into the store. Handles common message-list
   * mutations inline and delegates room/thread-specific MessagePostedEvent
   * handling to the current scope.
   */
  ingestServerEvent(serverEvent: EventEnvelope): void {
    const eventData = serverEvent.event;
    if (!eventData) return;
    // Subscription and historical-query payloads share the same Event
    // envelope. Cast once at the room boundary so downstream code can keep
    // using the RoomEventViewFragment shape it renders with.
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
    const eventRoomId =
      'roomId' in eventData
        ? eventData.roomId
        : 'processingRoomId' in eventData
          ? eventData.processingRoomId
          : null;
    if (eventRoomId != null && eventRoomId !== this.roomId) return;

    if (eventData.__typename === 'MessageRetractedEvent') {
      this.applyDeletion(eventData.messageEventId);
      return;
    }

    if (eventData.__typename === 'MessageEditedEvent') {
      this.applyEdit(eventData.messageEventId, eventData);
      return;
    }

    if (
      eventData.__typename === 'ReactionAddedEvent' ||
      eventData.__typename === 'ReactionRemovedEvent'
    ) {
      this.refetchByMessageEventId(eventData.messageEventId);
      return;
    }

    if (
      eventData.__typename === 'VideoProcessingCompletedEvent' ||
      eventData.__typename === 'AssetProcessingStartedEvent' ||
      eventData.__typename === 'AssetProcessingSucceededEvent' ||
      eventData.__typename === 'AssetProcessingFailedEvent'
    ) {
      if (!eventData.processingMessageEventId) return;
      this.refetchByMessageEventId(eventData.processingMessageEventId);
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

  async loadMore(): Promise<void> {
    if (this.isLoadingMore || this.hasReachedStart || !this.oldestCursor) return;

    const before = this.oldestCursor;
    this.isLoadingMore = true;

    try {
      const page = await this.fetchOlderPage(before);
      if (!page) return;

      const olderEvents = unmask(page.events);
      if (olderEvents.length === 0) {
        this.hasReachedStart = true;
      } else {
        if (page.startCursor) {
          this.oldestCursor = page.startCursor;
        }
        const added = this.prependEvents(olderEvents);
        this.afterOlderPagePrepended();
        if (added === 0) this.hasReachedStart = true;
      }

      if (!page.hasOlder) this.hasReachedStart = true;
    } catch (error) {
      console.error('MessagesStore: loadMore failed:', error);
    } finally {
      // Yield a frame so the virtualizer can settle before another loadMore.
      await tick();
      await new Promise((r) => requestAnimationFrame(r));
      this.isLoadingMore = false;
    }
  }

  private catchUp(): void {
    if (!this.scope || !this.roomId) return;
    if (this.scope === 'thread' && !this.threadRootEventId) return;

    const thisLoad = this.startLoad();
    if (this.events.length === 0) {
      this.fetchInitial(thisLoad);
    } else {
      this.catchUpForward(thisLoad);
    }
  }

  async refetchAll(): Promise<void> {
    const snapshot = this.scope === 'thread' ? [...this.threadEvents] : [...this.rootEvents];
    for (const event of snapshot) {
      await this.refetchOne(event.id);
    }
  }

  private async fetchOlderPage(before: string): Promise<EventConnectionPage | null> {
    if (this.scope === 'thread') {
      const result = await this.client
        .query(ThreadEventsQuery, {
          roomId: this.roomId,
          threadRootEventId: this.threadRootEventId,
          limit: PAGE_SIZE,
          before
        })
        .toPromise();

      return threadRepliesConnection(result.data?.room?.event);
    }

    const result = await this.client
      .query(RoomBeforeQuery, {
        roomId: this.roomId,
        limit: PAGE_SIZE,
        before
      })
      .toPromise();

    return result.data?.room?.events ?? null;
  }

  private afterOlderPagePrepended(): void {
    if (this.scope === 'thread') {
      this.sortThreadEvents();
    }
  }

  async loadNewer(jumpState: JumpToMessageState): Promise<void> {
    if (this.scope !== 'room') return;
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
      console.error('MessagesStore: loadNewer failed:', error);
    } finally {
      jumpState.isLoadingNewer = false;
    }
  }

  async jumpToMessage(eventId: string, jumpState: JumpToMessageState): Promise<void> {
    if (this.scope !== 'room') return;
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
        if (result.error) console.error('MessagesStore: jumpToMessage failed:', result.error);
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

  jumpToPresent(jumpState: JumpToMessageState): void {
    if (this.scope !== 'room') return;
    jumpState.reset();
    this.resetAndFetchLatest();
  }

  private onMessagePosted(
    spaceEvent: RoomEventViewFragment,
    eventData: Extract<RoomEventViewFragment['event'], { __typename: 'MessagePostedEvent' }>
  ): void {
    if (this.scope === 'thread') {
      if (eventData.threadRootEventId === this.threadRootEventId) {
        this.addEvent(spaceEvent);
        this.sortThreadEvents();
      }
      return;
    }

    // Thread replies don't enter the room timeline; instead, update
    // metadata on the root message (replyCount, lastReplyAt, participants,
    // viewerIsFollowingThread auto-follow).
    if (eventData.threadRootEventId) {
      this.applyThreadReplyToRoot(spaceEvent, eventData);
      return;
    }
    this.addEvent(spaceEvent);
  }

  private onSystemEvent(spaceEvent: RoomEventViewFragment): void {
    if (this.scope === 'room') {
      this.addEvent(spaceEvent);
    }
  }

  private async refetchOne(eventId: string): Promise<void> {
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

  private async refetchByMessageEventId(messageEventId: string): Promise<void> {
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
   * Apply a deletion locally. Direct echo retractions hide only the echo
   * artifact; original-message retractions tombstone the original and any
   * visible echoes that point at it.
   * Reactions and reply metadata are left intact so the tombstone row keeps
   * its existing engagement visible alongside the placeholder.
   */
  private applyDeletion(messageEventId: string): void {
    const targetIndex = this.events.findIndex((e) => e.id === messageEventId);
    const target = targetIndex === -1 ? null : this.events[targetIndex];
    const targetPayload = target?.event;
    if (
      targetPayload?.__typename === 'MessagePostedEvent' &&
      targetPayload.echoOfEventId
    ) {
      this.events.splice(targetIndex, 1);
      return;
    }

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
  private applyEdit(
    messageEventId: string,
    edit: Extract<EventEnvelope['event'], { __typename: 'MessageEditedEvent' }>
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

  private addEvent(event: RoomEventViewFragment): boolean {
    if (this.seenIds.has(event.id)) return false;
    this.seenIds.add(event.id);
    this.events.push(event);
    return true;
  }

  private appendMany(events: RoomEventViewFragment[]): void {
    for (const e of events) this.addEvent(e);
  }

  private prependEvents(olderEvents: RoomEventViewFragment[]): number {
    const newOnes = olderEvents.filter((e) => !this.seenIds.has(e.id));
    for (const e of newOnes) this.seenIds.add(e.id);
    this.events.unshift(...newOnes);
    return newOnes.length;
  }

  /**
   * Replace the buffer with fetched events but preserve any subscription
   * events that arrived during the in-flight query. Always the right
   * choice when a paginated query result replaces the timeline: the
   * eventBus subscription has been live since layout mount, so any
   * MessagePostedEvent for this room that lands while the query is in
   * flight has already been added to {@link events} via
   * {@link ingestServerEvent} and must not be wiped by the result.
   */
  private replaceMergingExisting(rawEvents: readonly RawEvent[]): void {
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

  private resetState(): void {
    this.events = [];
    this.seenIds = new SvelteSet();
    this.oldestCursor = undefined;
    this.newestCursor = undefined;
    this.hasReachedStart = false;
    this.isLoadingMore = false;
  }

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

  private resetAndFetchLatest(): void {
    const thisLoad = this.startLoad();
    this.resetState();
    this.isInitialLoading = true;
    this.fetchLatest(thisLoad);
  }

  private fetchInitial(thisLoad: number): void {
    if (this.scope === 'thread') {
      this.fetchThread(thisLoad);
    } else {
      this.fetchLatest(thisLoad);
    }
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
        if (result.error) console.error('MessagesStore: fetchLatest error:', result.error);
        const page = result.data?.room?.events;
        if (page) {
          this.replaceWithFetchedAndUpdateCursors(page);
          this.hasReachedStart = !page.hasOlder;
        }
        this.isInitialLoading = false;
      })
      .catch((error: unknown) => {
        if (this.isStale(thisLoad)) return;
        console.error('MessagesStore: fetchLatest failed:', error);
        this.isInitialLoading = false;
      });
  }

  private fetchThread(thisLoad: number): void {
    this.client
      .query(ThreadEventsQuery, {
        roomId: this.roomId,
        threadRootEventId: this.threadRootEventId,
        limit: PAGE_SIZE
      })
      .toPromise()
      .then((result) => {
        if (this.isStale(thisLoad)) return;
        if (result.error) console.error('MessagesStore: fetchThread error:', result.error);
        const root = result.data?.room?.event;
        if (root) {
          // Merge with any subscription events that arrived during the
          // in-flight query (e.g. the user's own reply or a fast cross-user
          // reply). Overwriting would drop them.
          const page = threadRepliesConnection(root);
          const replies = page?.events ?? [];
          this.replaceMergingExisting([root, ...replies]);
          this.sortThreadEvents();
          this.oldestCursor = page?.startCursor ?? undefined;
          this.newestCursor = page?.endCursor ?? undefined;
          this.hasReachedStart = !(page?.hasOlder ?? false);
        }
        this.isInitialLoading = false;
      })
      .catch((error: unknown) => {
        if (this.isStale(thisLoad)) return;
        console.error('MessagesStore: fetchThread failed:', error);
        this.isInitialLoading = false;
      });
  }

  private catchUpForward(thisLoad: number): void {
    if (!this.newestCursor) {
      this.fetchInitial(thisLoad);
      return;
    }

    if (this.scope === 'thread') {
      this.catchUpThreadForward(thisLoad, this.newestCursor);
    } else {
      this.catchUpRoomForward(thisLoad, this.newestCursor);
    }
  }

  private catchUpRoomForward(thisLoad: number, after: string): void {
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
          console.error('MessagesStore: room catchUp error:', result.error);
          return;
        }
        const page = result.data?.room?.events;
        if (!page) return;

        const fetched = unmask(page.events);
        const strategy = page.hasNewer ? 'replace' : 'append';
        console.debug(
          '[MessagesStore] catchUpForward: roomId=%s after=%s fetched=%d hasNewer=%s strategy=%s',
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
        console.error('MessagesStore: room catchUp failed:', error);
      });
  }

  private catchUpThreadForward(thisLoad: number, after: string): void {
    this.client
      .query(ThreadEventsQuery, {
        roomId: this.roomId,
        threadRootEventId: this.threadRootEventId,
        limit: PAGE_SIZE,
        after
      })
      .toPromise()
      .then((result) => {
        if (this.isStale(thisLoad)) return;
        if (result.error) {
          console.error('MessagesStore: thread catchUp error:', result.error);
          return;
        }

        const page = threadRepliesConnection(result.data?.room?.event);
        if (!page) return;

        const newerReplies = unmask(page.events);
        if (page.endCursor) {
          this.newestCursor = page.endCursor;
        }

        this.appendMany(newerReplies);
        this.sortThreadEvents();

        if (page.hasNewer) {
          this.fetchThread(thisLoad);
        }
      })
      .catch((error: unknown) => {
        if (this.isStale(thisLoad)) return;
        console.error('MessagesStore: thread catchUp failed:', error);
      });
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

  private sortThreadEvents(): void {
    this.events = [...this.events].sort((a, b) => {
      if (a.id === this.threadRootEventId) return -1;
      if (b.id === this.threadRootEventId) return 1;

      const aTime = Date.parse(a.createdAt);
      const bTime = Date.parse(b.createdAt);
      if (aTime !== bTime) return aTime - bTime;
      return a.id.localeCompare(b.id);
    });
  }
}
