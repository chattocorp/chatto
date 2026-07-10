import { tick } from 'svelte';
import { SvelteDate, SvelteMap, SvelteSet } from 'svelte/reactivity';
import type { RoomEventView } from '$lib/render/types';
import type { EventEnvelope } from '$lib/eventBus.svelte';
import { RoomEventKind, roomEventKind } from '$lib/render/eventKinds';
import { createRoomTimelineAPI, type RoomTimelineAPI } from '$lib/api-client/roomTimeline';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
import type { JumpToMessageState } from '../composerContext.svelte';
import { INITIAL_ROOM_MESSAGE_BACKFILL_TARGET, PAGE_SIZE } from './queries';
import { isRootRoomEvent, isThreadEvent } from './filters';
import { type EventConnectionPage, type RawEvent, getActorId, unmask } from './helpers';

type MessageScope = 'room' | 'thread';
type RoomEventPayload = NonNullable<RoomEventView['event']>;
type MessagePostedPayload = Extract<RoomEventPayload, { kind: typeof RoomEventKind.MessagePosted }>;
type MessageEditedPayload = Extract<RoomEventPayload, { kind: typeof RoomEventKind.MessageEdited }>;
type MessageRetractedPayload = Extract<
  RoomEventPayload,
  { kind: typeof RoomEventKind.MessageRetracted }
>;
type ReactionMutationPayload =
  | Extract<RoomEventPayload, { kind: typeof RoomEventKind.ReactionAdded }>
  | Extract<RoomEventPayload, { kind: typeof RoomEventKind.ReactionRemoved }>;
type MessageReactionSummary = MessagePostedPayload['reactions'][number];
type AssetProcessingPayload =
  | Extract<RoomEventPayload, { kind: typeof RoomEventKind.AssetProcessingStarted }>
  | Extract<RoomEventPayload, { kind: typeof RoomEventKind.AssetProcessingSucceeded }>
  | Extract<RoomEventPayload, { kind: typeof RoomEventKind.AssetProcessingFailed }>;
type RoomDeletedPayload = Extract<RoomEventPayload, { kind: typeof RoomEventKind.RoomDeleted }>;

export type OptimisticReactionAction = 'add' | 'remove';

export type OptimisticReactionServerSummary = {
  emoji: string;
  count: number;
  hasReacted: boolean;
} | null;

export type OptimisticReactionHandle = {
  applyServerReaction(reaction: OptimisticReactionServerSummary): void;
  rollback(): void;
};

export type RefreshCurrentWindowResult = {
  hasOlder: boolean;
  hasNewer: boolean;
  refreshed: boolean;
  changed: boolean;
};

type OptimisticReactionSnapshot = {
  key: string;
  emoji: string;
  previousReaction: MessageReactionSummary | null;
  source: 'events' | 'preview';
  eventId?: string;
  previewKey?: string;
};

function eventCacheKey(roomId: string, eventId: string): string {
  return `${roomId}\u0000${eventId}`;
}

function compareEventCreatedAt(a: RoomEventView, b: RoomEventView): number {
  return Date.parse(a.createdAt) - Date.parse(b.createdAt);
}

function sortRoomEventList(events: RoomEventView[]): RoomEventView[] {
  return events
    .map((event, index) => ({ event, index }))
    .sort((a, b) => compareEventCreatedAt(a.event, b.event) || a.index - b.index)
    .map(({ event }) => event);
}

function sortThreadEventList(events: RoomEventView[], threadRootEventId: string): RoomEventView[] {
  return events
    .map((event, index) => ({ event, index }))
    .sort((a, b) => {
      const aIsRoot = a.event.id === threadRootEventId;
      const bIsRoot = b.event.id === threadRootEventId;
      if (aIsRoot && !bIsRoot) return -1;
      if (!aIsRoot && bIsRoot) return 1;

      const byCreatedAt = compareEventCreatedAt(a.event, b.event);
      return byCreatedAt || a.index - b.index;
    })
    .map(({ event }) => event);
}

function eventFingerprint(event: RoomEventView): string {
  return JSON.stringify(event);
}

function sameEventList(a: readonly RoomEventView[], b: readonly RoomEventView[]): boolean {
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) {
    if (a[i].id !== b[i].id) return false;
    if (eventFingerprint(a[i]) !== eventFingerprint(b[i])) return false;
  }
  return true;
}

