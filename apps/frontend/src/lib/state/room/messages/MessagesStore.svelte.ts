import { tick } from 'svelte';
import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import type { Client } from '@urql/svelte';
import { useFragment } from '$lib/gql';
import {
  RoomEventViewFragmentDoc,
  type RoomEventViewFragment
} from '$lib/gql/graphql';
import type { EventEnvelope } from '$lib/eventBus.svelte';
import {
  createRoomTimelineAPI,
  type RoomTimelineAPI
} from '$lib/api/roomTimeline';
import type { GraphQLClient } from '$lib/state/server/graphqlClient.svelte';
import type { JumpToMessageState } from '../composerContext.svelte';
import {
  PAGE_SIZE,
  RefetchOneQuery,
  RoomAfterQuery,
  RoomAroundQuery,
  RoomBeforeQuery,
  RoomLatestQuery
} from './queries';
import { isRootRoomEvent, isThreadEvent } from './filters';
import {
  type EventConnectionPage,
  type RawEvent,
  getActorId,
  unmask
} from './helpers';

type MessageScope = 'room' | 'thread';

export type RefreshCurrentWindowResult = {
  hasOlder: boolean;
  hasNewer: boolean;
  refreshed: boolean;
};

function eventCacheKey(roomId: string, eventId: string): string {
  return `${roomId}\u0000${eventId}`;
}

function compareEventCreatedAt(a: RoomEventViewFragment, b: RoomEventViewFragment): number {
  return Date.parse(a.createdAt) - Date.parse(b.createdAt);
}

function roomTimelineFromGraphQLClient(gqlClient: GraphQLClient): RoomTimelineAPI | null {
  const candidate = gqlClient as {
    serverId?: string;
    connectBaseUrl?: string;
    bearerToken?: string | null;
  };
  if (!candidate.connectBaseUrl) return null;
  return createRoomTimelineAPI({
    serverId: candidate.serverId,
    baseUrl: candidate.connectBaseUrl,
    bearerToken: candidate.bearerToken ?? null
  });
}

/**
 * Message store for both the main room timeline and a single thread pane.
 * Room history now uses the protobuf ConnectRPC timeline API when available;
 * thread history requires that path. Single-event refetches still use GraphQL
 * during the migration. Lifecycle, pagination, refetch, and subscription
 * ingestion behavior stays shared across both scopes.
 */
export class MessagesStore {
  events = $state<RoomEventViewFragment[]>([]);
  isInitialLoading = $state(true);
  isLoadingMore = $state(false);
  hasReachedStart = $state(false);

  private readonly client: Client;
  private readonly roomTimeline: RoomTimelineAPI | null;
  private scope: MessageScope | null = null;
  private threadRootEventId = '';
  private seenIds: SvelteSet<string> = new SvelteSet<string>();
  private previewEvents = new SvelteMap<string, RoomEventViewFragment | null>();
  private pendingPreviewFetches = new SvelteMap<string, Promise<void>>();
  private roomId = '';
  private oldestCursor: string | undefined;
  private newestCursor: string | undefined;

  /** Increments on every load kickoff. Async callbacks compare against
   *  it via {@link isStale} to discard results from superseded loads. */
  #loadId = 0;

  constructor(
    gqlClient: GraphQLClient,
    private readonly getCurrentUserId: () => string | null,
    roomTimeline?: RoomTimelineAPI | null
  ) {
    this.client = gqlClient.client;
    this.roomTimeline = roomTimeline ?? roomTimelineFromGraphQLClient(gqlClient);
  }

  /** Tear down lifecycle listeners. Idempotent. */
  dispose(): void {
    // The message store has no owned subscriptions. Server-event replay is
    // managed by the singleton event bus.
  }

  /** Root-level events only (excludes thread replies). */
  get rootEvents(): RoomEventViewFragment[] {
    return this.events.filter(isRootRoomEvent);
  }

  /** Events that belong to this thread (root + replies). */
  get threadEvents(): RoomEventViewFragment[] {
    return this.events.filter((e) => isThreadEvent(e, this.roomId, this.threadRootEventId));
  }

  /** Look up an event already known to this room, including off-window preview targets. */
  getEventById(eventId: string): RoomEventViewFragment | null | undefined {
    return (
      this.events.find((e) => e.id === eventId) ?? this.previewEvents.get(this.previewKey(eventId))
    );
  }

