import { tick } from 'svelte';
import { SvelteDate, SvelteMap, SvelteSet } from 'svelte/reactivity';
import {
  TimelineEventKind,
  timelineEventKind,
  type MessagePostedPayload,
  type TimelineEventPayload,
  type TimelineEventView
} from '$lib/render/timelineEvents';
import {
  createRoomTimelineAPI,
  roomTimelineEventToView,
  roomTimelinePageToEventConnectionPage,
  type RoomTimelineAPI
} from '$lib/api-client/roomTimeline';
import type {
  RoomTimelineEvent,
  RoomTimelineIncludes,
  RoomTimelinePage
} from '@chatto/api-types/api/v1/room_timeline_pb';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
import type { JumpToMessageState } from '../composerContext.svelte';
import { INITIAL_ROOM_MESSAGE_BACKFILL_TARGET, PAGE_SIZE } from './queries';
import { getActorId, unmask } from './helpers';
import { MessageTimelineSource } from './MessageTimelineSource';
import { OptimisticMutationRegistry } from '$lib/state/optimisticMutations';
import {
  beginOptimisticReaction as beginOptimisticReactionPatch,
  clearOptimisticReactionsForEvent,
  type OptimisticReactionAction,
  type OptimisticReactionHandle
} from './optimisticReactions';
import {
  beginOptimisticThreadFollow as beginOptimisticThreadFollowPatch,
  clearOptimisticThreadFollowForEvent,
  type OptimisticThreadFollowHandle
} from './optimisticThreadFollow';

export type {
  OptimisticReactionAction,
  OptimisticReactionHandle,
  OptimisticReactionServerSummary
} from './optimisticReactions';
export type { OptimisticThreadFollowHandle } from './optimisticThreadFollow';

type RoomDeletedPayload = Extract<
  TimelineEventPayload,
  { kind: typeof TimelineEventKind.RoomDeleted }
>;

export type RefreshCurrentWindowResult = {
  hasOlder: boolean;
  hasNewer: boolean;
  refreshed: boolean;
  changed: boolean;
};

function eventCacheKey(roomId: string, eventId: string): string {
  return `${roomId}\u0000${eventId}`;
}

function eventFingerprint(event: TimelineEventView): string {
  return JSON.stringify(event);
}

function sameEventList(a: readonly TimelineEventView[], b: readonly TimelineEventView[]): boolean {
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) {
    if (a[i].id !== b[i].id) return false;
    if (eventFingerprint(a[i]) !== eventFingerprint(b[i])) return false;
  }
  return true;
}

function snapshotEventFingerprints(
  events: readonly TimelineEventView[]
): SvelteMap<string, string> {
  return new SvelteMap(events.map((event) => [event.id, eventFingerprint(event)]));
}

function skippedRefreshResult(): RefreshCurrentWindowResult {
  return { hasOlder: false, hasNewer: false, refreshed: false, changed: false };
}

function isMessagePostedPayload(
  event: TimelineEventView['event'] | null | undefined
): event is MessagePostedPayload {
  return timelineEventKind(event) === TimelineEventKind.MessagePosted;
}

function scrubUserFromEvent(event: TimelineEventView, userId: string): TimelineEventView {
  const scrubActor = event.actor?.id === userId;
  const payload = event.event;
  if (!isMessagePostedPayload(payload)) {
    return scrubActor ? { ...event, actor: null } : event;
  }

  const threadParticipants = payload.threadParticipants.filter(
    (participant) => participant.id !== userId
  );
  let reactionsChanged = false;
  const reactions = payload.reactions.map((reaction) => {
    const users = reaction.users.filter((user) => user.id !== userId);
    if (users.length === reaction.users.length) return reaction;
    reactionsChanged = true;
    return { ...reaction, users };
  });
  const participantsChanged = threadParticipants.length !== payload.threadParticipants.length;
  if (!scrubActor && !participantsChanged && !reactionsChanged) return event;

  return {
    ...event,
    actor: scrubActor ? null : event.actor,
    event: {
      ...payload,
      threadParticipants,
      reactions
    }
  };
}

function isRoomDeletedPayload(event: TimelineEventView['event']): event is RoomDeletedPayload {
  return timelineEventKind(event) === TimelineEventKind.RoomDeleted;
}

function roomTimelineFromServerConnection(serverConnection: ServerConnection): RoomTimelineAPI {
  return serverConnection.getAPI(createRoomTimelineAPI);
}

/**
 * Message store for both the main room timeline and a single thread pane.
 * Room history uses the protobuf ConnectRPC timeline API when available;
 * thread history requires that path. Lifecycle, pagination, refetch, and
 * authoritative projection ingestion behavior stays shared across both scopes.
 */
export class MessagesStore {
  events = $state<TimelineEventView[]>([]);
  isInitialLoading = $state(true);
  isLoadingMore = $state(false);
  hasReachedStart = $state(false);

  private readonly roomTimeline: RoomTimelineAPI;
  private source: MessageTimelineSource | null = null;
  private seenIds: SvelteSet<string> = new SvelteSet<string>();
  private previewEvents = new SvelteMap<string, TimelineEventView | null>();
  private pendingPreviewFetches = new SvelteMap<string, Promise<void>>();
  private scrubbedUserIds = new SvelteSet<string>();
  private messageTombstones = new SvelteMap<string, string>();
  private removedMessageEventIds = new SvelteSet<string>();
  private oldestCursor: string | undefined;
  private newestCursor: string | undefined;
  private optimisticReactions = new OptimisticMutationRegistry();
  private optimisticThreadFollows = new OptimisticMutationRegistry();