function isContinuityEvent(
  event: RoomEventView,
  scope: MessageScope | null,
  threadRootEventId: string
): boolean {
  return scope !== 'thread' || event.id !== threadRootEventId;
}

function skippedRefreshResult(): RefreshCurrentWindowResult {
  return { hasOlder: false, hasNewer: false, refreshed: false, changed: false };
}

function isMessagePostedPayload(
  event: RoomEventView['event'] | EventEnvelope['event'] | null | undefined
): event is MessagePostedPayload {
  return roomEventKind(event) === RoomEventKind.MessagePosted;
}

function isRoomDeletedPayload(event: RoomEventView['event']): event is RoomDeletedPayload {
  return roomEventKind(event) === RoomEventKind.RoomDeleted;
}

function isMessageRetractedPayload(
  event: RoomEventView['event']
): event is MessageRetractedPayload {
  return roomEventKind(event) === RoomEventKind.MessageRetracted;
}

function isMessageEditedPayload(event: RoomEventView['event']): event is MessageEditedPayload {
  return roomEventKind(event) === RoomEventKind.MessageEdited;
}

function isReactionMutationPayload(
  event: RoomEventView['event']
): event is ReactionMutationPayload {
  const kind = roomEventKind(event);
  return kind === RoomEventKind.ReactionAdded || kind === RoomEventKind.ReactionRemoved;
}

function isAssetProcessingPayload(event: RoomEventView['event']): event is AssetProcessingPayload {
  const kind = roomEventKind(event);
  return (
    kind === RoomEventKind.AssetProcessingStarted ||
    kind === RoomEventKind.AssetProcessingSucceeded ||
    kind === RoomEventKind.AssetProcessingFailed
  );
}

function roomTimelineFromServerConnection(serverConnection: ServerConnection): RoomTimelineAPI {
  const candidate = serverConnection as {
    serverId?: string;
    connectBaseUrl?: string;
    bearerToken?: string | null;
  };
  if (!candidate.connectBaseUrl) {
    throw new Error('MessagesStore requires the ConnectRPC timeline API');
  }
  return createRoomTimelineAPI({
    serverId: candidate.serverId,
    baseUrl: candidate.connectBaseUrl,
    bearerToken: candidate.bearerToken ?? null
  });
}

/**
 * Message store for both the main room timeline and a single thread pane.
 * Room history uses the protobuf ConnectRPC timeline API when available;
 * thread history requires that path. Lifecycle, pagination, refetch, and
 * realtime ingestion behavior stays shared across both scopes.
 */
export class MessagesStore {
  events = $state<RoomEventView[]>([]);
  isInitialLoading = $state(true);
  isLoadingMore = $state(false);
  hasReachedStart = $state(false);

  private readonly roomTimeline: RoomTimelineAPI;
  private scope: MessageScope | null = null;
  private threadRootEventId = '';
  private seenIds: SvelteSet<string> = new SvelteSet<string>();
  private previewEvents = new SvelteMap<string, RoomEventView | null>();
  private pendingPreviewFetches = new SvelteMap<string, Promise<void>>();
  private roomId = '';
  private oldestCursor: string | undefined;
  private newestCursor: string | undefined;
  private optimisticReactionVersion = 0;
  private optimisticReactionVersions = new SvelteMap<string, number>();

  /** Increments on every load kickoff. Async callbacks compare against
   *  it via {@link isStale} to discard results from superseded loads. */
  #loadId = 0;

  constructor(
    serverConnection: ServerConnection,
    private readonly getCurrentUserId: () => string | null,
    roomTimeline?: RoomTimelineAPI
  ) {
    this.roomTimeline = roomTimeline ?? roomTimelineFromServerConnection(serverConnection);
  }

  /** Tear down lifecycle listeners. Idempotent. */
  dispose(): void {
    // The message store has no owned subscriptions. Server-event replay is
    // managed by the singleton event bus.
  }

  /** Root-level events only (excludes thread replies). */
  get rootEvents(): RoomEventView[] {
    return this.events.filter(isRootRoomEvent);
  }

