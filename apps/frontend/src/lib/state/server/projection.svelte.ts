import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import { RoomTimelineIncludes, RoomTimelinePage } from '@chatto/api-types/api/v1/room_timeline_pb';
import { DirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import { ThreadViewerState } from '@chatto/api-types/api/v1/message_types_pb';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { RoomWithViewerState, type RoomGroup } from '@chatto/api-types/api/v1/room_directory_pb';
import type { ServerPublicProfile } from '@chatto/api-types/api/v1/server_pb';
import type { GetViewerResponse } from '@chatto/api-types/api/v1/viewer_pb';
import type { ListNotificationsResponse } from '@chatto/api-types/api/v1/notifications_pb';
import type { ActiveCall } from '@chatto/api-types/api/v1/voice_calls_pb';
import { RealtimeProjectionRoom } from '@chatto/api-types/realtime/v1/realtime_pb';
import type {
  RealtimeProjectionEvent,
  RealtimeProjectionServerState
} from '@chatto/api-types/realtime/v1/realtime_pb';

/** Canonical protobuf-native state for one connected Chatto server. */
export class ServerProjectionStore {
  server = $state.raw<ServerPublicProfile | null>(null);
  serverState = $state.raw<RealtimeProjectionServerState | null>(null);
  viewer = $state.raw<GetViewerResponse | null>(null);
  users = new SvelteMap<string, DirectoryMember>();
  rooms = new SvelteMap<string, RealtimeProjectionRoom>();
  roomGroups = $state.raw<RoomGroup[]>([]);
  notifications = $state.raw<ListNotificationsResponse | null>(null);
  activeCalls = $state.raw<ActiveCall[]>([]);
  /** Complete current followed-thread viewer state, keyed by room and root ID. */
  threadViewerStates = new SvelteMap<string, ThreadViewerState>();
  timelines = new SvelteMap<string, RoomTimelinePage>();
  private timelineEventCursors = new SvelteMap<string, SvelteMap<string, string>>();
  private revokedRoomIds = new SvelteSet<string>();

  apply(event: RealtimeProjectionEvent): void {
    // Validate the entire atomic event before mutating anything. An unknown
    // operation must fail the subscription without partially applying state
    // or advancing its cursor.
    for (const operation of event.operations) {
      switch (operation.operation.case) {
        case 'reset':
        case 'serverUpsert':
        case 'serverStateUpsert':
        case 'viewerUpsert':
        case 'userUpsert':
        case 'userRemove':
        case 'roomUpsert':
        case 'roomRemove':
        case 'roomGroupsReplace':
        case 'roomTimelineReplace':
        case 'roomTimelineEventUpsert':
        case 'roomTimelineEventRemove':
        case 'notificationsReplace':
        case 'roomViewerStateReplace':
        case 'activeCallsReplace':
        case 'presencesReplace':
        case 'threadViewerStatesReplace':
        case 'roomActivity':
          break;
        case undefined:
          throw new Error('unsupported realtime projection operation');
      }
    }
    for (const operation of event.operations) {
      switch (operation.operation.case) {
        case 'reset':
          this.reset();
          break;
        case 'serverUpsert':
          this.server = operation.operation.value;
          break;
        case 'serverStateUpsert':
          this.serverState = operation.operation.value;
          break;
        case 'viewerUpsert':
          this.viewer = operation.operation.value;
          break;
        case 'userUpsert': {
          const member = operation.operation.value;
          const userId = member.user?.id;
          if (userId) this.users.set(userId, member);
          break;
        }
        case 'userRemove':
          this.removeUser(operation.operation.value.userId);
          break;
        case 'roomUpsert': {
          const room = operation.operation.value;
          const roomId = room.room?.room?.id;
          if (roomId) {
            this.rooms.set(roomId, room);
            if (room.room?.viewerState?.isMember === false) {
              this.revokedRoomIds.add(roomId);
              this.timelines.delete(roomId);
              this.timelineEventCursors.delete(roomId);
              this.removeActiveCallRoom(roomId);
            } else if (room.room?.viewerState?.isMember === true) this.revokedRoomIds.delete(roomId);
          }
          break;
        }
        case 'roomRemove':
          this.revokedRoomIds.add(operation.operation.value.roomId);
          this.rooms.delete(operation.operation.value.roomId);
          this.timelines.delete(operation.operation.value.roomId);
          this.timelineEventCursors.delete(operation.operation.value.roomId);
          this.removeActiveCallRoom(operation.operation.value.roomId);
          break;
        case 'roomGroupsReplace':
          this.roomGroups = [...operation.operation.value.groups];
          break;
        case 'roomTimelineReplace': {
          const replacement = operation.operation.value;
          if (replacement.page && !this.revokedRoomIds.has(replacement.roomId)) {
            this.timelines.set(replacement.roomId, replacement.page);
            this.seedTimelineEventCursors(
              replacement.roomId,
              replacement.page,
              replacement.eventCursors
            );
          }
          break;
        }
        case 'roomTimelineEventUpsert': {
          const update = operation.operation.value;
          if (!this.revokedRoomIds.has(update.roomId)) this.upsertTimelineEvent(update);
          break;
        }
        case 'roomTimelineEventRemove':
          this.removeTimelineEvent(
            operation.operation.value.roomId,
            operation.operation.value.eventId
          );
          break;
        case 'notificationsReplace': {
          const replacement = operation.operation.value;
          this.notifications = replacement.page ?? null;
          const counts = Object.fromEntries(
            replacement.roomCounts.map((count) => [count.roomId, count.totalCount])
          );
          for (const [roomId, current] of this.rooms) {
            this.rooms.set(
              roomId,
              new RealtimeProjectionRoom({
                room: current.room,
                memberUserIds: [...current.memberUserIds],
                viewerNotificationCount: Math.max(0, counts[roomId] ?? 0)
              })
            );
          }
          break;
        }
        case 'roomViewerStateReplace': {
          const replacement = operation.operation.value;
          const current = this.rooms.get(replacement.roomId);
          if (current) {
            this.rooms.set(
              replacement.roomId,
              new RealtimeProjectionRoom({
                room: new RoomWithViewerState({
                  room: current.room?.room,
                  viewerState: replacement.viewerState
                }),
                memberUserIds: [...current.memberUserIds],
                viewerNotificationCount: current.viewerNotificationCount
              })
            );
          }
          if (replacement.viewerState?.isMember === false) {
            this.revokedRoomIds.add(replacement.roomId);
            this.timelines.delete(replacement.roomId);
            this.timelineEventCursors.delete(replacement.roomId);
            this.removeActiveCallRoom(replacement.roomId);
          } else if (replacement.viewerState?.isMember === true) {
            this.revokedRoomIds.delete(replacement.roomId);
          }
          break;
        }
        case 'activeCallsReplace':
          this.activeCalls = [...operation.operation.value.calls];
          break;
        case 'presencesReplace':
          for (const [userId, member] of this.users) {
            if (!member.user) continue;
            const user = member.user.clone();
            user.presenceStatus =
              operation.operation.value.statuses[userId] ?? PresenceStatus.OFFLINE;
            this.users.set(
              userId,
              new DirectoryMember({ user, roles: [...member.roles], createdAt: member.createdAt })
            );
          }
          break;
        case 'threadViewerStatesReplace': {
          this.threadViewerStates.clear();
          for (const state of operation.operation.value.states) {
            this.threadViewerStates.set(
              `${state.roomId}\u0000${state.threadRootEventId}`,
              state.viewerState ?? new ThreadViewerState()
            );
          }
          for (const [roomId, page] of this.timelines) {
            let changed = false;
            const events = page.events.map((event) => {
              if (event.event.case !== 'messagePosted') return event;
              const thread = event.event.value.message?.thread;
              if (!thread?.threadRootEventId) return event;
              const next = event.clone();
              const nextThread =
                next.event.case === 'messagePosted' ? next.event.value.message?.thread : undefined;
              if (!nextThread) return event;
              nextThread.viewerState =
                this.threadViewerStates
                  .get(`${roomId}\u0000${thread.threadRootEventId}`)
                  ?.clone() ?? new ThreadViewerState({ isFollowing: false, hasUnread: false });
              changed = true;
              return next;
            });
            if (changed) {
              this.timelines.set(
                roomId,
                new RoomTimelinePage({
                  events,
                  startCursor: page.startCursor,
                  endCursor: page.endCursor,
                  hasOlder: page.hasOlder,
                  hasNewer: page.hasNewer,
                  includes: page.includes
                })
              );
            }
          }
          break;
        }
        case 'roomActivity':
          this.bumpRoom(operation.operation.value.roomId);
          break;
        case undefined:
          throw new Error('unsupported realtime projection operation');
      }
    }
  }

  /** Drop one LRU timeline and optionally demote eager channel membership. */
  evictRoomTimeline(roomId: string, clearMembership: boolean): void {
    this.timelines.delete(roomId);
    this.timelineEventCursors.delete(roomId);
    if (!clearMembership) return;
    const room = this.rooms.get(roomId);
    if (!room) return;
    this.rooms.set(
      roomId,
      new RealtimeProjectionRoom({
        room: room.room,
        memberUserIds: [],
        viewerNotificationCount: room.viewerNotificationCount
      })
    );
  }

  reset(): void {
    this.server = null;
    this.serverState = null;
    this.viewer = null;
    this.users.clear();
    this.rooms.clear();
    this.roomGroups = [];
    this.notifications = null;
    this.activeCalls = [];
    this.threadViewerStates.clear();
    this.timelines.clear();
    this.timelineEventCursors.clear();
    this.revokedRoomIds.clear();
  }

  /**
   * Purge every canonical copy of profile data for an account removed from the
   * server directory. Stable user IDs remain on historical facts, but no
   * renderable user object survives the removal operation.
   */
  private removeUser(userId: string): void {
    this.users.delete(userId);

    for (const [roomId, room] of this.rooms) {
      if (!room.memberUserIds.includes(userId)) continue;
      this.rooms.set(
        roomId,
        new RealtimeProjectionRoom({
          room: room.room,
          memberUserIds: room.memberUserIds.filter((candidate) => candidate !== userId),
          viewerNotificationCount: room.viewerNotificationCount
        })
      );
    }

    for (const [roomId, page] of this.timelines) {
      if (!page.includes?.users[userId]) continue;
      const next = page.clone();
      if (next.includes) delete next.includes.users[userId];
      this.timelines.set(roomId, next);
    }

    if (this.notifications) {
      let changed = false;
      const next = this.notifications.clone();
      for (const notification of next.notifications) {
        if (notification.actor?.id !== userId) continue;
        notification.actor = undefined;
        changed = true;
      }
      if (changed) this.notifications = next;
    }

    let callsChanged = false;
    const calls = this.activeCalls.map((call) => {
      if (!call.participants.some((participant) => participant.user?.id === userId)) return call;
      callsChanged = true;
      const next = call.clone();
      next.participants = next.participants.filter(
        (participant) => participant.user?.id !== userId
      );
      return next;
    });
    if (callsChanged) this.activeCalls = calls;
  }

  private removeActiveCallRoom(roomId: string): void {
    if (!this.activeCalls.some((call) => call.room?.id === roomId)) return;
    this.activeCalls = this.activeCalls.filter((call) => call.room?.id !== roomId);
  }

  private upsertTimelineEvent(input: {
    roomId: string;
    event?: RoomTimelinePage['events'][number];
    includes?: RoomTimelineIncludes;
    eventCursor: string;
  }): void {
    if (!input.event) return;
    const current = this.timelines.get(input.roomId) ?? new RoomTimelinePage();
    const events = [...current.events];
    const index = events.findIndex((event) => event.id === input.event?.id);
    if (index === -1) events.push(input.event);
    else events[index] = input.event;
    const cursors = this.timelineEventCursors.get(input.roomId) ?? new SvelteMap<string, string>();
    this.timelineEventCursors.set(input.roomId, cursors);
    if (input.eventCursor) cursors.set(input.event.id, input.eventCursor);
    events.sort(
      (left, right) =>
        (left.createdAt?.toDate().getTime() ?? 0) - (right.createdAt?.toDate().getTime() ?? 0)
    );
    const desiredEvents = events.slice(-50);
    const desiredStartCursor = cursors.get(desiredEvents[0]?.id ?? '');
    // A compacted prefix supplies cursors only for its boundary rows. Keep at
    // most one extra prefix window until live row cursors can advance the
    // retained start boundary without a separate bootstrap read.
    const didTrim = events.length > 50 && Boolean(desiredStartCursor);
    const retainedEvents = didTrim ? desiredEvents : events;
    if (didTrim) {
      const retainedIds = new SvelteSet(retainedEvents.map((event) => event.id));
      for (const eventId of cursors.keys()) if (!retainedIds.has(eventId)) cursors.delete(eventId);
    }
    const users = {
      ...(current.includes?.users ?? {}),
      ...(input.includes?.users ?? {})
    };
    this.timelines.set(
      input.roomId,
      new RoomTimelinePage({
        events: retainedEvents,
        startCursor: didTrim ? desiredStartCursor : current.startCursor,
        endCursor: cursors.get(retainedEvents.at(-1)?.id ?? '') ?? current.endCursor,
        hasOlder: current.hasOlder || didTrim,
        hasNewer: current.hasNewer,
        includes: new RoomTimelineIncludes({ users })
      })
    );
  }

  private removeTimelineEvent(roomId: string, eventId: string): void {
    const current = this.timelines.get(roomId);
    if (!current || !current.events.some((event) => event.id === eventId)) return;
    this.timelineEventCursors.get(roomId)?.delete(eventId);
    this.timelines.set(
      roomId,
      new RoomTimelinePage({
        events: current.events.filter((event) => event.id !== eventId),
        startCursor: current.startCursor,
        endCursor: current.endCursor,
        hasOlder: current.hasOlder,
        hasNewer: current.hasNewer,
        includes: current.includes
      })
    );
  }

  /** Retain newest-activity-first DM ordering across later room-state replacements. */
  private bumpRoom(roomId: string): void {
    const room = this.rooms.get(roomId);
    if (!room || this.rooms.keys().next().value === roomId) return;
    const remaining = [...this.rooms.entries()].filter(([id]) => id !== roomId);
    this.rooms.clear();
    this.rooms.set(roomId, room);
    for (const [id, entry] of remaining) this.rooms.set(id, entry);
  }

  private seedTimelineEventCursors(
    roomId: string,
    page: RoomTimelinePage,
    eventCursors: Record<string, string>
  ): void {
    const cursors = new SvelteMap<string, string>(Object.entries(eventCursors));
    const first = page.events[0];
    const last = page.events.at(-1);
    if (first && page.startCursor) cursors.set(first.id, page.startCursor);
    if (last && page.endCursor) cursors.set(last.id, page.endCursor);
    this.timelineEventCursors.set(roomId, cursors);
  }
}