  /** Increments on every load kickoff. Async callbacks compare against
   *  it via {@link isStale} to discard results from superseded loads. */
  #loadId = 0;
  #jumpId = 0;
  #windowId = 0;
  #pendingAuthoritativeLoadId: number | null = null;
  #pendingJumpId: number | null = null;
  #projectionAccessRevoked = false;
  #previewGeneration = 0;

  constructor(
    serverConnection: ServerConnection,
    private readonly getCurrentUserId: () => string | null,
    roomTimeline?: RoomTimelineAPI
  ) {
    this.roomTimeline = roomTimeline ?? roomTimelineFromServerConnection(serverConnection);
  }

  private get scope() {
    return this.source?.scope ?? null;
  }

  private get roomId() {
    return this.source?.roomId ?? '';
  }

  private get threadRootEventId() {
    return this.source?.threadRootEventId ?? '';
  }

  private selectRoom(roomId: string): MessageTimelineSource {
    if (!this.source?.matches('room', roomId)) {
      this.source = MessageTimelineSource.room(this.roomTimeline, roomId);
    }
    return this.source;
  }

  /** Tear down lifecycle listeners. Idempotent. */
  dispose(): void {
    // Invalidate every outstanding async read before its owner drops the
    // store. Server-event replay itself is managed by the singleton event bus.
    this.startLoad();
    this.invalidatePendingPreviewFetches();
  }

  /** Root-level events only (excludes thread replies). */
  get rootEvents(): TimelineEventView[] {
    const events = this.events;
    return this.source?.rootEventsFrom(events) ?? [];
  }

  /** Events that belong to this thread (root + replies). */
  get threadEvents(): TimelineEventView[] {
    const events = this.events;
    return this.source?.threadEventsFrom(events) ?? [];
  }

  /** Look up an event already known to this room, including off-window preview targets. */
  getEventById(eventId: string): TimelineEventView | null | undefined {
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
      if (isMessagePostedPayload(payload) && payload.echoOfEventId === messageEventId) {
        return event.id;
      }
      if (isMessagePostedPayload(payload) && payload.channelEchoEventId === messageEventId) {
        return event.id;
      }
    }

    for (const event of this.previewEvents.values()) {
      if (!event) continue;
      if (event.id === messageEventId) return event.id;
      const payload = event.event;
      if (isMessagePostedPayload(payload) && payload.echoOfEventId === messageEventId) {
        return event.id;
      }
      if (isMessagePostedPayload(payload) && payload.channelEchoEventId === messageEventId) {
        return event.id;
      }
    }