  /** Events that belong to this thread (root + replies). */
  get threadEvents(): RoomEventView[] {
    return this.events.filter((e) => isThreadEvent(e, this.roomId, this.threadRootEventId));
  }

  /** Look up an event already known to this room, including off-window preview targets. */
  getEventById(eventId: string): RoomEventView | null | undefined {
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
    const token = ++this.optimisticReactionVersion;
    const targetIds = this.optimisticReactionTargetIds(input.messageEventId);
    const snapshots: OptimisticReactionSnapshot[] = [];

    const record = (snapshot: OptimisticReactionSnapshot) => {
      snapshots.push(snapshot);
      this.optimisticReactionVersions.set(snapshot.key, token);
    };

    for (let i = 0; i < this.events.length; i++) {
      const event = this.events[i];
      if (!this.isReactionTarget(event, targetIds)) continue;
      const updated = this.eventWithOptimisticReaction(event, input.emoji, input.action);
      if (!updated) continue;
      const key = this.optimisticEventKey(event.id, input.emoji);
      record({
        key,
        emoji: input.emoji,
        previousReaction: this.reactionSummary(event, input.emoji),
        source: 'events',
        eventId: event.id
      });
      this.events[i] = updated;
    }

    for (const [previewKey, event] of this.previewEvents) {
      if (!event || !this.isReactionTarget(event, targetIds)) continue;
      const updated = this.eventWithOptimisticReaction(event, input.emoji, input.action);
      if (!updated) continue;
      const key = this.optimisticPreviewKey(previewKey, input.emoji);
      record({
        key,
        emoji: input.emoji,
        previousReaction: this.reactionSummary(event, input.emoji),
        source: 'preview',
        previewKey
      });
      this.previewEvents.set(previewKey, updated);
    }

    return {
      applyServerReaction: (reaction) => {
        for (const snapshot of snapshots) {
          if (this.optimisticReactionVersions.get(snapshot.key) !== token) continue;
          this.applyServerReactionSnapshot(snapshot, input.emoji, reaction);
          this.optimisticReactionVersions.delete(snapshot.key);
        }
      },
      rollback: () => {
        for (const snapshot of snapshots) {
          if (this.optimisticReactionVersions.get(snapshot.key) !== token) continue;
          this.restoreOptimisticReactionSnapshot(snapshot);
          this.optimisticReactionVersions.delete(snapshot.key);
        }
      }
    };
  }

