import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import { RoomTimelineIncludes, RoomTimelinePage } from '@chatto/api-types/api/v1/room_timeline_pb';
import type { DirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import { RoomWithViewerState, type RoomGroup } from '@chatto/api-types/api/v1/room_directory_pb';
import type { ServerPublicProfile } from '@chatto/api-types/api/v1/server_pb';
import type { GetViewerResponse } from '@chatto/api-types/api/v1/viewer_pb';
import type { ListNotificationsResponse } from '@chatto/api-types/api/v1/notifications_pb';
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
  timelines = new SvelteMap<string, RoomTimelinePage>();
  private timelineEventCursors = new SvelteMap<string, SvelteMap<string, string>>();

  apply(event: RealtimeProjectionEvent): void {
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
          this.users.delete(operation.operation.value.userId);
          break;
        case 'roomUpsert': {
          const room = operation.operation.value;
          const roomId = room.room?.room?.id;
          if (roomId) this.rooms.set(roomId, room);
          break;
        }
        case 'roomRemove':
          this.rooms.delete(operation.operation.value.roomId);
          this.timelines.delete(operation.operation.value.roomId);
          this.timelineEventCursors.delete(operation.operation.value.roomId);
          break;
        case 'roomGroupsReplace':
          this.roomGroups = [...operation.operation.value.groups];
          break;
        case 'roomTimelineReplace': {
          const replacement = operation.operation.value;
          if (replacement.page) {
            this.timelines.set(replacement.roomId, replacement.page);
            this.seedTimelineEventCursors(
              replacement.roomId,
              replacement.page,
              replacement.eventCursors
            );
          }
          break;
        }
        case 'roomTimelineEventUpsert':
          this.upsertTimelineEvent(operation.operation.value);
          break;
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
          break;
        }
        case undefined:
          break;
      }
    }
  }

  reset(): void {
    this.server = null;
    this.serverState = null;
    this.viewer = null;
    this.users.clear();
    this.rooms.clear();
    this.roomGroups = [];
    this.notifications = null;
    this.timelines.clear();
    this.timelineEventCursors.clear();
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