  /** Find the visible event to anchor a refresh for a message mutation.
   * Mutations from channel echoes use the original message ID, while the
   * rendered room timeline contains the echo wrapper event.
   */
  refreshAnchorForMessageMutation(messageEventId: string): string | null {
    for (const event of this.events) {
      if (event.id === messageEventId) return event.id;
      const payload = event.event;
      if (payload?.__typename === 'MessagePostedEvent' && payload.echoOfEventId === messageEventId) {
        return event.id;
      }
      if (
        payload?.__typename === 'MessagePostedEvent' &&
        payload.channelEchoEventId === messageEventId
      ) {
        return event.id;
      }
    }

    for (const event of this.previewEvents.values()) {
      if (!event) continue;
      if (event.id === messageEventId) return event.id;
      const payload = event.event;
      if (payload?.__typename === 'MessagePostedEvent' && payload.echoOfEventId === messageEventId) {
        return event.id;
      }
      if (
        payload?.__typename === 'MessagePostedEvent' &&
        payload.channelEchoEventId === messageEventId
      ) {
        return event.id;
      }
    }

    return null;
  }

  /** Apply a successful local message delete without querying around a now-hidden echo. */
  applyLocalMessageDeletion(messageEventId: string): void {
    this.applyDeletion(messageEventId);
  }

  /** Update the viewer's thread follow state on a known thread root event. */
  setThreadRootFollowState(threadRootEventId: string, isFollowing: boolean): void {
    const idx = this.events.findIndex((e) => e.id === threadRootEventId);
    if (idx === -1) return;

    const rootEvent = this.events[idx];
    if (rootEvent.event?.__typename !== 'MessagePostedEvent') return;
    if (rootEvent.event.viewerIsFollowingThread === isFollowing) return;

    this.events[idx] = {
      ...rootEvent,
      event: {
        ...rootEvent.event,
        viewerIsFollowingThread: isFollowing
      }
    };
  }

  /** Fetch an off-window event for previews. Transient errors are not cached. */
  ensureEvent(eventId: string): Promise<void> | undefined {
    if (!this.roomId) return undefined;
    if (this.events.some((e) => e.id === eventId)) return undefined;

    const key = this.previewKey(eventId);
    if (this.previewEvents.has(key)) return undefined;

    const existing = this.pendingPreviewFetches.get(key);
    if (existing) return existing;

    const roomId = this.roomId;
    const promise = this.client
      .query(RefetchOneQuery, { roomId, eventId })
      .toPromise()
      .then((result) => {
        if (result.error) {
          console.error('MessagesStore: ensureEvent error:', result.error);
          return;
        }

        const fetched = result.data?.room?.event;
        const event = fetched ? useFragment(RoomEventViewFragmentDoc, fetched) : null;
        this.previewEvents.set(key, event ?? null);
      })
      .catch((error: unknown) => {
        console.error('MessagesStore: ensureEvent failed:', error);
      })
      .finally(() => {
        this.pendingPreviewFetches.delete(key);
      });

    this.pendingPreviewFetches.set(key, promise);
    return promise;
  }

  /** Allocate a new load id; pair with {@link isStale} in async callbacks. */
  private startLoad(): number {
    return ++this.#loadId;
  }

  /** True if a newer load has started; caller should discard its result. */
  private isStale(thisLoad: number): boolean {
    return this.#loadId !== thisLoad;
  }

  private previewKey(eventId: string): string {
    return eventCacheKey(this.roomId, eventId);
  }

  setRoom(roomId: string): void {
    if (this.scope === 'room' && this.roomId === roomId) return;

    this.scope = 'room';
    this.roomId = roomId;
    this.threadRootEventId = '';
    this.resetAndFetchLatest();
  }

  setThread(roomId: string, threadRootEventId: string): void {
    if (
      this.scope === 'thread' &&
      this.roomId === roomId &&
      this.threadRootEventId === threadRootEventId
    ) {
      return;
    }

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
    // Subscription and historical-query payloads share the same Event
    // envelope. Cast once at the room boundary so downstream code can keep
    // using the RoomEventViewFragment shape it renders with.
    const spaceEvent = serverEvent as unknown as RoomEventViewFragment;
    this.ingestEvent(spaceEvent);
  }