  /** Update the viewer's thread follow state on a known thread root event. */
  setThreadRootFollowState(threadRootEventId: string, isFollowing: boolean): void {
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
    if (this.events.some((e) => e.id === eventId)) return undefined;

    const key = this.previewKey(eventId);
    if (this.previewEvents.has(key)) return undefined;

    const existing = this.pendingPreviewFetches.get(key);
    if (existing) return existing;

    const promise = this.fetchEventById(eventId)
      .then((event) => {
        if (event) this.clearOptimisticVersionForEvent(event.id);
        this.previewEvents.set(key, event);
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

  private optimisticEventKey(eventId: string, emoji: string): string {
    return `${this.optimisticEventKeyPrefix(eventId)}${emoji}`;
  }

  private optimisticEventKeyPrefix(eventId: string): string {
    return `events:${eventId}\u0000`;
  }

  private optimisticPreviewKey(previewKey: string, emoji: string): string {
    return `${this.optimisticPreviewKeyPrefix(previewKey)}${emoji}`;
  }

  private optimisticPreviewKeyPrefix(previewKey: string): string {
    return `preview:${previewKey}\u0000`;
  }

  private clearOptimisticVersionForEvent(eventId: string): void {
    const eventPrefix = this.optimisticEventKeyPrefix(eventId);
    const previewPrefix = this.optimisticPreviewKeyPrefix(this.previewKey(eventId));
    for (const key of this.optimisticReactionVersions.keys()) {
      if (key.startsWith(eventPrefix) || key.startsWith(previewPrefix)) {
        this.optimisticReactionVersions.delete(key);
      }
    }
  }

  private optimisticReactionTargetIds(messageEventId: string): SvelteSet<string> {
    const targetIds = new SvelteSet([messageEventId]);
    let changed = true;

    while (changed) {
      changed = false;
      for (const event of this.events) {
        changed = this.addLinkedReactionTargetIds(event, targetIds) || changed;
      }
      for (const event of this.previewEvents.values()) {
        if (event) changed = this.addLinkedReactionTargetIds(event, targetIds) || changed;
      }
    }

    return targetIds;
  }

  private addLinkedReactionTargetIds(event: RoomEventView, targetIds: SvelteSet<string>): boolean {
    const payload = event.event;
    if (!isMessagePostedPayload(payload)) return false;

    const before = targetIds.size;
    if (targetIds.has(event.id)) {
      if (payload.echoOfEventId) targetIds.add(payload.echoOfEventId);
      if (payload.channelEchoEventId) targetIds.add(payload.channelEchoEventId);
    }
    if (payload.echoOfEventId && targetIds.has(payload.echoOfEventId)) targetIds.add(event.id);
    if (payload.channelEchoEventId && targetIds.has(payload.channelEchoEventId)) {
      targetIds.add(event.id);
    }
    return targetIds.size !== before;
  }

  private isReactionTarget(event: RoomEventView, targetIds: SvelteSet<string>): boolean {
    if (targetIds.has(event.id)) return true;
    const payload = event.event;
    return (
      isMessagePostedPayload(payload) &&
      Boolean(
        (payload.echoOfEventId && targetIds.has(payload.echoOfEventId)) ||
          (payload.channelEchoEventId && targetIds.has(payload.channelEchoEventId))
      )
    );
  }

  private eventWithOptimisticReaction(
    event: RoomEventView,
    emoji: string,
    action: OptimisticReactionAction
  ): RoomEventView | null {
    const payload = event.event;
    if (!isMessagePostedPayload(payload)) return null;
    return {
      ...event,
      event: {
        ...payload,
        reactions: this.optimisticReactions(payload.reactions, emoji, action)
      }
    };
  }

  private reactionSummary(event: RoomEventView, emoji: string): MessageReactionSummary | null {
    const payload = event.event;
    if (!isMessagePostedPayload(payload)) return null;
    return payload.reactions.find((reaction) => reaction.emoji === emoji) ?? null;
  }

  private eventWithReactionSummary(
    event: RoomEventView,
    emoji: string,
    reaction: MessageReactionSummary | null
  ): RoomEventView | null {
    const payload = event.event;
    if (!isMessagePostedPayload(payload)) return null;
    return {
      ...event,
      event: {
        ...payload,
        reactions: this.reactionsWithSummary(payload.reactions, emoji, reaction)
      }
    };
  }

  private optimisticReactions(
    reactions: readonly MessageReactionSummary[],
    emoji: string,
    action: OptimisticReactionAction
  ): MessageReactionSummary[] {
    const existingIndex = reactions.findIndex((reaction) => reaction.emoji === emoji);
    if (action === 'add') {
      if (existingIndex === -1) {
        return [...reactions, { emoji, count: 1, hasReacted: true, users: [] }];
      }

      return reactions.map((reaction, index) =>
        index === existingIndex
          ? {
              ...reaction,
              count: reaction.hasReacted ? reaction.count : reaction.count + 1,
              hasReacted: true
            }
          : reaction
      );
    }

    if (existingIndex === -1) return [...reactions];

    return reactions.flatMap((reaction, index) => {
      if (index !== existingIndex) return [reaction];
      const count = reaction.hasReacted ? Math.max(0, reaction.count - 1) : reaction.count;
      if (count === 0) return [];
      return [{ ...reaction, count, hasReacted: false }];
    });
  }

  private reactionsWithSummary(
    reactions: readonly MessageReactionSummary[],
    emoji: string,
    reaction: MessageReactionSummary | null
  ): MessageReactionSummary[] {
    if (!reaction || reaction.count <= 0) {
      return reactions.filter((existing) => existing.emoji !== emoji);
    }

    const existingIndex = reactions.findIndex((existing) => existing.emoji === emoji);
    const nextReaction = {
      ...reaction,
      users: [...reaction.users]
    };
    if (existingIndex === -1) return [...reactions, nextReaction];
    return reactions.map((existing, index) => (index === existingIndex ? nextReaction : existing));
  }

  private serverReactions(
    reactions: readonly MessageReactionSummary[],
    emoji: string,
    reaction: OptimisticReactionServerSummary
  ): MessageReactionSummary[] {
    if (!reaction || reaction.count <= 0) {
      return reactions.filter((existing) => existing.emoji !== emoji);
    }

    const existingIndex = reactions.findIndex((existing) => existing.emoji === emoji);
    const nextReaction = (existing?: MessageReactionSummary): MessageReactionSummary => ({
      emoji: reaction.emoji,
      count: reaction.count,
      hasReacted: reaction.hasReacted,
      users: existing?.users ?? []
    });

    if (existingIndex === -1) return [...reactions, nextReaction()];
    return reactions.map((existing, index) =>
      index === existingIndex ? nextReaction(existing) : existing
    );
  }

  private applyServerReactionSnapshot(
    snapshot: OptimisticReactionSnapshot,
    emoji: string,
    reaction: OptimisticReactionServerSummary
  ): void {
    const apply = (event: RoomEventView): RoomEventView | null => {
      const payload = event.event;
      if (!isMessagePostedPayload(payload)) return null;
      return {
        ...event,
        event: {
          ...payload,
          reactions: this.serverReactions(payload.reactions, emoji, reaction)
        }
      };
    };

    if (snapshot.source === 'events') {
      const index = this.events.findIndex((event) => event.id === snapshot.eventId);
      if (index === -1) return;
      const updated = apply(this.events[index]);
      if (updated) this.events[index] = updated;
      return;
    }

    if (!snapshot.previewKey) return;
    const event = this.previewEvents.get(snapshot.previewKey);
    if (!event) return;
    const updated = apply(event);
    if (updated) this.previewEvents.set(snapshot.previewKey, updated);
  }

  private restoreOptimisticReactionSnapshot(snapshot: OptimisticReactionSnapshot): void {
    if (snapshot.source === 'events') {
      const index = this.events.findIndex((event) => event.id === snapshot.eventId);
      if (index === -1) return;
      const updated = this.eventWithReactionSummary(
        this.events[index],
        snapshot.emoji,
        snapshot.previousReaction
      );
      if (updated) this.events[index] = updated;
      return;
    }

    if (!snapshot.previewKey) return;
    const event = this.previewEvents.get(snapshot.previewKey);
    if (!event) return;
    const updated = this.eventWithReactionSummary(
      event,
      snapshot.emoji,
      snapshot.previousReaction
    );
    if (updated) this.previewEvents.set(snapshot.previewKey, updated);
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
    // using the RoomEventView shape it renders with.
    const spaceEvent = serverEvent as unknown as RoomEventView;
    this.ingestEvent(spaceEvent);
  }

  /**
   * Route an already-renderable event into the store. Used for read-your-writes
   * after mutations that return the posted event; live subscription delivery
   * still follows {@link ingestServerEvent} and is deduped by event ID.
   */
  ingestEvent(spaceEvent: RoomEventView): void {
    const eventData = spaceEvent.event;
    if (!eventData) return;
    const kind = roomEventKind(eventData);

    if (kind === RoomEventKind.ServerMemberDeleted) {
      this.refetchAll();
      return;
    }

    if (isRoomDeletedPayload(eventData)) {
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

    if (isMessageRetractedPayload(eventData)) {
      this.applyDeletion(eventData.messageEventId, spaceEvent.createdAt);
      return;
    }

    if (isMessageEditedPayload(eventData)) {
      if (!('body' in eventData)) {
        void this.refetchByMessageEventId(eventData.messageEventId);
        return;
      }
      this.applyEdit(eventData.messageEventId, eventData);
      return;
    }

    if (isReactionMutationPayload(eventData)) {
      this.refetchByMessageEventId(eventData.messageEventId);
      return;
    }

    if (isAssetProcessingPayload(eventData)) {
      if (!eventData.processingMessageEventId) return;
      this.refetchByMessageEventId(eventData.processingMessageEventId);
      return;
    }

    if (isMessagePostedPayload(eventData)) {
      if (!('body' in eventData)) {
        const messageEventId =
          'messageEventId' in eventData && typeof eventData.messageEventId === 'string'
            ? eventData.messageEventId
            : spaceEvent.id;
        void this.fetchAndIngestMessagePostedSignal(
          messageEventId,
          eventData.threadRootEventId ?? null
        );
        return;
      }
      this.onMessagePosted(spaceEvent, eventData);
      return;
    }

    if (
      kind === RoomEventKind.UserJoinedRoom ||
      kind === RoomEventKind.UserLeftRoom ||
      kind === RoomEventKind.RoomUpdated ||
      kind === RoomEventKind.RoomArchived ||
      kind === RoomEventKind.RoomUnarchived
    ) {
      if (!spaceEvent.actor && this.roomTimeline) {
        void this.fetchAndIngestSystemEvent(spaceEvent.id);
        return;
      }
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
        this.afterOlderPagePrepended();
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
      return this.roomTimeline.getThreadEvents({
        roomId: this.roomId,
        threadRootEventId: this.threadRootEventId,
        limit: PAGE_SIZE,
        before
      });
    }

    return this.roomTimeline.getRoomEvents({
      roomId: this.roomId,
      limit: PAGE_SIZE,
      before
    });
  }

  private afterOlderPagePrepended(): void {
    if (this.scope === 'thread') {
      this.sortThreadEvents();
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
    if (this.scope !== 'room') return;
    if (jumpState.isLoadingNewer || jumpState.hasReachedEnd) return;
    if (!this.newestCursor) return;

    jumpState.isLoadingNewer = true;
    try {
      const page = await this.roomTimeline.getRoomEvents({
        roomId: this.roomId,
        limit: PAGE_SIZE,
        after: this.newestCursor
      });

      // User left jumped mode while in flight — abandon the result.
      if (!jumpState.isJumpedMode) return;

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
      const around = await this.roomTimeline.getRoomEventsAround({
        roomId: this.roomId,
        eventId,
        limit: PAGE_SIZE
      });

      const { events: rawEvents, hasOlder, hasNewer, startCursor, endCursor } = around;
      const parsed = unmask(rawEvents);

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
    if (!this.scope || !this.roomId) return skippedRefreshResult();

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
      if (this.isStale(thisLoad)) return skippedRefreshResult();
      console.error('MessagesStore: refreshCurrentWindow failed:', error);
      return skippedRefreshResult();
    }
  }

  private onMessagePosted(
    spaceEvent: RoomEventView,
    eventData: Extract<RoomEventView['event'], { kind: typeof RoomEventKind.MessagePosted }>
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
      if (this.seenIds.has(spaceEvent.id)) return;
      this.seenIds.add(spaceEvent.id);
      this.applyThreadReplyToRoot(spaceEvent, eventData);
      return;
    }
    this.addEvent(spaceEvent);
  }

  private onSystemEvent(spaceEvent: RoomEventView): void {
    if (this.scope === 'room') {
      this.addEvent(spaceEvent);
    }
  }

  private async fetchAndIngestSystemEvent(eventId: string): Promise<void> {
    const fetched = await this.fetchEventById(eventId);
    if (fetched) {
      this.ingestEvent(fetched);
    }
  }

  private async fetchAndIngestMessagePostedSignal(
    messageEventId: string,
    threadRootEventId: string | null
  ): Promise<void> {
    const fetched = await this.fetchEventById(messageEventId, threadRootEventId);
    if (fetched) {
      this.ingestEvent(fetched);
      return;
    }

    if (this.scope === 'room' && threadRootEventId) {
      await this.refetchOne(threadRootEventId);
    }
  }

  private async fetchEventById(
    eventId: string,
    threadRootEventId?: string | null
  ): Promise<RoomEventView | null> {
    const page = threadRootEventId
      ? await this.roomTimeline.getThreadEventsAround({
          roomId: this.roomId,
          threadRootEventId,
          eventId,
          limit: 1
        })
      : await this.roomTimeline.getRoomEventsAround({
          roomId: this.roomId,
          eventId,
          limit: 1
        });
    return unmask(page.events).find((event) => event.id === eventId) ?? null;
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

  private async refetchByMessageEventId(messageEventId: string): Promise<void> {
    // Match either the direct event id or an echo whose original points here.
    for (const e of this.events) {
      const evt = e.event;
      if (
        e.id === messageEventId ||
        (isMessagePostedPayload(evt) && evt.echoOfEventId === messageEventId)
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
  private applyDeletion(messageEventId: string, deletedAt: string): void {
    this.clearChannelEchoLink(messageEventId);

    const targetIndex = this.events.findIndex((e) => e.id === messageEventId);
    const target = targetIndex === -1 ? null : this.events[targetIndex];
    const targetPayload = target?.event;
    if (isMessagePostedPayload(targetPayload) && targetPayload.echoOfEventId) {
      this.events.splice(targetIndex, 1);
      this.seenIds.delete(messageEventId);
      return;
    }

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

    const previewKey = this.previewKey(messageEventId);
    const preview = this.previewEvents.get(previewKey);
    if (isMessagePostedPayload(preview?.event)) {
      this.previewEvents.set(previewKey, {
        ...preview,
        event: {
          ...preview.event,
          body: null,
          attachments: [],
          linkPreview: null,
          deletedAt
        }
      });
    }
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

  /**
   * Apply an edit payload directly to the matching MessagePostedEvent. The
   * backend emits one canonical edit event per linked post/echo, so we only
   * patch the direct event ID here; the linked event will arrive separately.
   */
  private applyEdit(messageEventId: string, edit: MessageEditedPayload): void {
    for (let i = 0; i < this.events.length; i++) {
      const e = this.events[i];
      const evt = e.event;
      if (!isMessagePostedPayload(evt)) continue;
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
    if (isMessagePostedPayload(preview?.event)) {
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

  private addEvent(event: RoomEventView, options: { sortRoom?: boolean } = {}): boolean {
    if (this.seenIds.has(event.id)) return false;
    this.seenIds.add(event.id);
    this.events.push(event);
    if ((options.sortRoom ?? true) && this.scope === 'room') this.sortRoomEvents();
    return true;
  }

  private appendMany(events: RoomEventView[]): void {
    let added = false;
    for (const e of events) {
      this.clearOptimisticVersionForEvent(e.id);
      added = this.addEvent(e, { sortRoom: false }) || added;
    }
    if (added && this.scope === 'room') this.sortRoomEvents();
  }

  private prependEvents(olderEvents: RoomEventView[]): number {
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
   * eventBus subscription has been live since layout mount, so any
   * MessagePostedEvent for this room that lands while the query is in
   * flight has already been added to {@link events} via
   * {@link ingestServerEvent} and must not be wiped by the result.
   */
  private replaceMergingExisting(rawEvents: readonly RawEvent[]): void {
    const fetched = unmask(rawEvents);
    const newSeen = new SvelteSet<string>();
    const merged: RoomEventView[] = [];
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
    if (this.scope === 'room') this.sortRoomEvents();
    this.seenIds = newSeen;
  }

  private resetState(): void {
    this.events = [];
    this.seenIds = new SvelteSet();
    this.previewEvents.clear();
    this.pendingPreviewFetches.clear();
    this.optimisticReactionVersions.clear();
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
    existingBeforeFetch: ReadonlySet<string>,
    options: { preserveExistingWindow?: boolean; latestSnapshot?: boolean } = {}
  ): boolean {
    const fetched = unmask(connection.events);
    const newSeen = new SvelteSet<string>();
    const merged: RoomEventView[] = [];
    const previousOldestCursor = this.oldestCursor;
    const previousNewestCursor = this.newestCursor;
    const previousHasReachedStart = this.hasReachedStart;
    const hasExistingContinuityEvents = this.events.some(
      (event) =>
        existingBeforeFetch.has(event.id) &&
        isContinuityEvent(event, this.scope, this.threadRootEventId)
    );
    const hasFetchedOverlap = fetched.some(
      (event) =>
        existingBeforeFetch.has(event.id) &&
        isContinuityEvent(event, this.scope, this.threadRootEventId)
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
      merged.push(e);
    }

    // Preserve subscription events that arrived while the refresh query was in
    // flight. Anchored refreshes also preserve already-loaded rows outside the
    // fetched window so returning from another tab does not visually collapse a
    // long scrolled buffer.
    for (const e of this.events) {
      if (
        (!options.preserveExistingWindow || discontinuousLatestSnapshot) &&
        existingBeforeFetch.has(e.id)
      ) {
        continue;
      }
      if (newSeen.has(e.id)) continue;
      newSeen.add(e.id);
      merged.push(e);
    }

    const nextEvents =
      this.scope === 'room'
        ? sortRoomEventList(merged)
        : this.scope === 'thread'
          ? sortThreadEventList(merged, this.threadRootEventId)
          : merged;
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

  private async refreshRoomLatest(
    thisLoad: number,
    existingBeforeFetch: ReadonlySet<string>
  ): Promise<RefreshCurrentWindowResult> {
    const page = await this.roomTimeline.getRoomEvents({
      roomId: this.roomId,
      limit: PAGE_SIZE
    });

    if (this.isStale(thisLoad)) return skippedRefreshResult();
    const changed = this.replaceWithSnapshotAndUpdateCursors(page, existingBeforeFetch, {
      preserveExistingWindow: true,
      latestSnapshot: true
    });
    return { hasOlder: page.hasOlder, hasNewer: page.hasNewer, refreshed: true, changed };
  }

  private async refreshRoomAround(
    thisLoad: number,
    anchorEventId: string,
    existingBeforeFetch: ReadonlySet<string>
  ): Promise<RefreshCurrentWindowResult | null> {
    const page = await this.roomTimeline.getRoomEventsAround({
      roomId: this.roomId,
      eventId: anchorEventId,
      limit: PAGE_SIZE
    });

    if (this.isStale(thisLoad)) return skippedRefreshResult();
    const changed = this.replaceWithSnapshotAndUpdateCursors(page, existingBeforeFetch, {
      preserveExistingWindow: true
    });
    return { hasOlder: page.hasOlder, hasNewer: page.hasNewer, refreshed: true, changed };
  }

  private async refreshThreadWindow(
    thisLoad: number,
    existingBeforeFetch: ReadonlySet<string>,
    anchorEventId: string | null
  ): Promise<RefreshCurrentWindowResult> {
    const page = anchorEventId
      ? await this.roomTimeline.getThreadEventsAround({
          roomId: this.roomId,
          threadRootEventId: this.threadRootEventId,
          eventId: anchorEventId,
          limit: PAGE_SIZE
        })
      : await this.roomTimeline.getThreadEvents({
          roomId: this.roomId,
          threadRootEventId: this.threadRootEventId,
          limit: PAGE_SIZE
        });
    if (this.isStale(thisLoad)) return skippedRefreshResult();
    const changed = this.replaceWithSnapshotAndUpdateCursors(page, existingBeforeFetch, {
      preserveExistingWindow: anchorEventId === null || anchorEventId !== this.threadRootEventId,
      latestSnapshot: anchorEventId === null
    });
    return { hasOlder: page.hasOlder, hasNewer: page.hasNewer, refreshed: true, changed };
  }

  private resetAndFetchLatest(): void {
    const thisLoad = this.startLoad();
    this.resetState();
    this.isInitialLoading = true;
    this.fetchLatest(thisLoad);
  }

  private fetchLatest(thisLoad: number): void {
    const promise = this.roomTimeline.getRoomEvents({
      roomId: this.roomId,
      limit: PAGE_SIZE
    });

    promise
      .then(async (page) => {
        if (this.isStale(thisLoad)) return;
        if (page) {
          this.replaceWithFetchedAndUpdateCursors(page);
          this.hasReachedStart = !page.hasOlder;
          await this.backfillInitialRoomWindow(thisLoad);
        }
        if (this.isStale(thisLoad)) return;
        this.isInitialLoading = false;
      })
      .catch((error: unknown) => {
        if (this.isStale(thisLoad)) return;
        console.error('MessagesStore: fetchLatest failed:', error);
        this.isInitialLoading = false;
      });
  }

  private fetchThread(thisLoad: number): void {
    const promise = this.roomTimeline.getThreadEvents({
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

  /**
   * Mirror the backend's auto-follow behavior on the root message when a
   * thread reply arrives, so the UI updates instantly without refetching.
   */
  private applyThreadReplyToRoot(spaceEvent: RoomEventView, eventData: MessagePostedPayload): void {
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

  private sortThreadEvents(): void {
    this.events = sortThreadEventList(this.events, this.threadRootEventId);
  }

  private sortRoomEvents(): void {
    this.events = sortRoomEventList(this.events);
  }
}
