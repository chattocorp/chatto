import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import { RoomTimelineIncludes, RoomTimelinePage } from '@chatto/api-types/api/v1/room_timeline_pb';
import { DirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import { ThreadViewerState } from '@chatto/api-types/api/v1/message_types_pb';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import {
  RoomViewerState,
  RoomWithViewerState,
  type RoomGroup
} from '@chatto/api-types/api/v1/room_directory_pb';
import type { ServerPublicProfile } from '@chatto/api-types/api/v1/server_pb';
import type { GetViewerResponse } from '@chatto/api-types/api/v1/viewer_pb';
import type { ActiveCall } from '@chatto/api-types/api/v1/voice_calls_pb';
import { RealtimeRoomState } from '@chatto/api-types/realtime/v1/realtime_pb';
import type { RealtimeServerState } from '@chatto/api-types/realtime/v1/realtime_pb';
import type { RealtimeProjectionUpdate } from '$lib/eventBus.svelte';

/** Canonical protobuf-native state for one connected Chatto server. */
export class ServerProjectionStore {
  server = $state.raw<ServerPublicProfile | null>(null);
  serverState = $state.raw<RealtimeServerState | null>(null);
  viewer = $state.raw<GetViewerResponse | null>(null);
  users = new SvelteMap<string, DirectoryMember>();
  rooms = new SvelteMap<string, RealtimeRoomState>();
  roomGroups = $state.raw<RoomGroup[]>([]);
  activeCalls = $state.raw<ActiveCall[]>([]);
  /** Complete current followed-thread viewer state, keyed by room and root ID. */
  threadViewerStates = new SvelteMap<string, ThreadViewerState>();
  timelines = new SvelteMap<string, RoomTimelinePage>();
  private timelineEventCursors = new SvelteMap<string, SvelteMap<string, string>>();
  private revokedRoomIds = new SvelteSet<string>();

  apply(update: RealtimeProjectionUpdate): void {
    if (update.reset) this.reset({ preserveViewer: true });
    const semantic = update.event?.event;

    // Unknown additive state variants decode without a known oneof case. The
    // bundled projection ignores them while still accepting the event cursor.
    for (const stateItem of update.state) {
      switch (stateItem.state.case) {
        case 'server':
        case 'serverState':
        case 'viewer':
        case 'user':
        case 'userRemoved':
        case 'room':
        case 'roomRemoved':
        case 'roomGroups':
        case 'roomTimeline':
        case 'roomTimelineEvent':
        case 'roomTimelineEventRemoved':
        case 'notifications':
        case 'roomViewer':
        case 'roomViewerActivity':
        case 'activeCalls':
        case 'presences':
        case 'threadViewerStates':
        case undefined:
          break;
      }
    }
    for (const stateItem of update.state) {
      switch (stateItem.state.case) {
        case 'server':
          this.server = stateItem.state.value;
          break;
        case 'serverState':
          this.serverState = stateItem.state.value;
          break;
        case 'viewer':
          this.viewer = stateItem.state.value;
          break;
        case 'user': {
          const member = stateItem.state.value;
          const userId = member.user?.id;
          if (userId) this.users.set(userId, member);
          break;
        }
        case 'userRemoved':
          this.removeUser(stateItem.state.value.userId);
          break;
        case 'room': {
          const room = stateItem.state.value;
          const roomId = room.room?.room?.id;
          if (roomId) {
            this.rooms.set(roomId, room);
            if (room.room?.viewerState?.isMember === false) {
              this.revokedRoomIds.add(roomId);
              this.timelines.delete(roomId);
              this.timelineEventCursors.delete(roomId);
              this.removeActiveCallRoom(roomId);
            } else if (room.room?.viewerState?.isMember === true)
              this.revokedRoomIds.delete(roomId);
          }
          break;
        }
        case 'roomRemoved':
          this.revokedRoomIds.add(stateItem.state.value.roomId);
          this.rooms.delete(stateItem.state.value.roomId);
          this.timelines.delete(stateItem.state.value.roomId);
          this.timelineEventCursors.delete(stateItem.state.value.roomId);
          this.removeActiveCallRoom(stateItem.state.value.roomId);
          break;
        case 'roomGroups':
          this.roomGroups = [...stateItem.state.value.groups];
          break;
        case 'roomTimeline': {
          const replacement = stateItem.state.value;
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
        case 'roomTimelineEvent': {
          const update = stateItem.state.value;
          if (!this.revokedRoomIds.has(update.roomId)) this.upsertTimelineEvent(update);
          break;
        }
        case 'roomTimelineEventRemoved':
          this.removeTimelineEvent(stateItem.state.value.roomId, stateItem.state.value.eventId);
          break;
        case 'notifications': {
          // Notification state is owned by NotificationStore. Keeping another
          // hydrated payload mirror here would make authorization scrubbing
          // and optimistic count updates race across two owners.
          break;
        }
        case 'roomViewer': {
          const replacement = stateItem.state.value;
          const current = this.rooms.get(replacement.roomId);
          if (current) {
            this.rooms.set(
              replacement.roomId,
              new RealtimeRoomState({
                room: new RoomWithViewerState({
                  room: current.room?.room,
                  viewerState: replacement.viewerState
                }),
                memberUserIds: [...current.memberUserIds],
                hasMessageHistory: current.hasMessageHistory
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
        case 'roomViewerActivity': {
          const replacement = stateItem.state.value;
          const current = this.rooms.get(replacement.roomId);
          if (current) {
            const viewerState = current.room?.viewerState?.clone() ?? new RoomViewerState();
            viewerState.hasUnread = replacement.hasUnread;
            viewerState.slowModeNextPostAt = replacement.slowModeNextPostAt;
            this.rooms.set(
              replacement.roomId,
              new RealtimeRoomState({
                room: new RoomWithViewerState({
                  room: current.room?.room,
                  viewerState
                }),
                memberUserIds: [...current.memberUserIds],
                hasMessageHistory: current.hasMessageHistory
              })
            );
          }
          break;
        }
        case 'activeCalls':
          this.activeCalls = [...stateItem.state.value.calls];
          break;
        case 'presences':
          for (const [userId, member] of this.users) {
            if (!member.user) continue;
            const user = member.user.clone();
            user.presenceStatus = stateItem.state.value.statuses[userId] ?? PresenceStatus.OFFLINE;
            this.users.set(
              userId,
              new DirectoryMember({ user, roles: [...member.roles], createdAt: member.createdAt })
            );
          }
          break;
        case 'threadViewerStates': {
          this.threadViewerStates.clear();
          for (const state of stateItem.state.value.states) {
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
                  ?.clone() ??
                new ThreadViewerState({ isFollowing: false, hasUnreadReplies: false });
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
        case undefined:
          break;
      }
    }
    if (
      semantic?.case === 'message' &&
      semantic.value.action === 1 &&
      !semantic.value.threadRootEventId
    ) {
      this.activateRoom(semantic.value.roomId);
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
      new RealtimeRoomState({
        room: room.room,
        memberUserIds: [],
        hasMessageHistory: room.hasMessageHistory
      })
    );
  }

  /** Clear projected state, optionally retaining the last confirmed viewer during catch-up. */
  reset({ preserveViewer = false }: { preserveViewer?: boolean } = {}): void {
    const viewer = preserveViewer ? this.viewer : null;
    this.server = null;
    this.serverState = null;
    this.viewer = viewer;
    this.users.clear();
    this.rooms.clear();
    this.roomGroups = [];
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
        new RealtimeRoomState({
          room: room.room,
          memberUserIds: room.memberUserIds.filter((candidate) => candidate !== userId),
          hasMessageHistory: room.hasMessageHistory
        })
      );
    }

    for (const [roomId, page] of this.timelines) {
      if (!page.includes?.users[userId]) continue;
      const next = page.clone();
      if (next.includes) delete next.includes.users[userId];
      this.timelines.set(roomId, next);
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
    // A snapshot prefix supplies cursors only for its boundary rows. Keep at
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

  /** Activate first-message visibility and retain newest-activity-first ordering. */
  private activateRoom(roomId: string): void {
    const current = this.rooms.get(roomId);
    if (!current) return;
    const room = new RealtimeRoomState({
      room: current.room,
      memberUserIds: [...current.memberUserIds],
      hasMessageHistory: true
    });
    if (this.rooms.keys().next().value === roomId) {
      this.rooms.set(roomId, room);
      return;
    }
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