  /**
   * Route an already-renderable event into the store. Used for read-your-writes
   * after mutations that return the posted event; live subscription delivery
   * still follows {@link ingestServerEvent} and is deduped by event ID.
   */
  ingestEvent(spaceEvent: RoomEventViewFragment): void {
    const eventData = spaceEvent.event;
    if (!eventData) return;

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

  async refetchAll(): Promise<void> {
    const snapshot = this.scope === 'thread' ? [...this.threadEvents] : [...this.rootEvents];
    for (const event of snapshot) {
      await this.refetchOne(event.id);
    }
  }

  private async fetchOlderPage(before: string): Promise<EventConnectionPage | null> {
    if (this.scope === 'thread') {
      return this.requireRoomTimeline().getThreadEvents({
        roomId: this.roomId,
        threadRootEventId: this.threadRootEventId,
        limit: PAGE_SIZE,
        before
      });
    }

    if (this.roomTimeline) {
      return this.roomTimeline.getRoomEvents({
        roomId: this.roomId,
        limit: PAGE_SIZE,
        before
      });
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
      const page = this.roomTimeline
        ? await this.roomTimeline.getRoomEvents({
            roomId: this.roomId,
            limit: PAGE_SIZE,
            after: this.newestCursor
          })
        : await this.fetchRoomAfterViaGraphQL(this.newestCursor);

      // User left jumped mode while in flight — abandon the result.
      if (!jumpState.isJumpedMode) return;

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
      const around = this.roomTimeline
        ? await this.roomTimeline.getRoomEventsAround({
            roomId: this.roomId,
            eventId,
            limit: PAGE_SIZE
          })
        : await this.fetchRoomAroundViaGraphQL(eventId);

      if (!around) return;

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

  /**
   * Refresh the currently displayed message window from projected state without
   * clearing the buffer. Used after tab wake / reconnect when the client may
   * have missed subscription events.
   */
  async refreshCurrentWindow(anchorEventId?: string | null): Promise<RefreshCurrentWindowResult> {
    if (!this.scope || !this.roomId) return { hasOlder: false, hasNewer: false, refreshed: false };

    const thisLoad = this.startLoad();
    const existingBeforeFetch = new SvelteSet(this.events.map((e) => e.id));
    console.debug('[room-refresh] store refresh started', {
      roomId: this.roomId,
      scope: this.scope,
      anchorEventId: anchorEventId ?? null,
      existingCount: this.events.length
    });

    try {
      if (this.scope === 'thread') {
        const result = await this.refreshThreadWindow(
          thisLoad,
          existingBeforeFetch,
          anchorEventId ?? null
        );
        console.debug('[room-refresh] store refresh finished', {
          roomId: this.roomId,
          scope: this.scope,
          mode: anchorEventId ? 'thread-around' : 'thread-latest',
          anchorEventId: anchorEventId ?? null,
          result,
          eventCount: this.events.length
        });
        return result;
      }

      if (anchorEventId) {
        const refreshedAroundAnchor = await this.refreshRoomAround(
          thisLoad,
          anchorEventId,
          existingBeforeFetch
        );
        if (refreshedAroundAnchor) {
          console.debug('[room-refresh] store refresh finished', {
            roomId: this.roomId,
            scope: this.scope,
            mode: 'around',
            anchorEventId,
            result: refreshedAroundAnchor,
            eventCount: this.events.length
          });
          return refreshedAroundAnchor;
        }
        console.debug('[room-refresh] anchor refresh unavailable, falling back to latest', {
          roomId: this.roomId,
          anchorEventId
        });
      }

      const result = await this.refreshRoomLatest(thisLoad, existingBeforeFetch);
      console.debug('[room-refresh] store refresh finished', {
        roomId: this.roomId,
        scope: this.scope,
        mode: 'latest',
        result,
        eventCount: this.events.length
      });
      return result;
    } catch (error) {
      if (this.isStale(thisLoad)) return { hasOlder: false, hasNewer: false, refreshed: false };
      console.error('MessagesStore: refreshCurrentWindow failed:', error);
      return { hasOlder: false, hasNewer: false, refreshed: false };
    }
  }

  private onMessagePosted(
    spaceEvent: RoomEventViewFragment,
    eventData: Extract<RoomEventViewFragment['event'], { __typename: 'MessagePostedEvent' }>
  ): void {
    if (this.scope === 'thread') {
      if (
        eventData.echoOfEventId &&
        eventData.echoFromThreadRootEventId === this.threadRootEventId
      ) {
        this.applyChannelEchoLink(eventData.echoOfEventId, spaceEvent.id);
        return;
      }

      if (eventData.threadRootEventId === this.threadRootEventId) {
        this.addEvent(spaceEvent, { sortRoom: false });
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
    this.clearChannelEchoLink(messageEventId);

    const targetIndex = this.events.findIndex((e) => e.id === messageEventId);
    const target = targetIndex === -1 ? null : this.events[targetIndex];
    const targetPayload = target?.event;
    if (
      targetPayload?.__typename === 'MessagePostedEvent' &&
      targetPayload.echoOfEventId
    ) {
      this.events.splice(targetIndex, 1);
      this.seenIds.delete(messageEventId);
      return;
    }

    for (let i = 0; i < this.events.length; i++) {
      const e = this.events[i];
      const evt = e.event;
      if (evt?.__typename !== 'MessagePostedEvent') continue;
      if (e.id !== messageEventId && evt.echoOfEventId !== messageEventId) continue;

      this.events[i] = {
        ...e,
        event: { ...evt, body: null, attachments: [], linkPreview: null }
      };
    }

    const previewKey = this.previewKey(messageEventId);
    const preview = this.previewEvents.get(previewKey);
    if (preview?.event?.__typename === 'MessagePostedEvent') {
      this.previewEvents.set(previewKey, {
        ...preview,
        event: { ...preview.event, body: null, attachments: [], linkPreview: null }
      });
    }
  }

  private applyChannelEchoLink(originalEventId: string, echoEventId: string): void {
    for (let i = 0; i < this.events.length; i++) {
      const e = this.events[i];
      const evt = e.event;
      if (e.id !== originalEventId || evt?.__typename !== 'MessagePostedEvent') continue;
      this.events[i] = {
        ...e,
        event: { ...evt, channelEchoEventId: echoEventId }
      };
    }

    const previewKey = this.previewKey(originalEventId);
    const preview = this.previewEvents.get(previewKey);
    if (preview?.event?.__typename === 'MessagePostedEvent') {
      this.previewEvents.set(previewKey, {
        ...preview,
        event: { ...preview.event, channelEchoEventId: echoEventId }
      });
    }
  }

  private clearChannelEchoLink(echoEventId: string): void {
    for (let i = 0; i < this.events.length; i++) {
      const e = this.events[i];
      const evt = e.event;
      if (evt?.__typename !== 'MessagePostedEvent') continue;
      if (evt.channelEchoEventId !== echoEventId) continue;
      this.events[i] = {
        ...e,
        event: { ...evt, channelEchoEventId: null }
      };
    }

    for (const [key, preview] of this.previewEvents) {
      if (preview?.event?.__typename !== 'MessagePostedEvent') continue;
      if (preview.event.channelEchoEventId !== echoEventId) continue;
      this.previewEvents.set(key, {
        ...preview,
        event: { ...preview.event, channelEchoEventId: null }
      });
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

    const previewKey = this.previewKey(messageEventId);
    const preview = this.previewEvents.get(previewKey);
    if (preview?.event?.__typename === 'MessagePostedEvent') {
      this.previewEvents.set(previewKey, {
        ...preview,
        event: {
          ...preview.event,
          body: edit.body,
          attachments: edit.attachments,
          linkPreview: edit.linkPreview,
          updatedAt: edit.updatedAt
        }
      });
    }
  }

  private addEvent(
    event: RoomEventViewFragment,
    options: { sortRoom?: boolean } = {}
  ): boolean {
    if (this.seenIds.has(event.id)) return false;
    this.seenIds.add(event.id);
    this.events.push(event);
    if ((options.sortRoom ?? true) && this.scope === 'room') this.sortRoomEvents();
    return true;
  }

  private appendMany(events: RoomEventViewFragment[]): void {
    let added = false;
    for (const e of events) {
      added = this.addEvent(e, { sortRoom: false }) || added;
    }
    if (added && this.scope === 'room') this.sortRoomEvents();
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
    if (this.scope === 'room') this.sortRoomEvents();
    this.seenIds = newSeen;
  }

  private resetState(): void {
    this.events = [];
    this.seenIds = new SvelteSet();
    this.previewEvents.clear();
    this.pendingPreviewFetches.clear();
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

  private replaceWithSnapshotAndUpdateCursors(
    connection: {
      events: readonly RawEvent[];
      startCursor?: string | null;
      endCursor?: string | null;
      hasOlder?: boolean;
    },
    existingBeforeFetch: ReadonlySet<string>
  ): void {
    const fetched = unmask(connection.events);
    const newSeen = new SvelteSet<string>();
    const merged: RoomEventViewFragment[] = [];

    for (const e of fetched) {
      if (newSeen.has(e.id)) continue;
      newSeen.add(e.id);
      merged.push(e);
    }

    // Preserve subscription events that arrived while the refresh query was in
    // flight. Older pre-refresh rows outside the fetched window are deliberately
    // dropped: this is a projected-state reload of the current window.
    for (const e of this.events) {
      if (existingBeforeFetch.has(e.id) || newSeen.has(e.id)) continue;
      newSeen.add(e.id);
      merged.push(e);
    }

    this.events = merged;
    if (this.scope === 'room') this.sortRoomEvents();
    this.seenIds = newSeen;
    this.oldestCursor = connection.startCursor ?? undefined;
    this.newestCursor = connection.endCursor ?? undefined;
    this.hasReachedStart = !(connection.hasOlder ?? false);
    console.debug('[room-refresh] snapshot applied', {
      fetchedCount: fetched.length,
      preservedInFlightCount: merged.length - fetched.length,
      eventCount: this.events.length,
      hasOlder: connection.hasOlder ?? false,
      hasReachedStart: this.hasReachedStart
    });
  }

  private async refreshRoomLatest(
    thisLoad: number,
    existingBeforeFetch: ReadonlySet<string>
  ): Promise<RefreshCurrentWindowResult> {
    const page = this.roomTimeline
      ? await this.roomTimeline.getRoomEvents({
          roomId: this.roomId,
          limit: PAGE_SIZE
        })
      : await this.fetchRoomLatestViaGraphQL({ requestPolicy: 'network-only' });

    if (this.isStale(thisLoad)) return { hasOlder: false, hasNewer: false, refreshed: false };
    if (!page) return { hasOlder: false, hasNewer: false, refreshed: false };
    this.replaceWithSnapshotAndUpdateCursors(page, existingBeforeFetch);
    return { hasOlder: page.hasOlder, hasNewer: page.hasNewer, refreshed: true };
  }

  private async refreshRoomAround(
    thisLoad: number,
    anchorEventId: string,
    existingBeforeFetch: ReadonlySet<string>
  ): Promise<RefreshCurrentWindowResult | null> {
    const page = this.roomTimeline
      ? await this.roomTimeline.getRoomEventsAround({
          roomId: this.roomId,
          eventId: anchorEventId,
          limit: PAGE_SIZE
        })
      : await this.fetchRoomAroundViaGraphQL(anchorEventId, { requestPolicy: 'network-only' });

    if (this.isStale(thisLoad)) return { hasOlder: false, hasNewer: false, refreshed: false };
    if (!page) return null;
    this.replaceWithSnapshotAndUpdateCursors(page, existingBeforeFetch);
    return { hasOlder: page.hasOlder, hasNewer: page.hasNewer, refreshed: true };
  }

  private async fetchRoomLatestViaGraphQL(options?: {
    requestPolicy: 'network-only';
  }): Promise<EventConnectionPage | null> {
    const result = await this.client
      .query(
        RoomLatestQuery,
        {
          roomId: this.roomId,
          limit: PAGE_SIZE
        },
        options
      )
      .toPromise();

    if (result.error) {
      console.error('MessagesStore: fetchRoomLatestViaGraphQL error:', result.error);
      return null;
    }
    return result.data?.room?.events ?? null;
  }

  private async fetchRoomAfterViaGraphQL(after: string): Promise<EventConnectionPage | null> {
    const result = await this.client
      .query(RoomAfterQuery, {
        roomId: this.roomId,
        limit: PAGE_SIZE,
        after
      })
      .toPromise();

    if (result.error) {
      console.error('MessagesStore: fetchRoomAfterViaGraphQL error:', result.error);
      return null;
    }
    return result.data?.room?.events ?? null;
  }

  private async fetchRoomAroundViaGraphQL(
    eventId: string,
    options?: { requestPolicy: 'network-only' }
  ): Promise<EventConnectionPage | null> {
    const result = await this.client
      .query(
        RoomAroundQuery,
        {
          roomId: this.roomId,
          eventId,
          limit: PAGE_SIZE
        },
        options
      )
      .toPromise();

    if (result.error) {
      console.error('MessagesStore: fetchRoomAroundViaGraphQL error:', result.error);
      return null;
    }
    return result.data?.room?.eventsAround ?? null;
  }

  private async refreshThreadWindow(
    thisLoad: number,
    existingBeforeFetch: ReadonlySet<string>,
    anchorEventId: string | null
  ): Promise<RefreshCurrentWindowResult> {
    const timeline = this.requireRoomTimeline();
    const page = anchorEventId
      ? await timeline.getThreadEventsAround({
          roomId: this.roomId,
          threadRootEventId: this.threadRootEventId,
          eventId: anchorEventId,
          limit: PAGE_SIZE
        })
      : await timeline.getThreadEvents({
          roomId: this.roomId,
          threadRootEventId: this.threadRootEventId,
          limit: PAGE_SIZE
        });
    if (this.isStale(thisLoad)) return { hasOlder: false, hasNewer: false, refreshed: false };
    this.replaceWithSnapshotAndUpdateCursors(page, existingBeforeFetch);
    this.sortThreadEvents();
    return { hasOlder: page.hasOlder, hasNewer: page.hasNewer, refreshed: true };
  }

  private resetAndFetchLatest(): void {
    const thisLoad = this.startLoad();
    this.resetState();
    this.isInitialLoading = true;
    this.fetchLatest(thisLoad);
  }

  private fetchLatest(thisLoad: number): void {
    const promise = this.roomTimeline
      ? this.roomTimeline.getRoomEvents({
          roomId: this.roomId,
          limit: PAGE_SIZE
        })
      : this.fetchRoomLatestViaGraphQL();

    promise
      .then((page) => {
        if (this.isStale(thisLoad)) return;
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
    const promise = this.requireRoomTimeline().getThreadEvents({
      roomId: this.roomId,
      threadRootEventId: this.threadRootEventId,
      limit: PAGE_SIZE
    });

    promise
      .then((page) => {
        if (this.isStale(thisLoad)) return;
        // Merge with any subscription events that arrived during the
        // in-flight query (e.g. the user's own reply or a fast cross-user
        // reply). Overwriting would drop them.
        this.replaceMergingExisting(page.events);
        this.sortThreadEvents();
        this.oldestCursor = page.startCursor ?? undefined;
        this.newestCursor = page.endCursor ?? undefined;
        this.hasReachedStart = !page.hasOlder;
        this.isInitialLoading = false;
      })
      .catch((error: unknown) => {
        if (this.isStale(thisLoad)) return;
        console.error('MessagesStore: fetchThread failed:', error);
        this.isInitialLoading = false;
      });
  }

  private requireRoomTimeline(): RoomTimelineAPI {
    if (!this.roomTimeline) {
      throw new Error('MessagesStore: thread history requires the ConnectRPC timeline API');
    }
    return this.roomTimeline;
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
    this.events = this.events
      .map((event, index) => ({ event, index }))
      .sort((a, b) => {
        const aIsRoot = a.event.id === this.threadRootEventId;
        const bIsRoot = b.event.id === this.threadRootEventId;
        if (aIsRoot && !bIsRoot) return -1;
        if (!aIsRoot && bIsRoot) return 1;

        const byCreatedAt = compareEventCreatedAt(a.event, b.event);
        return byCreatedAt || a.index - b.index;
      })
      .map(({ event }) => event);
  }

  private sortRoomEvents(): void {
    this.events = this.events
      .map((event, index) => ({ event, index }))
      .sort((a, b) => compareEventCreatedAt(a.event, b.event) || a.index - b.index)
      .map(({ event }) => event);
  }
}