    return null;
  }

  /** Apply a successful local message delete without querying around a now-hidden echo. */
  applyLocalMessageDeletion(messageEventId: string): void {
    // The committed realtime retraction replaces this client timestamp with
    // the server event time. This provisional value lets the local tombstone
    // enter the grace period immediately after the mutation succeeds.
    this.applyDeletion(messageEventId, new SvelteDate().toISOString());
  }

  /**
   * Apply a provisional local reaction update. The returned handle can
   * reconcile the touched emoji from the RPC response or roll back if the
   * request fails. Projected server rows remain authoritative and clear the
   * optimistic version before a stale rollback can restore old state.
   */
  beginOptimisticReaction(input: {
    messageEventId: string;
    emoji: string;
    action: OptimisticReactionAction;
  }): OptimisticReactionHandle {
    return beginOptimisticReactionPatch({
      ...input,
      getEvents: () => this.events,
      previews: this.previewEvents,
      registry: this.optimisticReactions,
      setEvent: (eventId, event) => {
        const index = this.events.findIndex((candidate) => candidate.id === eventId);
        if (index !== -1) this.events[index] = event;
      },
      setPreview: (key, event) => {
        this.previewEvents.set(key, event);
      }
    });
  }

  /**
   * Apply a provisional local thread follow-state update on a known thread root.
   * Projected server rows and live follow events remain authoritative and clear
   * the pending optimistic mutation for the affected root row.
   */
  beginOptimisticThreadFollow(
    threadRootEventId: string,
    isFollowing: boolean
  ): OptimisticThreadFollowHandle {
    return beginOptimisticThreadFollowPatch({
      threadRootEventId,
      isFollowing,
      getEvents: () => this.events,
      registry: this.optimisticThreadFollows,
      setEvent: (eventId, event) => {
        const index = this.events.findIndex((candidate) => candidate.id === eventId);
        if (index !== -1) this.events[index] = event;
      }
    });
  }

  /** Update the viewer's thread follow state on a known thread root event. */
  setThreadRootFollowState(threadRootEventId: string, isFollowing: boolean): void {
    clearOptimisticThreadFollowForEvent(this.optimisticThreadFollows, threadRootEventId);
    const idx = this.events.findIndex((e) => e.id === threadRootEventId);
    if (idx === -1) return;

    const rootEvent = this.events[idx];
    if (!isMessagePostedPayload(rootEvent.event)) return;
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
    if (this.#projectionAccessRevoked) return undefined;
    if (this.events.some((e) => e.id === eventId)) return undefined;

    const key = this.previewKey(eventId);
    if (this.previewEvents.has(key)) return undefined;

    const existing = this.pendingPreviewFetches.get(key);
    if (existing) return existing;

    const previewGeneration = this.#previewGeneration;
    const roomId = this.roomId;
    const promise = this.fetchEventById(eventId)
      .then((event) => {
        if (this.#previewGeneration !== previewGeneration || this.roomId !== roomId) return;
        if (event) this.clearOptimisticVersionForEvent(event.id);
        this.previewEvents.set(key, event);
      })
      .catch((error: unknown) => {
        console.error('MessagesStore: ensureEvent failed:', error);
      })
      .finally(() => {
        if (this.pendingPreviewFetches.get(key) === promise) {
          this.pendingPreviewFetches.delete(key);
        }
      });

    this.pendingPreviewFetches.set(key, promise);
    return promise;
  }

  /** Allocate a new load id; pair with {@link isStale} in async callbacks. */
  private startLoad(): number {
    if (this.#pendingAuthoritativeLoadId !== null) {
      this.#pendingAuthoritativeLoadId = null;
      this.isInitialLoading = false;
    }
    return ++this.#loadId;
  }

  /** True if a newer load has started; caller should discard its result. */
  private isStale(thisLoad: number): boolean {
    return this.#loadId !== thisLoad;
  }

  private previewKey(eventId: string): string {
    return eventCacheKey(this.roomId, eventId);
  }

  private clearOptimisticVersionForEvent(eventId: string): void {
    clearOptimisticReactionsForEvent(this.optimisticReactions, eventId, this.previewKey(eventId));
    clearOptimisticThreadFollowForEvent(this.optimisticThreadFollows, eventId);
  }

  setRoom(roomId: string): void {
    if (this.source?.matches('room', roomId)) return;

    this.selectRoom(roomId);
    this.#jumpId++;
    this.#windowId++;
    this.#pendingJumpId = null;
    void this.resetAndFetchLatest();
  }

  /** Select a room without issuing a read while its projection prefix is in flight. */
  awaitRoomProjection(roomId: string): void {
    if (this.source?.matches('room', roomId)) return;
    this.startLoad();
    this.selectRoom(roomId);
    this.#pendingAuthoritativeLoadId = null;
    this.resetState();
    this.isInitialLoading = true;
  }

  /** Replace this room's recent retained window from the realtime projection stream. */
  replaceRoomProjectionPage(roomId: string, page: RoomTimelinePage): void {
    // A message deep-link may start its around-window read while the lazy
    // latest-page hydration is still in flight. Install the useful fallback
    // page, but do not let its late delivery cancel the newer navigation intent.
    const preservePendingJump = !!(
      this.#pendingJumpId !== null && this.source?.matches('room', roomId)
    );
    this.startLoad();
    if (!preservePendingJump) {
      this.#jumpId++;
      this.#pendingJumpId = null;
    }
    this.selectRoom(roomId);
    this.#pendingAuthoritativeLoadId = null;
    const connection = roomTimelinePageToEventConnectionPage(page);
    // Reset already purged the pre-prefix state. Preserve writes ingested
    // after that reset: the compacted page was captured before those writes
    // and its later arrival must not erase read-your-writes.
    this.replaceWithFetchedAndUpdateCursors(connection);
    this.hasReachedStart = !connection.hasOlder;
    this.isInitialLoading = preservePendingJump;
  }

  /** Supersede a historical jump when this room crosses a route boundary. */
  cancelPendingHistoricalJump(): void {
    this.#jumpId++;
    this.#windowId++;
    this.#pendingJumpId = null;
  }

  /** Restore this retained room's canonical latest projection at a route boundary. */
  restoreRoomProjectionPage(roomId: string, page: RoomTimelinePage): void {
    this.cancelPendingHistoricalJump();
    this.startLoad();
    const source = this.selectRoom(roomId);
    this.#pendingAuthoritativeLoadId = null;
    const connection = roomTimelinePageToEventConnectionPage(page);
    const projected = this.unmaskEvents(connection.events);
    for (const event of projected) this.clearOptimisticVersionForEvent(event.id);
    this.events = source.sort(projected);
    this.seenIds = new SvelteSet(projected.map((event) => event.id));
    this.oldestCursor = connection.startCursor ?? undefined;
    this.newestCursor = connection.endCursor ?? undefined;
    this.hasReachedStart = !connection.hasOlder;
    this.isInitialLoading = false;
  }

  /** Purge retained rows without changing this store's identity for mounted consumers. */
  resetProjectionState(): void {
    const thisLoad = this.startLoad();
    this.#jumpId++;
    this.#windowId++;
    this.#pendingJumpId = null;
    this.#pendingAuthoritativeLoadId = null;
    this.resetState();
    this.isInitialLoading = true;

    // Thread detail is intentionally lazy and is not part of the compacted
    // server prefix. Reload an open thread through its existing read model.
    if (this.scope === 'thread' && this.roomId && this.threadRootEventId) {
      void this.fetchCurrent(thisLoad);
    }
  }

  /**
   * Purge plaintext after projected room access is revoked.
   *
   * Unlike a transport reset, this must not issue a replacement read. The
   * incremented load generation also prevents an older room/thread response
   * from reinstalling data after the authorization transition.
   */
  clearForAccessRevocation(): void {
    this.startLoad();
    this.#jumpId++;
    this.#windowId++;
    this.#pendingJumpId = null;
    this.#pendingAuthoritativeLoadId = null;
    this.#projectionAccessRevoked = true;
    this.resetState();
    this.isInitialLoading = false;
  }

  /** Reload an open thread only when it was previously scrubbed for access loss. */
  restoreAfterAccessGrant(): void {
    if (!this.#projectionAccessRevoked) return;
    this.#projectionAccessRevoked = false;
    this.isInitialLoading = true;
    if (this.scope === 'thread' && this.roomId && this.threadRootEventId) {
      void this.fetchCurrent(this.startLoad());
    }
  }

  /**
   * Remove copied render data for a deleted account while preserving stable
   * actor and participant IDs on historical facts.
   */
  scrubUserReferences(userId: string): void {
    this.invalidatePendingPreviewFetches();
    this.optimisticReactions.clearAll();
    this.scrubbedUserIds.add(userId);
    const events = this.events.map((event) => this.scrubKnownUserReferences(event));
    if (events.some((event, index) => event !== this.events[index])) this.events = events;

    for (const [key, event] of this.previewEvents) {
      if (!event) continue;
      const scrubbed = this.scrubKnownUserReferences(event);
      if (scrubbed !== event) this.previewEvents.set(key, scrubbed);
    }
  }

  /** Apply one authoritative current timeline row from the projection stream. */
  upsertRoomProjectionEvent(
    roomId: string,
    event: RoomTimelineEvent,
    includes: RoomTimelineIncludes | undefined,
    retainDeletedRow = false,
    insertIfMissing = true
  ): void {
    if (this.roomId !== roomId) return;
    this.isInitialLoading = false;
    const projectedMessage =
      event.event.case === 'messagePosted' ? event.event.value.message : null;
    if (projectedMessage?.deletedAt) {
      const deletedAt = projectedMessage.deletedAt.toDate().toISOString();
      if (retainDeletedRow) this.applyRetainedDeletion(event.id, deletedAt);
      else this.applyDeletion(event.id, deletedAt);
      return;
    }
    const view = roomTimelineEventToView(event, includes?.users ?? {});
    if (!view) return;
    const projected = this.unmaskEvents([view])[0];
    if (!projected) return;

    const existingIndex = this.events.findIndex((candidate) => candidate.id === projected.id);
    if (existingIndex === -1) {
      if (!insertIfMissing) return;
      this.ingestEvent(projected);
      return;
    }
    this.clearOptimisticVersionForEvent(projected.id);
    this.events[existingIndex] = projected;
    this.sortEvents();
  }

  private applyRetainedDeletion(messageEventId: string, deletedAt: string): void {
    this.invalidatePendingPreviewFetches();
    this.messageTombstones.set(messageEventId, deletedAt);
    const index = this.events.findIndex((event) => event.id === messageEventId);
    if (index !== -1) {
      const event = this.applyPrivacyBoundaries(this.events[index]);
      if (event) this.events[index] = event;
    }
    this.applyPrivacyBoundariesToPreviews();
  }

  /** Remove one projection-only row, such as a disabled channel echo. */
  removeRoomProjectionEvent(roomId: string, eventId: string): void {
    if (this.roomId !== roomId) return;
    this.invalidatePendingPreviewFetches();
    this.removedMessageEventIds.add(eventId);
    this.clearChannelEchoLink(eventId);
    this.previewEvents.delete(this.previewKey(eventId));
    const index = this.events.findIndex((event) => event.id === eventId);
    if (index === -1) return;
    this.events.splice(index, 1);
    this.seenIds.delete(eventId);
  }

  setThread(roomId: string, threadRootEventId: string): void {
    if (this.source?.matches('thread', roomId, threadRootEventId)) return;

    this.source = MessageTimelineSource.thread(this.roomTimeline, roomId, threadRootEventId);
    this.#jumpId++;
    this.#windowId++;
    this.#pendingJumpId = null;

    const thisLoad = this.startLoad();
    this.resetState();
    this.isInitialLoading = true;
    void this.fetchCurrent(thisLoad);
  }

  /**
   * Route an already-renderable event into the store. Used for historical
   * pages and read-your-writes after mutations that return the posted event.
   */
  ingestEvent(spaceEvent: TimelineEventView): void {
    const sanitisedEvent = this.applyPrivacyBoundaries(spaceEvent);
    if (!sanitisedEvent) return;
    spaceEvent = sanitisedEvent;
    const eventData = spaceEvent.event;
    const kind = timelineEventKind(eventData);

    if (isRoomDeletedPayload(eventData)) {
      if (eventData.roomId === this.roomId) this.resetState();
      return;
    }

    // From here on, only events scoped to this room are interesting.
    if (eventData.roomId !== this.roomId) return;

    if (isMessagePostedPayload(eventData)) {
      this.onMessagePosted(spaceEvent, eventData);
      return;
    }

    if (
      kind === TimelineEventKind.UserJoinedRoom ||
      kind === TimelineEventKind.UserLeftRoom ||
      kind === TimelineEventKind.RoomUpdated ||
      kind === TimelineEventKind.RoomArchived ||
      kind === TimelineEventKind.RoomUnarchived ||
      kind === TimelineEventKind.RoomCreated ||
      kind === TimelineEventKind.CallStarted ||
      kind === TimelineEventKind.CallEnded
    ) {
      this.onSystemEvent(spaceEvent);
    }
  }

  async loadMore(): Promise<void> {
    const source = this.source;
    if (
      !source ||
      this.#projectionAccessRevoked ||
      this.isLoadingMore ||
      this.hasReachedStart ||
      !this.oldestCursor
    )
      return;

    const before = this.oldestCursor;
    const loadId = this.#loadId;
    this.isLoadingMore = true;

    try {
      const page = await source.fetchPage({ limit: PAGE_SIZE, before });

      // A reset, access revocation, route/scope change, or owner disposal may
      // have happened while this page was in flight. Never let an older
      // authorization context reinstall plaintext or overwrite new cursors.
      if (this.isStale(loadId) || this.#projectionAccessRevoked || this.source !== source) {
        return;
      }

      const olderEvents = this.unmaskEvents(page.events);
      if (olderEvents.length === 0) {
        if (page.startCursor) {
          this.oldestCursor = page.startCursor;
        }
        if (!page.hasOlder || !page.startCursor || page.startCursor === before) {
          this.hasReachedStart = true;
        }
      } else {
        if (page.startCursor) {
          this.oldestCursor = page.startCursor;
        }
        const added = this.prependEvents(olderEvents);
        if (source.scope === 'thread') this.events = source.sort(this.events);
        if (added === 0 && (!page.hasOlder || !page.startCursor || page.startCursor === before)) {
          this.hasReachedStart = true;
        }
      }

      if (!page.hasOlder) this.hasReachedStart = true;
    } catch (error) {
      console.error('MessagesStore: loadMore failed:', error);
    } finally {
      // Yield a frame so the virtualizer can settle before another loadMore.
      await tick();
      await new Promise((r) => requestAnimationFrame(r));
      if (!this.isStale(loadId) && this.source === source) {
        this.isLoadingMore = false;
      }
    }
  }

  async refetchAll(): Promise<void> {
    const snapshot = [...(this.source?.eventsFrom(this.events) ?? [])];
    for (const event of snapshot) {
      await this.refetchOne(event.id);
    }
  }

  private roomWindowMessageCount(): number {
    return this.rootEvents.filter((event) => isMessagePostedPayload(event.event)).length;
  }

  private async backfillInitialRoomWindow(thisLoad: number): Promise<void> {
    while (
      !this.isStale(thisLoad) &&
      this.scope === 'room' &&
      !this.hasReachedStart &&
      this.oldestCursor &&
      this.roomWindowMessageCount() < INITIAL_ROOM_MESSAGE_BACKFILL_TARGET
    ) {
      await this.loadMore();
    }
  }

  async loadNewer(jumpState: JumpToMessageState): Promise<void> {
    const source = this.source;
    if (source?.scope !== 'room') return;
    if (jumpState.isLoadingNewer || jumpState.hasReachedEnd) return;
    if (!this.newestCursor) return;

    const windowId = this.#windowId;
    jumpState.isLoadingNewer = true;
    try {
      const page = await source.fetchPage({
        limit: PAGE_SIZE,
        after: this.newestCursor
      });

      // User left jumped mode while in flight — abandon the result.
      if (!jumpState.isJumpedMode || this.source !== source || this.#windowId !== windowId) {
        return;
      }

      const newer = this.unmaskEvents(page.events);
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
      if (this.source === source && this.#windowId === windowId) {
        jumpState.isLoadingNewer = false;
      }
    }
  }

  async jumpToMessage(eventId: string, jumpState: JumpToMessageState): Promise<boolean> {
    const source = this.source;
    if (source?.scope !== 'room') return false;
    const jumpId = ++this.#jumpId;
    if (this.events.some((e) => e.id === eventId)) {
      if (this.#pendingJumpId !== null) {
        this.#pendingJumpId = null;
        if (this.#pendingAuthoritativeLoadId === null) this.isInitialLoading = false;
      }
      jumpState.scrollToEventId = eventId;
      return true;
    }

    this.#windowId++;
    this.#pendingJumpId = jumpId;
    jumpState.isLoadingNewer = false;
    this.isInitialLoading = true;
    try {
      const around = await source.fetchAround(eventId, PAGE_SIZE);

      if (this.#jumpId !== jumpId || this.source !== source) return false;

      const { events: rawEvents, hasOlder, hasNewer, startCursor, endCursor } = around;
      const parsed = this.unmaskEvents(rawEvents);
      if (!parsed.some((event) => event.id === eventId)) {
        if (this.events.some((event) => event.id === eventId)) {
          jumpState.scrollToEventId = eventId;
          return true;
        }
        jumpState.scrollToEventId = null;
        jumpState.isJumpedMode = false;
        jumpState.hasReachedEnd = false;
        jumpState.hasOlderMessages = false;
        return false;
      }

      // This replacement becomes the authoritative room window. Cancel any
      // older latest-page load before installing it.
      this.startLoad();
      this.#pendingAuthoritativeLoadId = null;
      for (const event of parsed) this.clearOptimisticVersionForEvent(event.id);
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
      return true;
    } catch (error) {
      if (this.#jumpId !== jumpId || this.source !== source) return false;
      if (this.events.some((event) => event.id === eventId)) {
        jumpState.scrollToEventId = eventId;
        return true;
      }
      console.error('MessagesStore: jumpToMessage failed:', error);
      jumpState.scrollToEventId = null;
      jumpState.isJumpedMode = false;
      jumpState.hasReachedEnd = false;
      jumpState.hasOlderMessages = false;
      return false;
    } finally {
      if (this.#jumpId === jumpId && this.source === source) {
        this.#pendingJumpId = null;
        this.isInitialLoading = this.#pendingAuthoritativeLoadId !== null;
      }
    }
  }

  jumpToPresent(jumpState: JumpToMessageState): Promise<boolean> {
    if (this.scope !== 'room') return Promise.resolve(false);
    this.#jumpId++;
    this.#windowId++;
    this.#pendingJumpId = null;
    jumpState.reset();
    return this.resetAndFetchLatest();
  }

  /**
   * Refresh the currently displayed message window from projected state without
   * clearing the buffer. Used after tab wake / reconnect when the client may
   * have missed subscription events.
   */
  async refreshCurrentWindow(anchorEventId?: string | null): Promise<RefreshCurrentWindowResult> {
    const source = this.source;
    if (!source) return skippedRefreshResult();

    const thisLoad = this.startLoad();
    const existingBeforeFetch = snapshotEventFingerprints(this.events);
    const anchor = anchorEventId ?? null;
    const mode = anchor
      ? source.scope === 'thread'
        ? 'thread-around'
        : 'around'
      : source.scope === 'thread'
        ? 'thread-latest'
        : 'latest';
    console.debug('[room-refresh] store refresh started', {
      roomId: source.roomId,
      scope: source.scope,
      anchorEventId: anchor,
      existingCount: this.events.length
    });

    try {
      const page = anchor
        ? await source.fetchAround(anchor, PAGE_SIZE)
        : await source.fetchPage({ limit: PAGE_SIZE });
      if (this.isStale(thisLoad) || this.source !== source) return skippedRefreshResult();
      const changed = this.replaceWithSnapshotAndUpdateCursors(page, existingBeforeFetch, {
        preserveExistingWindow:
          source.scope === 'room' || anchor === null || anchor !== source.threadRootEventId,
        latestSnapshot: anchor === null
      });
      const result = {
        hasOlder: page.hasOlder,
        hasNewer: page.hasNewer,
        refreshed: true,
        changed
      };
      console.debug('[room-refresh] store refresh finished', {
        roomId: source.roomId,
        scope: source.scope,
        mode,
        anchorEventId: anchor,
        result,
        eventCount: this.events.length
      });
      return result;
    } catch (error) {
      if (this.isStale(thisLoad)) return skippedRefreshResult();
      console.error('MessagesStore: refreshCurrentWindow failed:', error);
      return skippedRefreshResult();
    }
  }

  private onMessagePosted(spaceEvent: TimelineEventView, eventData: MessagePostedPayload): void {
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
        this.sortEvents();
      }
      return;
    }

    // Thread replies don't enter the room timeline; instead, update
    // metadata on the root message (replyCount, lastReplyAt, participants,
    // viewerIsFollowingThread auto-follow).
    if (eventData.threadRootEventId) {
      if (this.seenIds.has(spaceEvent.id)) return;
      this.seenIds.add(spaceEvent.id);
      this.applyThreadReplyToRoot(spaceEvent, eventData);
      return;
    }
    this.addEvent(spaceEvent);
  }

  private onSystemEvent(spaceEvent: TimelineEventView): void {
    if (this.scope === 'room') {
      this.addEvent(spaceEvent);
    }
  }

  private async fetchEventById(
    eventId: string,
    threadRootEventId?: string | null
  ): Promise<TimelineEventView | null> {
    const page = await this.source?.fetchAround(eventId, 1, threadRootEventId ?? null);
    if (!page) return null;
    return this.unmaskEvents(page.events).find((event) => event.id === eventId) ?? null;
  }

  private async refetchOne(eventId: string): Promise<void> {
    const updated = await this.fetchEventById(
      eventId,
      this.scope === 'thread' && eventId !== this.threadRootEventId ? this.threadRootEventId : null
    );
    if (!updated) return;
    this.clearOptimisticVersionForEvent(updated.id);
    const idx = this.events.findIndex((e) => e.id === eventId);
    if (idx !== -1) this.events[idx] = updated;
  }

  /**
   * Apply a deletion locally. Direct echo retractions hide only the echo
   * artifact; original-message retractions tombstone the original and any
   * visible echoes that point at it.
   * Reactions and reply metadata are left intact so the tombstone row keeps
   * its existing engagement visible alongside the placeholder.
   */
  private applyDeletion(messageEventId: string, deletedAt: string): void {
    this.invalidatePendingPreviewFetches();
    this.clearChannelEchoLink(messageEventId);

    const targetIndex = this.events.findIndex((e) => e.id === messageEventId);
    const target = targetIndex === -1 ? null : this.events[targetIndex];
    const targetPayload = target?.event;
    if (isMessagePostedPayload(targetPayload) && targetPayload.echoOfEventId) {
      this.removedMessageEventIds.add(messageEventId);
      this.events.splice(targetIndex, 1);
      this.seenIds.delete(messageEventId);
      this.previewEvents.delete(this.previewKey(messageEventId));
      return;
    }

    this.messageTombstones.set(messageEventId, deletedAt);

    for (let i = 0; i < this.events.length; i++) {
      const e = this.events[i];
      const evt = e.event;
      if (!isMessagePostedPayload(evt)) continue;
      if (e.id !== messageEventId && evt.echoOfEventId !== messageEventId) continue;

      this.events[i] = {
        ...e,
        event: { ...evt, body: null, attachments: [], linkPreview: null, deletedAt }
      };
    }

    this.applyPrivacyBoundariesToPreviews();
  }

  private applyChannelEchoLink(originalEventId: string, echoEventId: string): void {
    for (let i = 0; i < this.events.length; i++) {
      const e = this.events[i];
      const evt = e.event;
      if (e.id !== originalEventId || !isMessagePostedPayload(evt)) continue;
      this.events[i] = {
        ...e,
        event: { ...evt, channelEchoEventId: echoEventId }
      };
    }

    const previewKey = this.previewKey(originalEventId);
    const preview = this.previewEvents.get(previewKey);
    if (isMessagePostedPayload(preview?.event)) {
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
      if (!isMessagePostedPayload(evt)) continue;
      if (evt.channelEchoEventId !== echoEventId) continue;
      this.events[i] = {
        ...e,
        event: { ...evt, channelEchoEventId: null }
      };
    }

    for (const [key, preview] of this.previewEvents) {
      if (!isMessagePostedPayload(preview?.event)) continue;
      if (preview.event.channelEchoEventId !== echoEventId) continue;
      this.previewEvents.set(key, {
        ...preview,
        event: { ...preview.event, channelEchoEventId: null }
      });
    }
  }

  private addEvent(event: TimelineEventView, options: { sortRoom?: boolean } = {}): boolean {
    if (this.seenIds.has(event.id)) return false;
    this.seenIds.add(event.id);
    this.events.push(event);
    if ((options.sortRoom ?? true) && this.scope === 'room') this.sortEvents();
    return true;
  }

  private appendMany(events: TimelineEventView[]): void {
    let added = false;
    for (const e of events) {
      this.clearOptimisticVersionForEvent(e.id);
      added = this.addEvent(e, { sortRoom: false }) || added;
    }
    if (added && this.scope === 'room') this.sortEvents();
  }

  private prependEvents(olderEvents: TimelineEventView[]): number {
    const newOnes = olderEvents.filter((e) => !this.seenIds.has(e.id));
    for (const e of newOnes) this.clearOptimisticVersionForEvent(e.id);
    for (const e of newOnes) this.seenIds.add(e.id);
    this.events.unshift(...newOnes);
    return newOnes.length;
  }

  /**
   * Replace the buffer with fetched events but preserve any subscription
   * events that arrived during the in-flight query. Always the right
   * choice when a paginated query result replaces the timeline: the
   * projection subscription has been live since layout mount, so any
   * timeline event for this room that lands while the query is in flight
   * has already been added to {@link events} via {@link ingestEvent} and
   * must not be wiped by the result.
   */
  private replaceMergingExisting(events: readonly TimelineEventView[]): void {
    const fetched = this.unmaskEvents(events);
    const newSeen = new SvelteSet<string>();
    const merged: TimelineEventView[] = [];
    for (const e of fetched) {
      if (newSeen.has(e.id)) continue;
      this.clearOptimisticVersionForEvent(e.id);
      newSeen.add(e.id);
      merged.push(e);
    }
    for (const e of this.events) {
      if (newSeen.has(e.id)) continue;
      newSeen.add(e.id);
      merged.push(e);
    }
    this.events = merged;
    if (this.scope === 'room') this.sortEvents();
    this.seenIds = newSeen;
  }

  private resetState(): void {
    this.events = [];
    this.seenIds = new SvelteSet();
    this.previewEvents.clear();
    this.invalidatePendingPreviewFetches();
    this.optimisticReactions.clearAll();
    this.optimisticThreadFollows.clearAll();
    this.oldestCursor = undefined;
    this.newestCursor = undefined;
    this.hasReachedStart = false;
    this.isLoadingMore = false;
  }

  /** Discard preview responses captured before a plaintext-clearing boundary. */
  private invalidatePendingPreviewFetches(): void {
    this.#previewGeneration++;
    this.pendingPreviewFetches.clear();
  }

  /** Remove render-only data for every account deleted during this store's lifetime. */
  private scrubKnownUserReferences(event: TimelineEventView): TimelineEventView {
    for (const userId of this.scrubbedUserIds) event = scrubUserFromEvent(event, userId);
    return event;
  }

  /** Apply persistent deletion and account-removal fences to a timeline row. */
  private applyPrivacyBoundaries(event: TimelineEventView): TimelineEventView | null {
    if (this.#projectionAccessRevoked) return null;
    if (this.removedMessageEventIds.has(event.id)) return null;
    event = this.scrubKnownUserReferences(event);
    const payload = event.event;
    if (!isMessagePostedPayload(payload)) return event;

    const deletedAt =
      this.messageTombstones.get(event.id) ??
      (payload.echoOfEventId ? this.messageTombstones.get(payload.echoOfEventId) : undefined);
    if (!deletedAt) return event;
    return {
      ...event,
      event: { ...payload, body: null, attachments: [], linkPreview: null, deletedAt }
    };
  }

  private applyPrivacyBoundariesToPreviews(): void {
    for (const [key, event] of this.previewEvents) {
      if (!event) continue;
      const sanitised = this.applyPrivacyBoundaries(event);
      if (sanitised) this.previewEvents.set(key, sanitised);
      else this.previewEvents.delete(key);
    }
  }

  private unmaskEvents(events: readonly TimelineEventView[]): TimelineEventView[] {
    return unmask(events).flatMap((event) => {
      const sanitised = this.applyPrivacyBoundaries(event);
      return sanitised ? [sanitised] : [];
    });
  }

  private replaceWithFetchedAndUpdateCursors(connection: {
    events: readonly TimelineEventView[];
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
      events: readonly TimelineEventView[];
      startCursor?: string | null;
      endCursor?: string | null;
      hasOlder?: boolean;
    },
    existingBeforeFetch: ReadonlyMap<string, string>,
    options: { preserveExistingWindow?: boolean; latestSnapshot?: boolean } = {}
  ): boolean {
    const fetched = this.unmaskEvents(connection.events);
    const newSeen = new SvelteSet<string>();
    const merged: TimelineEventView[] = [];
    const mergedIndexByID = new SvelteMap<string, number>();
    const previousOldestCursor = this.oldestCursor;
    const previousNewestCursor = this.newestCursor;
    const previousHasReachedStart = this.hasReachedStart;
    const hasExistingContinuityEvents = this.events.some(
      (event) => existingBeforeFetch.has(event.id) && !!this.source?.isContinuityEvent(event)
    );
    const hasFetchedOverlap = fetched.some(
      (event) => existingBeforeFetch.has(event.id) && !!this.source?.isContinuityEvent(event)
    );
    const discontinuousLatestSnapshot =
      !!options.preserveExistingWindow &&
      !!options.latestSnapshot &&
      !!connection.hasOlder &&
      hasExistingContinuityEvents &&
      !hasFetchedOverlap;

    for (const e of fetched) {
      if (newSeen.has(e.id)) continue;
      this.clearOptimisticVersionForEvent(e.id);
      newSeen.add(e.id);
      mergedIndexByID.set(e.id, merged.length);
      merged.push(e);
    }

    // Preserve subscription events that arrived while the refresh query was in
    // flight. Anchored refreshes also preserve already-loaded rows outside the
    // fetched window so returning from another tab does not visually collapse a
    // long scrolled buffer.
    for (const e of this.events) {
      const priorFingerprint = existingBeforeFetch.get(e.id);
      const changedDuringFetch =
        priorFingerprint === undefined || priorFingerprint !== eventFingerprint(e);
      const fetchedIndex = mergedIndexByID.get(e.id);
      if (changedDuringFetch && fetchedIndex !== undefined) {
        // A projection upsert can refresh an existing row while the snapshot
        // query is in flight (for example, thread follow state on the root).
        // The later local version is authoritative over the older query row.
        merged[fetchedIndex] = e;
        continue;
      }
      if (
        (!options.preserveExistingWindow || discontinuousLatestSnapshot) &&
        existingBeforeFetch.has(e.id)
      ) {
        continue;
      }
      if (newSeen.has(e.id)) continue;
      newSeen.add(e.id);
      mergedIndexByID.set(e.id, merged.length);
      merged.push(e);
    }

    const nextEvents = this.source?.sort(merged) ?? merged;
    const changed = !sameEventList(this.events, nextEvents);

    if (changed) {
      this.events = nextEvents;
      this.seenIds = newSeen;
    }

    if (options.preserveExistingWindow && !discontinuousLatestSnapshot) {
      this.oldestCursor = previousOldestCursor ?? connection.startCursor ?? undefined;
      this.newestCursor = options.latestSnapshot
        ? (connection.endCursor ?? previousNewestCursor ?? undefined)
        : (previousNewestCursor ?? connection.endCursor ?? undefined);
      this.hasReachedStart = previousHasReachedStart || !(connection.hasOlder ?? false);
    } else {
      this.oldestCursor = connection.startCursor ?? undefined;
      this.newestCursor = connection.endCursor ?? undefined;
      this.hasReachedStart = !(connection.hasOlder ?? false);
    }
    console.debug('[room-refresh] snapshot applied', {
      fetchedCount: fetched.length,
      preservedExistingCount: nextEvents.length - fetched.length,
      changed,
      discontinuousLatestSnapshot,
      eventCount: this.events.length,
      hasOlder: connection.hasOlder ?? false,
      hasReachedStart: this.hasReachedStart
    });
    return changed;
  }

  private resetAndFetchLatest(): Promise<boolean> {
    const thisLoad = this.startLoad();
    this.#pendingAuthoritativeLoadId = thisLoad;
    this.resetState();
    this.isInitialLoading = true;
    return this.fetchCurrent(thisLoad);
  }

  private async fetchCurrent(thisLoad: number): Promise<boolean> {
    const source = this.source;
    if (!source) return false;
    const existingBeforeFetch = snapshotEventFingerprints(this.events);
    try {
      const page = await source.fetchPage({ limit: PAGE_SIZE });
      if (this.isStale(thisLoad) || this.source !== source) return false;
      if (source.scope === 'room') {
        this.replaceWithFetchedAndUpdateCursors(page);
        this.hasReachedStart = !page.hasOlder;
        await this.backfillInitialRoomWindow(thisLoad);
      } else {
        // Merge with any subscription events that arrived during the
        // in-flight query (e.g. the user's own reply or a fast cross-user
        // reply). Overwriting would drop them.
        this.replaceWithSnapshotAndUpdateCursors(page, existingBeforeFetch);
      }
      if (this.isStale(thisLoad) || this.source !== source) return false;
      this.#pendingAuthoritativeLoadId = null;
      this.isInitialLoading = false;
      return true;
    } catch (error: unknown) {
      if (this.isStale(thisLoad) || this.source !== source) return false;
      console.error('MessagesStore: fetchCurrent failed:', error);
      this.#pendingAuthoritativeLoadId = null;
      this.isInitialLoading = false;
      return false;
    }
  }

  /**
   * Mirror the backend's auto-follow behavior on the root message when a
   * thread reply arrives, so the UI updates instantly without refetching.
   */
  private applyThreadReplyToRoot(
    spaceEvent: TimelineEventView,
    eventData: MessagePostedPayload
  ): void {
    const rootIdx = this.events.findIndex((e) => e.id === eventData.threadRootEventId);
    if (rootIdx === -1) return;

    const rootEvent = this.events[rootIdx];
    if (!isMessagePostedPayload(rootEvent.event)) return;

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

  private sortEvents(): void {
    if (this.source) this.events = this.source.sort(this.events);
  }
}
