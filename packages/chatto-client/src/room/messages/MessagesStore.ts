import { ReactiveMap, ReactiveSet, batch, computed, signal } from '../../reactivity/index.js';
import {
  TimelineEventKind,
  timelineEventKind,
  type MessagePostedPayload,
  type TimelineEventPayload,
  type TimelineEventView
} from '../../timeline/timelineEvents.js';
import {
  createRoomTimelineAPI,
  roomTimelineEventToView,
  type RoomTimelineAPI
} from '../../api/roomTimeline.js';
import type {
  RoomTimelineEvent,
  RoomTimelineIncludes
} from '@chatto/api-types/api/v1/room_timeline_pb';
import type { ServerConnection } from '../../server/serverConnection.js';
import { Code, ConnectError, isConnectCode, StaleResponseError } from '../../api/connect.js';
import { getActorId, unmask } from './helpers.js';
import { MessageTimelineSource } from './MessageTimelineSource.js';
import { OptimisticMutationRegistry } from '../../util/optimisticMutations.js';
import {
  beginOptimisticReaction as beginOptimisticReactionPatch,
  clearOptimisticReactionsForEvent,
  type OptimisticReactionAction,
  type OptimisticReactionHandle
} from './optimisticReactions.js';
import {
  beginOptimisticThreadFollow as beginOptimisticThreadFollowPatch,
  clearOptimisticThreadFollowForEvent,
  type OptimisticThreadFollowHandle
} from './optimisticThreadFollow.js';
import { debugLog } from '../../util/debugLog.js';

/** Messages requested per timeline page. */
export const PAGE_SIZE = 50;
/** Messages to backfill before an initial room window is considered filled. */
export const INITIAL_ROOM_MESSAGE_BACKFILL_TARGET = 10;

export type {
  OptimisticReactionAction,
  OptimisticReactionHandle,
  OptimisticReactionServerSummary
} from './optimisticReactions.js';
export type { OptimisticThreadFollowHandle } from './optimisticThreadFollow.js';

type RoomDeletedPayload = Extract<
  TimelineEventPayload,
  { kind: typeof TimelineEventKind.RoomDeleted }
>;

/** The timeline that a {@link MessagesStore} owns: a room, or one thread in it. */
export type MessageTimelineTarget = {
  roomId: string;
  /** The thread root event ID, or null for the room timeline. */
  threadRootEventId?: string | null;
};

/** Where a host shows a timeline: the event at the top of its view. */
export type TimelineAnchor = {
  eventId: string;
  /** Whether newer events exist after the window that a reset loaded around the anchor. */
  hasNewer?: boolean;
};

/** The result of {@link MessagesStore.jumpToMessage}. */
export type JumpResult =
  /** The event is in the window. */
  | { status: 'shown' }
  /** The window was replaced with the page around the event. */
  | { status: 'loaded'; hasNewer: boolean; hasOlder: boolean }
  /** The event cannot be loaded. */
  | { status: 'missing' }
  /** A newer jump or a route boundary superseded this jump. */
  | { status: 'superseded' };

/** The result of {@link MessagesStore.loadNewer}. */
export type LoadNewerResult =
  /** A newer page was read. `hasNewer` is false at the latest event. */
  | { status: 'loaded'; hasNewer: boolean }
  /** The read failed; nothing changed. */
  | { status: 'failed' }
  /** Nothing was applied: no newer page exists, or the caller declined the result. */
  | { status: 'declined' }
  /** A jump or reset replaced the window during the read; nothing was applied. */
  | { status: 'superseded' };

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
  return a.every((event, i) => {
    const other = b[i];
    return (
      other !== undefined &&
      event.id === other.id &&
      eventFingerprint(event) === eventFingerprint(other)
    );
  });
}

/** Thread fields can refresh without replacing message content or reactions. */
function threadSummary(payload: MessagePostedPayload) {
  return {
    threadExists: payload.threadExists,
    replyCount: payload.replyCount,
    lastReplyAt: payload.lastReplyAt,
    threadParticipantCount: payload.threadParticipantCount,
    threadParticipants: payload.threadParticipants,
    viewerIsFollowingThread: payload.viewerIsFollowingThread,
    viewerHasUnreadThread: payload.viewerHasUnreadThread
  };
}

