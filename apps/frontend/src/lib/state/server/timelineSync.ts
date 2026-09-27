import type { MessageResource } from '$lib/api-client/messageResources';
import type { UserAvatarUserView } from '$lib/render/users';
import { messagePostedPayload } from '$lib/api-client/roomTimeline';
import type { TimelineEventView } from '$lib/render/timelineEvents';
import { Message } from '@chatto/api-types/api/v1/message_types_pb';
import type { MessagesStore } from '$lib/state/room';
import type { RealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
import { MessageReconciler } from './messageReconciler';
import type { RoomStores } from './roomStores.svelte';

/** A message change that this client made, from {@link TimelineSync.applyLocalMutation}. */
export type LocalMessageMutation =
  | 'message-deleted'
  | 'attachment-deleted'
  | 'attachment-description-updated'
  | 'link-preview-deleted';

/** One bounded timeline read. Different anchors or directions need separate reads. */
type WindowRefresh = {
  anchorEventId: string | null;
  forward: boolean;
  minimumCursor?: string;
  generation: number;
};

/** What {@link TimelineSync} needs from its server store. */
export type TimelineSyncOptions = {
  rooms: RoomStores;
  /** Read the current resources of messages, at or after a cursor. */
  readMessages: (roomId: string, ids: string[], cursor?: string) => Promise<MessageResource[]>;
  /** The projection generation. A reset starts a new one and makes older reads stale. */
  generation: () => number;
  /** The cursor of the event that is being applied. Reads for it must reach this cursor. */
  eventCursor: () => string | undefined;
  /**
   * Keep the realtime cursor behind a read until it settles. The read can
   * reject; `track` must handle the failure. The server store reports it only
   * when the read belongs to the current generation.
   */
  track: (read: Promise<unknown>, generation: number) => void;
  /** The author of a posted message: a profile, or none while it loads or after deletion. */
  actor: (userId: string) => { user: UserAvatarUserView | null; deleted: boolean };
};

/**
 * Keeps the loaded room and thread timelines, file lists, and pin lists of one
 * server in step with realtime message events.
 *
 * A message change reads the affected messages once for all loaded views of
 * the room. A room or membership change reloads the window that each loaded
 * timeline shows. Every read for a realtime event goes to `track`, so the
 * realtime cursor does not move past the event before its reads are complete.
 */
export class TimelineSync {
  readonly #options: TimelineSyncOptions;
  readonly #reconciler: MessageReconciler;
  /** Timelines with a window read in progress, and the reads that wait for it. */
  readonly #windowQueues = new WeakMap<MessagesStore, WindowRefresh[]>();

  constructor(options: TimelineSyncOptions) {
    this.#options = options;
    this.#reconciler = new MessageReconciler(options.readMessages, (roomId, cursor) => {
      const timelines = options.rooms
        .timelines(roomId)
        .map((store) => store.captureMessageReconciliation());
      const { files, pins } = options.rooms.loaded(roomId) ?? {};
      return (id, resource, insert) => {
        for (const apply of timelines) apply(id, resource?.timeline ?? null, insert);
        files?.applyMessageUpdate(id, resource?.message ?? null, insert, cursor);
        pins?.applyMessageUpdate(id, resource?.message ?? null, cursor);
      };
    });
  }

  /** Show an authorized public post in the loaded timelines while its read runs. */
  ingestPost(event: RealtimeEvent): void {
    const posted = event.event.case === 'messagePosted' ? event.event.value : null;
    if (!posted || posted.bodyPlaintext === undefined || !event.id) return;
    const { user: actor, deleted: actorDeleted } = event.actorId
      ? this.#options.actor(event.actorId)
      : { user: null, deleted: false };
    const timelineEvent: TimelineEventView = {
      id: event.id,
      createdAt: event.createdAt?.toDate().toISOString() ?? new Date().toISOString(),
      actorId: event.actorId || null,
      actor,
      actorResolution: actorDeleted ? 'deleted' : actor ? undefined : 'loading',
      // The event has only the posted fields. The resource read fills the rest.
      event: messagePostedPayload(
        new Message({
          roomId: posted.roomId,
          body: posted.bodyPlaintext,
          inReplyTo: posted.inReplyTo,
          threadRootEventId: posted.threadRootEventId,
          echoOfEventId: posted.echoOfEventId,
          echoFromThreadRootEventId: posted.echoFromThreadRootEventId
        }),
        {}
      )
    };
    for (const store of this.#options.rooms.timelines(posted.roomId)) {
      store.ingestEvent(timelineEvent);
    }
  }

  /**
   * Reload the window of each loaded timeline of a room, or of all rooms when
   * `roomId` is empty. Thread timelines read the window around `anchorEventId`.
   * Room timelines do the same, or with `roomForward` read the events after
   * their newest loaded event.
   */
  refreshWindows(roomId: string, anchorEventId: string | null, roomForward = false): void {
    const minimumCursor = this.#options.eventCursor();
    const refresh = (store: MessagesStore, anchor: string | null, forward: boolean) => {
      const visibleAnchor = anchor
        ? (store.refreshAnchorForMessageMutation(anchor) ?? anchor)
        : null;
      this.#refreshWindow(store, visibleAnchor, forward, minimumCursor);
    };
    for (const [candidateRoomId, room] of this.#options.rooms.entries()) {
      if (roomId && candidateRoomId !== roomId) continue;
      if (room.messages) refresh(room.messages, anchorEventId, roomForward);
      for (const store of Object.values(room.threads)) refresh(store, anchorEventId, false);
    }
  }

  /**
   * Read a changed message, and the messages that show it, once for every
   * loaded view of its room, including the file lists of closed threads.
   */
  reconcile(roomId: string, id: string, insert = false, threadRootEventId?: string): void {
    if (!roomId || !id) return;
    const rooms = this.#options.rooms;
    const stores = rooms.timelines(roomId);
    const { files, pins } = rooms.loaded(roomId) ?? {};
    if (!stores.length && !files && !pins) return;
    const ids = new Set([id, ...stores.flatMap((store) => store.relatedMessageIds(id))]);
    for (const related of files?.relatedMessageIds(id) ?? []) ids.add(related);
    if (threadRootEventId) ids.add(threadRootEventId);
    const generation = this.#options.generation();
    const cursor = this.#options.eventCursor();
    let read: Promise<void> | undefined;
    for (const target of ids) {
      read = this.#reconciler.enqueue(roomId, target, target === id && insert, cursor);
    }
    if (read) this.#options.track(read, generation);
  }

  /** Remove a retracted message from the loaded views. Without a room, from every timeline. */
  retract(roomId: string, messageEventId: string, retractedAt: string): void {
    if (!messageEventId) return;
    const { files, pins } = this.#options.rooms.loaded(roomId) ?? {};
    files?.applyMessageUpdate(messageEventId, null, false);
    pins?.applyMessageRetraction(messageEventId);
    for (const store of this.#options.rooms.timelines(roomId || undefined)) {
      store.applyMessageRetraction(messageEventId, retractedAt);
    }
  }

  /**
   * Show a message change that this client made before its realtime event
   * arrives. Every loaded timeline of the room gets it, including closed
   * threads. A deletion applies at once; another change reloads the window
   * around the message when a timeline contains it.
   */
  applyLocalMutation(roomId: string, eventId: string, kind: LocalMessageMutation): void {
    for (const store of this.#options.rooms.timelines(roomId)) {
      if (kind === 'message-deleted') {
        store.applyLocalMessageDeletion(eventId);
        continue;
      }
      const anchorEventId = store.refreshAnchorForMessageMutation(eventId);
      if (anchorEventId) void store.refreshCurrentWindow(anchorEventId);
    }
  }

  /** Apply the viewer's follow state of a thread without restarting a thread read. */
  setThreadFollowState(roomId: string, threadRootEventId: string, isFollowing: boolean): void {
    if (!roomId || !threadRootEventId) return;
    const room = this.#options.rooms.loaded(roomId);
    room?.messages?.setThreadRootFollowState(threadRootEventId, isFollowing);
    if (room?.messages) this.reconcile(roomId, threadRootEventId);
    room?.threads[threadRootEventId]?.setThreadRootFollowState(threadRootEventId, isFollowing);
  }

  /** Discard the queued message reads of a room, for example when access to it ends. */
  invalidateRoom(roomId: string): void {
    this.#reconciler.invalidateRoom(roomId);
  }

  /** Discard all queued message reads, for example at a projection reset. */
  reset(): void {
    this.#reconciler.reset();
  }

  /**
   * Run one window read at a time for each timeline. Keep every distinct
   * request of the current generation that arrives while a read runs.
   */
  #refreshWindow(
    store: MessagesStore,
    anchorEventId: string | null,
    forward: boolean,
    minimumCursor: string | undefined
  ): void {
    const generation = this.#options.generation();
    const waiting = this.#windowQueues.get(store);
    if (waiting) {
      // Opaque cursors cannot be sorted. Only identical requests can be dropped.
      const duplicate = waiting.some(
        (request) =>
          request.generation === generation &&
          request.anchorEventId === anchorEventId &&
          request.forward === forward &&
          request.minimumCursor === minimumCursor
      );
      if (!duplicate) waiting.push({ anchorEventId, forward, minimumCursor, generation });
      return;
    }
    this.#windowQueues.set(store, []);
    const read = store
      .refreshCurrentWindow(
        anchorEventId,
        forward,
        minimumCursor,
        () => generation === this.#options.generation()
      )
      .finally(() => {
        // A reset makes the requests of older generations stale.
        const [next, ...rest] = (this.#windowQueues.get(store) ?? []).filter(
          (request) => request.generation === this.#options.generation()
        );
        this.#windowQueues.delete(store);
        if (!next) return;
        this.#refreshWindow(store, next.anchorEventId, next.forward, next.minimumCursor);
        this.#windowQueues.get(store)?.push(...rest);
      });
    this.#options.track(read, generation);
  }
}