function snapshotEventFingerprints(events: readonly TimelineEventView[]): Map<string, string> {
  return new Map(events.map((event) => [event.id, eventFingerprint(event)]));
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
  const scrubActor = event.actorId === userId || event.actor?.id === userId;
  const payload = event.event;
  if (!isMessagePostedPayload(payload)) {
    return scrubActor ? { ...event, actor: null, actorResolution: 'deleted' } : event;
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
    actorResolution: scrubActor ? 'deleted' : event.actorResolution,
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
  readonly #threadSlowModeDeadline = signal<string | null | undefined>(undefined);
  /** Current viewer deadline from a thread read or an authorized live row. */
  get threadSlowModeNextPostAt(): string | null | undefined {
    return this.#threadSlowModeDeadline.get();
  }
  readonly #eventsSignal = signal<TimelineEventView[]>([]);
  /**
   * The loaded rows in display order. Observers track the array reference:
   * every change assigns a new array, and rows are never mutated in place.
   */
  get events(): TimelineEventView[] {
    return this.#eventsSignal.get();
  }
  set events(value: TimelineEventView[]) {
    this.#eventsSignal.set(value);
  }
  readonly #isInitialLoadingSignal = signal(true);
  get isInitialLoading() {
    return this.#isInitialLoadingSignal.get();
  }
  set isInitialLoading(value) {
    this.#isInitialLoadingSignal.set(value);
  }
  readonly #isLoadingMoreSignal = signal(false);
  get isLoadingMore() {
    return this.#isLoadingMoreSignal.get();
  }
  set isLoadingMore(value) {
    this.#isLoadingMoreSignal.set(value);
  }
  readonly #hasReachedStartSignal = signal(false);
  get hasReachedStart() {
    return this.#hasReachedStartSignal.get();
  }
  set hasReachedStart(value) {
    this.#hasReachedStartSignal.set(value);
  }
  /**
   * The anchor that a reset kept. The replacement window loads around it; the
   * host restores its view there and then calls {@link completeRecovery}.
   * Only the event ID is kept, never message content. Reactive.
   */
  readonly #recoveryAnchorSignal = signal<TimelineAnchor | null>(null);
  get recoveryAnchor(): TimelineAnchor | null {
    return this.#recoveryAnchorSignal.get();
  }
  #anchor: string | null = null;

  /**
   * Report the event at the top of the host's view, or null when the view
   * follows the latest event. A reset reloads the window around it. Returns
   * false, and keeps the previous anchor, while the store loads or recovers.
   */
  setAnchor(eventId: string | null): boolean {
    if (this.isInitialLoading || this.recoveryAnchor) return false;
    this.#anchor = eventId;
    return true;
  }

  /** Forget the anchor, for example when the host's view unmounts. */
  clearAnchor(): void {
    this.#anchor = null;
    this.#recoveryAnchorSignal.set(null);
  }

  /** The host restored its view at the {@link recoveryAnchor}. */
  completeRecovery(): void {
    this.#recoveryAnchorSignal.set(null);
  }

  private readonly roomTimeline: RoomTimelineAPI;
  /** The room or thread timeline that this store owns for its whole lifetime. */
  private readonly source: MessageTimelineSource;
  private seenIds = new Set<string>();
  private previewEvents = new ReactiveMap<string, TimelineEventView | null>();
  private pendingPreviewFetches = new ReactiveMap<string, Promise<void>>();
  private scrubbedUserIds = new ReactiveSet<string>();
  private messageTombstones = new ReactiveMap<string, string>();
  private removedMessageEventIds = new ReactiveSet<string>();
  private oldestCursor: string | undefined;
  private newestCursor: string | undefined;
  private optimisticReactions = new OptimisticMutationRegistry();
  private optimisticThreadFollows = new OptimisticMutationRegistry();

  /** Increments on every load kickoff. Async callbacks compare against
   *  it via {@link isStale} to discard results from superseded loads. */
  #loadId = 0;
  #jumpId = 0;
  #windowId = 0;
  #previewGeneration = 0;

  // `isCurrent` derives from the signals below. Store methods read them with
  // `peek()`, so a host that creates a store inside a reactive read does not
  // subscribe to internal state that the same call then writes.
  readonly #pendingAuthoritativeLoadIdSignal = signal<number | null>(null);
  get #pendingAuthoritativeLoadId(): number | null {
    return this.#pendingAuthoritativeLoadIdSignal.peek();
  }
  set #pendingAuthoritativeLoadId(value: number | null) {
    this.#pendingAuthoritativeLoadIdSignal.set(value);
  }
  readonly #pendingJumpIdSignal = signal<number | null>(null);
  get #pendingJumpId(): number | null {
    return this.#pendingJumpIdSignal.peek();
  }
  set #pendingJumpId(value: number | null) {
    this.#pendingJumpIdSignal.set(value);
  }
  /** True when the retained window is historical or its latest window read failed. */
  readonly #needsLatestWindowSignal = signal(false);
  get #needsLatestWindow(): boolean {
    return this.#needsLatestWindowSignal.peek();
  }
  set #needsLatestWindow(value: boolean) {
    this.#needsLatestWindowSignal.set(value);
  }
  readonly #projectionAccessRevokedSignal = signal(false);
  get #projectionAccessRevoked(): boolean {
    return this.#projectionAccessRevokedSignal.peek();
  }
  set #projectionAccessRevoked(value: boolean) {
    this.#projectionAccessRevokedSignal.set(value);
  }
  readonly #disposedSignal = signal(false);
  /** The read of the latest authoritative load; see {@link refreshCurrentWindow}. */
  #authoritativeRead: { loadId: number; read: Promise<boolean> } | null = null;

  readonly #loadErrorSignal = signal<unknown>(null);
  /**
   * The error of the latest failed window read, or null. A read fails, for
   * example, when the server is unreachable, the viewer cannot read the
   * timeline, or the thread root does not exist. Loss of room access sets a
   * `PermissionDenied` error without a read. The next latest-window read, or
   * a jump that replaces the window, clears it.
   * Call {@link restoreLatestWindow} to read again after a failed read; after
   * access loss, the store reads again when access returns. Reactive.
   */
  get loadError(): unknown {
    return this.#loadErrorSignal.get();
  }

  readonly #isCurrent = computed(
    () =>
      !this.#disposedSignal.get() &&
      !this.isInitialLoading &&
      this.loadError === null &&
      this.#pendingAuthoritativeLoadIdSignal.get() === null &&
      this.#pendingJumpIdSignal.get() === null &&
      !this.#needsLatestWindowSignal.get() &&
      !this.#projectionAccessRevokedSignal.get()
  );
  /**
   * True when the window shows the latest events and realtime delivery keeps
   * it current: its latest window read succeeded, and no read, reconnect
   * reload, or jump is pending. While the realtime connection is down, the
   * window shows the state of the last delivered event. False while the store
   * loads, after a read fails ({@link loadError}), while it shows a historical
   * window, after room access is lost, and after {@link dispose}. Older pages
   * can still be missing; see {@link hasReachedStart}. Reactive.
   */
  get isCurrent(): boolean {
    return this.#isCurrent.get();
  }

  /**
   * Create the store for one room or thread timeline and start its first read.
   * The timeline cannot change later; each room and thread gets its own store.
   */
  constructor(
    serverConnection: ServerConnection,
    private readonly getCurrentUserId: () => string | null,
    target: MessageTimelineTarget,
    roomTimeline?: RoomTimelineAPI
  ) {
    this.roomTimeline = roomTimeline ?? roomTimelineFromServerConnection(serverConnection);
    this.source = target.threadRootEventId
      ? MessageTimelineSource.thread(this.roomTimeline, target.roomId, target.threadRootEventId)
      : MessageTimelineSource.room(this.roomTimeline, target.roomId);
    void this.resetAndFetchLatest();
  }

  private get scope() {
    return this.source.scope;
  }

  private get roomId() {
    return this.source.roomId;
  }

  private get threadRootEventId() {
    return this.source.threadRootEventId ?? '';
  }

  /**
   * True after {@link dispose}. A disposed store gets no more realtime updates,
   * and it is never current. A host that keeps a store across a long wait, for
   * example a wait for {@link isCurrent}, gets a new store from its registry
   * when this becomes true. Reactive.
   */
  get isDisposed(): boolean {
    return this.#disposedSignal.get();
  }

  /** Tear down lifecycle listeners. Idempotent. A disposed store is never current. */
  dispose(): void {
    this.#disposedSignal.set(true);
    // Invalidate every outstanding async read before its owner drops the
    // store. Server-event replay itself is managed by the singleton event bus.
    this.startLoad();
    this.invalidatePendingPreviewFetches();
  }

  /** Root-level events only (excludes thread replies). */
  get rootEvents(): TimelineEventView[] {
    const events = this.events;
    return this.source.rootEventsFrom(events);
  }

  /** Events that belong to this thread (root + replies). */
  get threadEvents(): TimelineEventView[] {
    const events = this.events;
    return this.source.threadEventsFrom(events);
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
    // the server event time. This provisional value marks a confirmed local
    // deletion so context-free filtering applies as soon as the mutation succeeds.
    this.applyDeletion(messageEventId, new Date().toISOString());
  }

  /** Fold a canonical retraction into every loaded original or echo row immediately. */
  applyMessageRetraction(messageEventId: string, retractedAt: string): void {
    this.applyDeletion(messageEventId, retractedAt);
  }

  /** Include loaded echo wrappers and thread roots in the shared message read. */
  relatedMessageIds(messageEventId: string): string[] {
    const ids = new Set<string>();
    for (const row of [...this.events, ...this.previewEvents.values()]) {
      if (!row || !isMessagePostedPayload(row.event)) continue;
      if (
        row.id !== messageEventId &&
        row.event.echoOfEventId !== messageEventId &&
        row.event.channelEchoEventId !== messageEventId
      )
        continue;
      ids.add(row.id);
      if (row.event.threadRootEventId) ids.add(row.event.threadRootEventId);
      if (row.event.echoOfEventId) ids.add(row.event.echoOfEventId);
      if (row.event.channelEchoEventId) ids.add(row.event.channelEchoEventId);
    }
    return [...ids];
  }

  /**
   * Capture loaded roots for a shared summary read. Each callback updates only
   * unchanged summary fields in this view. Content edits remain intact, and a
   * reset, disposal, or access loss makes the captured callbacks stale.
   */
  captureThreadSummaries(): Map<string, (root: TimelineEventView) => void> {
    const loadId = this.#loadId;
    const capture = (rows: Iterable<TimelineEventView | null>) => {
      const summaries = new Map<string, string>();
      for (const row of rows) {
        if (row && isMessagePostedPayload(row.event) && row.event.threadExists) {
          summaries.set(row.id, JSON.stringify(threadSummary(row.event)));
        }
      }
      return summaries;
    };
    const rowsBefore = capture(this.events);
    const previewsBefore = capture(this.previewEvents.values());
    const callbacks = new Map<string, (root: TimelineEventView) => void>();
    for (const rootId of new Set([...rowsBefore.keys(), ...previewsBefore.keys()])) {
      callbacks.set(rootId, (event) => {
        if (this.isStale(loadId) || this.#projectionAccessRevoked || event.id !== rootId) return;
        const root = this.unmaskEvents([event])[0];
        if (!root || !isMessagePostedPayload(root.event)) return;
        const summary = threadSummary(root.event);
        const apply = (row: TimelineEventView, before: string | undefined): TimelineEventView =>
          row.id === rootId &&
          isMessagePostedPayload(row.event) &&
          JSON.stringify(threadSummary(row.event)) === before
            ? { ...row, event: { ...row.event, ...summary } }
            : row;
        batch(() => {
          this.updateEvents((row) => apply(row, rowsBefore.get(rootId)));
          const preview = this.previewEvents.get(rootId);
          if (preview) this.previewEvents.set(rootId, apply(preview, previewsBefore.get(rootId)));
        });
      });
    }
    return callbacks;
  }

  /**
   * Capture this timeline before a shared read. A local edit or a newer page
   * response wins over that read. Only new posts may insert off-window rows;
   * resource updates do not change pagination cursors or loaded continuity.
   */
  captureMessageReconciliation(): (
    id: string,
    event: TimelineEventView | null,
    insert: boolean
  ) => void {
    const before = snapshotEventFingerprints(this.events);
    const previews = snapshotEventFingerprints(
      [...this.previewEvents.values()].filter((event): event is TimelineEventView => !!event)
    );
    return (id, event, insert) => {
      if (this.#projectionAccessRevoked) return;
      if (!event) {
        this.applyMessageRetraction(id, new Date().toISOString());
        return;
      }
      const hydrated = this.unmaskEvents([event])[0];
      if (!hydrated) return;
      const index = this.events.findIndex((row) => row.id === id);
      const current = this.events[index];
      if (current) {
        if (eventFingerprint(current) !== before.get(id)) return;
        this.applyThreadDeadlineFromMessage(hydrated);
        this.clearOptimisticVersionForEvent(id);
        this.events = this.source.sort(this.events.with(index, hydrated));
      } else if (insert) this.ingestEvent(hydrated);
      const preview = this.previewEvents.get(id);
      if (preview && eventFingerprint(preview) === previews.get(id))
        this.previewEvents.set(id, hydrated);
    };
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
        if (index !== -1) this.events = this.events.with(index, event);
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
        if (index !== -1) this.events = this.events.with(index, event);
      }
    });
  }

  /** Update the viewer's thread follow state on a known thread root event. */
  setThreadRootFollowState(threadRootEventId: string, isFollowing: boolean): void {
    clearOptimisticThreadFollowForEvent(this.optimisticThreadFollows, threadRootEventId);
    const idx = this.events.findIndex((e) => e.id === threadRootEventId);
    const rootEvent = this.events[idx];
    if (!rootEvent || !isMessagePostedPayload(rootEvent.event)) return;
    if (rootEvent.event.viewerIsFollowingThread === isFollowing) return;

    this.events = this.events.with(idx, {
      ...rootEvent,
      event: {
        ...rootEvent.event,
        viewerIsFollowingThread: isFollowing
      }
    });
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
    // A reply target can belong to an unopened thread. Room timeline reads
    // exclude those messages, so previews use the message resource directly.
    const promise = this.roomTimeline
      .getMessage({ roomId, eventId })
      .then((result) => {
        if (this.#previewGeneration !== previewGeneration || this.roomId !== roomId) return;
        const event = result ? (this.unmaskEvents([result])[0] ?? null) : null;
        if (event) this.clearOptimisticVersionForEvent(event.id);
        this.previewEvents.set(key, event);
      })
      .catch((error: unknown) => {
        if (isConnectCode(error, Code.NotFound) || isConnectCode(error, Code.PermissionDenied)) {
          // An unavailable target stays absent until the preview cache is reset.
          if (this.#previewGeneration !== previewGeneration || this.roomId !== roomId) return;
          this.previewEvents.set(key, null);
          return;
        }
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
    // A superseded loadMore cannot clear its own flag, so clear it here.
    this.isLoadingMore = false;
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

  /** Supersede a historical jump when this room crosses a route boundary. */
  cancelPendingHistoricalJump(): void {
    this.#jumpId++;
    this.#windowId++;
    this.#pendingJumpId = null;
    if (this.#pendingAuthoritativeLoadId === null) this.isInitialLoading = false;
  }

  /**
   * Cancel a pending jump and load the latest window at a route boundary, when
   * a jump left the retained window on older events or the latest window read
   * failed. Resolves to whether this call loaded the latest window; it does
   * not start a read when the window is already current or a read is pending.
   */
  restoreLatestWindow(): Promise<boolean> {
    if (this.recoveryAnchor) return Promise.resolve(false);
    this.cancelPendingHistoricalJump();
    if (this.#pendingAuthoritativeLoadId !== null || !this.#needsLatestWindow) {
      return Promise.resolve(false);
    }
    return this.resetAndFetchLatest();
  }

  /** Purge retained rows without starting a read outside a realtime boundary. */
  resetProjectionState(): void {
    batch(() => {
      if (!this.recoveryAnchor && this.#anchor) {
        this.#recoveryAnchorSignal.set({ eventId: this.#anchor });
      }
      const thisLoad = this.startLoad();
      this.#jumpId++;
      this.#windowId++;
      this.#pendingJumpId = null;
      this.#pendingAuthoritativeLoadId = thisLoad;
      this.resetState();
      this.isInitialLoading = true;
    });
  }

  /** Reload this mounted timeline at the resource boundary for a reset. */
  hydrateRealtimeProjection(
    minimumCursor: string,
    acceptResult: () => boolean,
    replaceWindow = false
  ): Promise<boolean> {
    const thisLoad = batch(() => {
      const thisLoad = this.startLoad();
      this.#pendingAuthoritativeLoadId = thisLoad;
      // Keep the retained timeline visible until its replacement read settles.
      this.isInitialLoading = this.events.length === 0;
      return thisLoad;
    });
    return this.trackAuthoritativeRead(
      thisLoad,
      this.fetchCurrent(
        thisLoad,
        minimumCursor,
        acceptResult,
        this.recoveryAnchor?.eventId,
        replaceWindow
      )
    );
  }

  /**
   * Purge plaintext after projected room access is revoked.
   *
   * Unlike a transport reset, this must not issue a replacement read. The
   * incremented load generation also prevents an older room/thread response
   * from reinstalling data after the authorization transition.
   */
  clearForAccessRevocation(): void {
    batch(() => {
      this.clearAnchor();
      this.startLoad();
      this.#jumpId++;
      this.#windowId++;
      this.#pendingJumpId = null;
      this.#pendingAuthoritativeLoadId = null;
      this.#projectionAccessRevoked = true;
      this.resetState();
      this.isInitialLoading = false;
      this.#loadErrorSignal.set(
        new ConnectError('access to the room timeline was revoked', Code.PermissionDenied)
      );
    });
  }

  /** Reload an open thread only when it was previously scrubbed for access loss. */
  restoreAfterAccessGrant(): void {
    if (!this.#projectionAccessRevoked) return;
    const thisLoad = batch(() => {
      this.#projectionAccessRevoked = false;
      this.isInitialLoading = true;
      if (this.scope !== 'thread' || !this.roomId || !this.threadRootEventId) return null;
      const thisLoad = this.startLoad();
      this.#pendingAuthoritativeLoadId = thisLoad;
      return thisLoad;
    });
    if (thisLoad !== null) void this.trackAuthoritativeRead(thisLoad, this.fetchCurrent(thisLoad));
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
    this.applyThreadDeadlineFromMessage(projected);

    const existingIndex = this.events.findIndex((candidate) => candidate.id === projected.id);
    if (existingIndex === -1) {
      if (!insertIfMissing) return;
      this.ingestEvent(projected);
      return;
    }
    this.clearOptimisticVersionForEvent(projected.id);
    this.events = this.source.sort(this.events.with(existingIndex, projected));
  }

  private applyRetainedDeletion(messageEventId: string, deletedAt: string): void {
    this.invalidatePendingPreviewFetches();
    this.messageTombstones.set(messageEventId, deletedAt);
    const index = this.events.findIndex((event) => event.id === messageEventId);
    const current = this.events[index];
    if (current) {
      const event = this.applyPrivacyBoundaries(current);
      if (event) this.events = this.events.with(index, event);
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
    this.events = this.events.toSpliced(index, 1);
    this.seenIds.delete(eventId);
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
      kind === TimelineEventKind.RoomThreadingModeChanged ||
      kind === TimelineEventKind.CallStarted ||
      kind === TimelineEventKind.CallEnded
    ) {
      this.onSystemEvent(spaceEvent);
    }
  }

  async loadMore(): Promise<void> {
    const source = this.source;
    if (
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

      // A reset, access revocation, or owner disposal may have happened while
      // this page was in flight. Never let an older
      // authorization context reinstall plaintext or overwrite new cursors.
      if (this.isStale(loadId) || this.#projectionAccessRevoked) {
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
      if (!this.isStale(loadId)) {
        this.isLoadingMore = false;
      }
    }
  }

  async refetchAll(): Promise<void> {
    const snapshot = [...this.source.eventsFrom(this.events)];
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

  /** Whether the window has a cursor to read newer events after it. */
  get canLoadNewer(): boolean {
    return this.newestCursor !== undefined;
  }

  /**
   * Read the next newer page after a historical window and append it.
   * `accept` is checked before the page is applied, for example whether the
   * host still shows the historical window.
   */
  async loadNewer(accept: () => boolean = () => true): Promise<LoadNewerResult> {
    const source = this.source;
    if (!this.newestCursor) return { status: 'declined' };

    const windowId = this.#windowId;
    try {
      const page = await source.fetchPage({
        limit: PAGE_SIZE,
        after: this.newestCursor
      });

      if (this.#windowId !== windowId) return { status: 'superseded' };
      if (!accept()) return { status: 'declined' };

      const newer = this.unmaskEvents(page.events);
      if (newer.length > 0) {
        if (page.endCursor) {
          this.newestCursor = page.endCursor;
        }
        this.appendMany(newer);
      }

      if (!page.hasNewer) this.#needsLatestWindow = false;
      return { status: 'loaded', hasNewer: newer.length > 0 && page.hasNewer };
    } catch (error) {
      console.error('MessagesStore: loadNewer failed:', error);
      return this.#windowId === windowId ? { status: 'failed' } : { status: 'superseded' };
    }
  }

  /**
   * Show a message: when the window does not contain it, replace the window
   * with the page around it. See {@link JumpResult}.
   */
  async jumpToMessage(eventId: string): Promise<JumpResult> {
    const source = this.source;
    const jumpId = ++this.#jumpId;
    if (this.events.some((e) => e.id === eventId)) {
      if (this.#pendingJumpId !== null) {
        this.#pendingJumpId = null;
        if (this.#pendingAuthoritativeLoadId === null) this.isInitialLoading = false;
      }
      return { status: 'shown' };
    }

    this.#windowId++;
    this.#pendingJumpId = jumpId;
    this.isInitialLoading = true;
    const deadlineBeforeFetch = this.#threadSlowModeDeadline.get();
    const existingBeforeFetch = snapshotEventFingerprints(this.events);
    try {
      const around = await source.fetchAround(eventId, PAGE_SIZE);

      if (this.#jumpId !== jumpId) return { status: 'superseded' };

      const { events: rawEvents, hasOlder, hasNewer, startCursor, endCursor } = around;
      const parsed = this.unmaskEvents(rawEvents).map((event) => {
        const current = this.events.find((row) => row.id === event.id);
        return current && eventFingerprint(current) !== existingBeforeFetch.get(event.id)
          ? current
          : event;
      });
      if (!parsed.some((event) => event.id === eventId)) {
        if (this.events.some((event) => event.id === eventId)) return { status: 'shown' };
        return { status: 'missing' };
      }

      // This replacement becomes the authoritative room window. Cancel any
      // older latest-page load before installing it.
      this.startLoad();
      this.#pendingAuthoritativeLoadId = null;
      for (const event of parsed) this.clearOptimisticVersionForEvent(event.id);
      this.events = [...parsed];
      this.applyThreadDeadlineFromSnapshot(around.threadSlowModeNextPostAt, deadlineBeforeFetch);
      this.seenIds = new Set(parsed.map((e) => e.id));
      this.oldestCursor = startCursor ?? undefined;
      this.newestCursor = endCursor ?? undefined;
      this.hasReachedStart = !hasOlder;
      this.#needsLatestWindow = hasNewer;
      this.#loadErrorSignal.set(null);
      return { status: 'loaded', hasNewer, hasOlder };
    } catch (error) {
      if (this.#jumpId !== jumpId) return { status: 'superseded' };
      if (this.events.some((event) => event.id === eventId)) return { status: 'shown' };
      console.error('MessagesStore: jumpToMessage failed:', error);
      return { status: 'missing' };
    } finally {
      if (this.#jumpId === jumpId) {
        this.#pendingJumpId = null;
        this.isInitialLoading = this.#pendingAuthoritativeLoadId !== null;
      }
    }
  }

  /** Leave a historical window: forget the anchor and load the latest window. */
  jumpToLatest(): Promise<boolean> {
    this.clearAnchor();
    this.#jumpId++;
    this.#windowId++;
    this.#pendingJumpId = null;
    return this.resetAndFetchLatest();
  }

  /**
   * Refresh the currently displayed message window from projected state without
   * clearing the buffer. Used after tab wake / reconnect when the client may
   * have missed subscription events. While a read that replaces the window is
   * pending, the refresh waits for it; see {@link isCurrent}.
   */
  async refreshCurrentWindow(
    anchorEventId?: string | null,
    forward = false,
    minimumCursor?: string,
    acceptResult: () => boolean = () => true
  ): Promise<RefreshCurrentWindowResult> {
    const source = this.source;

    // A refresh that superseded a pending read that replaces the window would
    // leave the store current without a window. The refresh waits for that
    // read instead. After a reset, the realtime reload reads the window.
    while (this.#pendingAuthoritativeLoadId !== null) {
      const pending = this.#authoritativeRead;
      if (pending?.loadId !== this.#pendingAuthoritativeLoadId) return skippedRefreshResult();
      await pending.read.catch(() => {});
      // A rejected result keeps its load pending until a later read.
      if (this.#pendingAuthoritativeLoadId === pending.loadId) return skippedRefreshResult();
    }

    const thisLoad = this.startLoad();
    const deadlineBeforeFetch = this.#threadSlowModeDeadline.get();
    const existingBeforeFetch = snapshotEventFingerprints(this.events);
    const anchor = anchorEventId ?? null;
    const forwardCursor = forward && anchor ? (this.newestCursor ?? null) : null;
    const mode = forwardCursor
      ? 'forward'
      : anchor
        ? source.scope === 'thread'
          ? 'thread-around'
          : 'around'
        : source.scope === 'thread'
          ? 'thread-latest'
          : 'latest';
    debugLog('[room-refresh] store refresh started', {
      roomId: source.roomId,
      scope: source.scope,
      anchorEventId: anchor,
      existingCount: this.events.length
    });

    try {
      const page = forwardCursor
        ? await source.fetchPage({ limit: PAGE_SIZE, after: forwardCursor, minimumCursor })
        : anchor
          ? await source.fetchAround(anchor, PAGE_SIZE, undefined, minimumCursor)
          : await source.fetchPage({ limit: PAGE_SIZE, minimumCursor });
      if (this.isStale(thisLoad) || !acceptResult()) {
        return skippedRefreshResult();
      }
      const changed = this.replaceWithSnapshotAndUpdateCursors(page, existingBeforeFetch, {
        deadlineBeforeFetch,
        preserveExistingWindow:
          source.scope === 'room' || anchor === null || anchor !== source.threadRootEventId,
        latestSnapshot: anchor === null,
        forwardSnapshot: forwardCursor !== null,
        reconciledEventId: forwardCursor === null ? anchor : null
      });
      const result = {
        hasOlder: page.hasOlder,
        hasNewer: page.hasNewer,
        refreshed: true,
        changed
      };
      debugLog('[room-refresh] store refresh finished', {
        roomId: source.roomId,
        scope: source.scope,
        mode,
        anchorEventId: anchor,
        result,
        eventCount: this.events.length
      });
      return result;
    } catch (error) {
      if (this.isStale(thisLoad) || !acceptResult()) return skippedRefreshResult();
      if (isConnectCode(error, Code.PermissionDenied) || isConnectCode(error, Code.NotFound)) {
        return skippedRefreshResult();
      }
      console.error('MessagesStore: refreshCurrentWindow failed:', error);
      if (minimumCursor) throw error;
      return skippedRefreshResult();
    }
  }

  /**
   * Read and ingest one newly posted message before the wider cursor window is
   * reconciled. This keeps realtime delivery responsive without treating the
   * canonical event as a second message-resource shape. The result reports
   * whether the caller still accepts the result and can reconcile the timeline.
   */
  async refreshPostedMessage(
    eventId: string,
    minimumCursor?: string,
    acceptResult: () => boolean = () => true
  ): Promise<boolean> {
    if (!eventId) return false;

    const previous = this.events.find((event) => event.id === eventId);
    const previousFingerprint = previous ? eventFingerprint(previous) : null;
    try {
      const event = await this.roomTimeline.getMessage({
        roomId: this.roomId,
        eventId,
        minimumCursor
      });
      if (!acceptResult()) return false;
      if (event) {
        const currentIndex = this.events.findIndex((candidate) => candidate.id === eventId);
        const current = this.events[currentIndex];
        const hydrated = this.applyPrivacyBoundaries(event);
        if (hydrated && !current) this.ingestEvent(hydrated);
        else if (hydrated && current && eventFingerprint(current) === previousFingerprint) {
          // Replace the temporary row. Ordinary ingestion deduplicates event IDs.
          // A newer local change or resource response takes precedence over this read.
          this.applyThreadDeadlineFromMessage(hydrated);
          this.clearOptimisticVersionForEvent(eventId);
          this.events = this.source.sort(this.events.with(currentIndex, hydrated));
        }
      } else this.markAuthorUnavailable(eventId);
      return true;
    } catch (error) {
      if (!acceptResult()) return false;
      this.markAuthorUnavailable(eventId);
      if (isConnectCode(error, Code.PermissionDenied) || isConnectCode(error, Code.NotFound)) {
        return true;
      }
      console.error('MessagesStore: refreshPostedMessage failed:', error);
      if (minimumCursor) throw error;
      return true;
    }
  }

  /** End a failed author load without changing resolved or deleted identities. */
  private markAuthorUnavailable(eventId: string): void {
    this.events = this.events.map((event) =>
      event.id === eventId && event.actorResolution === 'loading'
        ? { ...event, actorResolution: 'unavailable' }
        : event
    );
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
        this.applyThreadDeadlineFromMessage(spaceEvent);
        this.addEvent(spaceEvent);
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
    const page = await this.source.fetchAround(eventId, 1, threadRootEventId ?? null);
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
    if (idx !== -1) this.events = this.events.with(idx, updated);
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
      this.events = this.events.toSpliced(targetIndex, 1);
      this.seenIds.delete(messageEventId);
      this.previewEvents.delete(this.previewKey(messageEventId));
      return;
    }

    this.messageTombstones.set(messageEventId, deletedAt);

    this.updateEvents((e) => {
      const evt = e.event;
      if (!isMessagePostedPayload(evt)) return e;
      if (e.id !== messageEventId && evt.echoOfEventId !== messageEventId) return e;
      return {
        ...e,
        event: { ...evt, body: null, attachments: [], linkPreview: null, deletedAt }
      };
    });

    this.applyPrivacyBoundariesToPreviews();
  }

  private applyChannelEchoLink(originalEventId: string, echoEventId: string): void {
    this.updateEvents((e) => {
      const evt = e.event;
      if (e.id !== originalEventId || !isMessagePostedPayload(evt)) return e;
      return { ...e, event: { ...evt, channelEchoEventId: echoEventId } };
    });

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
    this.updateEvents((e) => {
      const evt = e.event;
      if (!isMessagePostedPayload(evt) || evt.channelEchoEventId !== echoEventId) return e;
      return { ...e, event: { ...evt, channelEchoEventId: null } };
    });

    for (const [key, preview] of this.previewEvents) {
      if (!isMessagePostedPayload(preview?.event)) continue;
      if (preview.event.channelEchoEventId !== echoEventId) continue;
      this.previewEvents.set(key, {
        ...preview,
        event: { ...preview.event, channelEchoEventId: null }
      });
    }
  }

  private addEvent(event: TimelineEventView): void {
    if (this.seenIds.has(event.id)) return;
    this.seenIds.add(event.id);
    this.events = this.source.sort([...this.events, event]);
  }

  private appendMany(events: TimelineEventView[]): void {
    const added: TimelineEventView[] = [];
    for (const e of events) {
      this.clearOptimisticVersionForEvent(e.id);
      if (this.seenIds.has(e.id)) continue;
      this.seenIds.add(e.id);
      added.push(e);
    }
    // A live event can arrive before an older page of newer events.
    if (added.length > 0) this.events = this.source.sort([...this.events, ...added]);
  }

  private prependEvents(olderEvents: TimelineEventView[]): number {
    const newOnes = olderEvents.filter((e) => !this.seenIds.has(e.id));
    for (const e of newOnes) this.clearOptimisticVersionForEvent(e.id);
    for (const e of newOnes) this.seenIds.add(e.id);
    if (newOnes.length > 0) this.events = [...newOnes, ...this.events];
    return newOnes.length;
  }

  /** Only hydrated messages in this canonical thread can update its deadline. */
  private applyThreadDeadlineFromMessage(view: TimelineEventView): void {
    if (
      this.scope === 'thread' &&
      isMessagePostedPayload(view.event) &&
      (view.event.threadRootEventId === this.threadRootEventId ||
        view.id === this.threadRootEventId) &&
      view.event.threadSlowModeNextPostAt !== undefined
    ) {
      this.#threadSlowModeDeadline.set(view.event.threadSlowModeNextPostAt);
    }
  }

  /** Preserve live state received while the snapshot request was in flight. */
  private applyThreadDeadlineFromSnapshot(
    deadline: string | null | undefined,
    deadlineBeforeFetch: string | null | undefined
  ): void {
    if (this.#threadSlowModeDeadline.get() === deadlineBeforeFetch) {
      this.#threadSlowModeDeadline.set(deadline);
    }
  }

  private resetState(): void {
    batch(() => {
      this.#threadSlowModeDeadline.set(undefined);
      this.events = [];
      this.seenIds = new Set();
      this.#needsLatestWindow = false;
      this.#loadErrorSignal.set(null);
      this.previewEvents.clear();
      this.invalidatePendingPreviewFetches();
      this.optimisticReactions.clearAll();
      this.optimisticThreadFollows.clearAll();
      this.oldestCursor = undefined;
      this.newestCursor = undefined;
      this.hasReachedStart = false;
      this.isLoadingMore = false;
    });
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

  private replaceWithSnapshotAndUpdateCursors(
    connection: {
      threadSlowModeNextPostAt?: string | null;
      events: readonly TimelineEventView[];
      startCursor?: string | null;
      endCursor?: string | null;
      hasOlder?: boolean;
    },
    existingBeforeFetch: ReadonlyMap<string, string>,
    options: {
      deadlineBeforeFetch?: string | null;
      preserveExistingWindow?: boolean;
      latestSnapshot?: boolean;
      forwardSnapshot?: boolean;
      reconciledEventId?: string | null;
    } = {}
  ): boolean {
    this.applyThreadDeadlineFromSnapshot(
      connection.threadSlowModeNextPostAt,
      options.deadlineBeforeFetch
    );
    const fetched = this.unmaskEvents(connection.events);
    const newSeen = new Set<string>();
    const merged: TimelineEventView[] = [];
    const mergedIndexByID = new Map<string, number>();
    const previousOldestCursor = this.oldestCursor;
    const previousNewestCursor = this.newestCursor;
    const previousHasReachedStart = this.hasReachedStart;
    const hasExistingContinuityEvents = this.events.some(
      (event) => existingBeforeFetch.has(event.id) && this.source.isContinuityEvent(event)
    );
    const hasFetchedOverlap = fetched.some(
      (event) => existingBeforeFetch.has(event.id) && this.source.isContinuityEvent(event)
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
      if (
        options.reconciledEventId &&
        existingBeforeFetch.has(e.id) &&
        this.isLinkedToMessage(e, options.reconciledEventId)
      ) {
        // An around read is authoritative for its message family. If the
        // projection no longer returns a linked echo, do not retain that echo
        // merely because the rest of the scrolled window is merge-preserved.
        continue;
      }
      if (newSeen.has(e.id)) continue;
      newSeen.add(e.id);
      mergedIndexByID.set(e.id, merged.length);
      merged.push(e);
    }

    const nextEvents = this.source.sort(merged);
    const changed = !sameEventList(this.events, nextEvents);

    if (changed) {
      this.events = nextEvents;
      this.seenIds = newSeen;
    }

    if (options.preserveExistingWindow && !discontinuousLatestSnapshot) {
      this.oldestCursor = previousOldestCursor ?? connection.startCursor ?? undefined;
      this.newestCursor =
        options.latestSnapshot || options.forwardSnapshot
          ? (connection.endCursor ?? previousNewestCursor ?? undefined)
          : (previousNewestCursor ?? connection.endCursor ?? undefined);
      // A page without older events reaches the start only when it joins the
      // loaded window. A separate older page leaves a gap that paging must fill.
      const continuesWindow = hasFetchedOverlap || !hasExistingContinuityEvents;
      this.hasReachedStart =
        previousHasReachedStart || (continuesWindow && !(connection.hasOlder ?? false));
    } else {
      this.oldestCursor = connection.startCursor ?? undefined;
      this.newestCursor = connection.endCursor ?? undefined;
      this.hasReachedStart = !(connection.hasOlder ?? false);
    }
    debugLog('[room-refresh] snapshot applied', {
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

  private isLinkedToMessage(event: TimelineEventView, eventId: string): boolean {
    if (event.id === eventId) return true;
    const payload = event.event;
    return (
      isMessagePostedPayload(payload) &&
      (payload.echoOfEventId === eventId || payload.channelEchoEventId === eventId)
    );
  }

  private async resetAndFetchLatest(): Promise<boolean> {
    const thisLoad = batch(() => {
      const thisLoad = this.startLoad();
      this.#pendingAuthoritativeLoadId = thisLoad;
      this.resetState();
      this.isInitialLoading = true;
      return thisLoad;
    });
    const loaded = await this.trackAuthoritativeRead(thisLoad, this.fetchCurrent(thisLoad));
    if (!this.isStale(thisLoad)) this.#needsLatestWindow = !loaded;
    return loaded;
  }

  /** Record the read of the pending authoritative load `loadId`. */
  private trackAuthoritativeRead(loadId: number, read: Promise<boolean>): Promise<boolean> {
    this.#authoritativeRead = { loadId, read };
    return read;
  }

  private async fetchCurrent(
    thisLoad: number,
    minimumCursor?: string,
    acceptResult: () => boolean = () => true,
    anchorEventId?: string,
    replaceWindow = false
  ): Promise<boolean> {
    const source = this.source;
    const deadlineBeforeFetch = this.#threadSlowModeDeadline.get();
    const existingBeforeFetch = snapshotEventFingerprints(this.events);
    this.#loadErrorSignal.set(null);
    try {
      // A removed anchor cannot prevent recovery. Only NotFound falls back;
      // permission and transient failures keep their normal handling below.
      let page;
      try {
        page = anchorEventId
          ? await source.fetchAround(anchorEventId, PAGE_SIZE, undefined, minimumCursor)
          : await source.fetchPage({ limit: PAGE_SIZE, minimumCursor });
      } catch (error) {
        if (!anchorEventId || !isConnectCode(error, Code.NotFound)) throw error;
        if (this.isStale(thisLoad) || !acceptResult()) return false;
        page = await source.fetchPage({ limit: PAGE_SIZE, minimumCursor });
      }
      if (this.isStale(thisLoad) || !acceptResult()) return false;
      if (source.scope === 'room') {
        this.replaceWithSnapshotAndUpdateCursors(page, existingBeforeFetch, {
          deadlineBeforeFetch,
          preserveExistingWindow: !replaceWindow
        });
        this.hasReachedStart = !page.hasOlder;
        if (!minimumCursor) await this.backfillInitialRoomWindow(thisLoad);
      } else {
        // Merge with any subscription events that arrived during the
        // in-flight query (e.g. the user's own reply or a fast cross-user
        // reply). Overwriting would drop them.
        this.replaceWithSnapshotAndUpdateCursors(page, existingBeforeFetch, {
          deadlineBeforeFetch
        });
      }
      if (this.isStale(thisLoad) || !acceptResult()) return false;
      this.#pendingAuthoritativeLoadId = null;
      // A concurrent historical jump still owns its loading state.
      this.isInitialLoading = this.#pendingJumpId !== null;
      // A read without an anchor installs the latest page, also after an
      // earlier read failed.
      this.#needsLatestWindow = anchorEventId ? page.hasNewer : false;
      if (anchorEventId) {
        const recovery = this.recoveryAnchor;
        if (recovery) this.#recoveryAnchorSignal.set({ ...recovery, hasNewer: page.hasNewer });
      }
      return true;
    } catch (error: unknown) {
      if (this.isStale(thisLoad) || !acceptResult()) return false;
      if (
        !(error instanceof StaleResponseError) &&
        !isConnectCode(error, Code.PermissionDenied) &&
        !isConnectCode(error, Code.NotFound)
      ) {
        console.error('MessagesStore: fetchCurrent failed:', error);
      }
      batch(() => {
        this.#pendingAuthoritativeLoadId = null;
        this.isInitialLoading = false;
        const accessLost =
          isConnectCode(error, Code.PermissionDenied) || isConnectCode(error, Code.NotFound);
        if (accessLost && minimumCursor) {
          this.clearForAccessRevocation();
          return;
        }
        if (accessLost) this.clearAnchor();
        this.#loadErrorSignal.set(error);
        this.#needsLatestWindow = true;
      });
      if (
        minimumCursor &&
        !isConnectCode(error, Code.PermissionDenied) &&
        !isConnectCode(error, Code.NotFound)
      ) {
        throw error;
      }
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
    const rootEvent = this.events[rootIdx];
    if (!rootEvent || !isMessagePostedPayload(rootEvent.event)) return;

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

    this.events = this.events.with(rootIdx, {
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
    });
  }

  /**
   * Replace rows through `update`, which returns the same row when it does not
   * change it. Publishes one new array, and only when a row changed.
   */
  private updateEvents(update: (event: TimelineEventView) => TimelineEventView): void {
    let changed = false;
    const next = this.events.map((event) => {
      const updated = update(event);
      if (updated !== event) changed = true;
      return updated;
    });
    if (changed) this.events = next;
  }
}
