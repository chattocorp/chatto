import { Timestamp } from '@bufbuild/protobuf';
import { describe, expect, it } from 'vitest';
import { DirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import {
  RoomGroup,
  RoomViewerState,
  RoomWithViewerState
} from '@chatto/api-types/api/v1/room_directory_pb';
import { ServerPublicProfile } from '@chatto/api-types/api/v1/server_pb';
import { GetViewerResponse } from '@chatto/api-types/api/v1/viewer_pb';
import {
  RoomMessagePosted,
  RoomTimelineEvent,
  RoomTimelinePage
} from '@chatto/api-types/api/v1/room_timeline_pb';
import { Room } from '@chatto/api-types/api/v1/rooms_pb';
import { User } from '@chatto/api-types/api/v1/users_pb';
import { ActiveCall } from '@chatto/api-types/api/v1/voice_calls_pb';
import {
  ListNotificationsResponse,
  RoomNotificationCount
} from '@chatto/api-types/api/v1/notifications_pb';
import {
  RealtimeProjectionEvent,
  RealtimeProjectionActiveCallsReplace,
  RealtimeProjectionOperation,
  RealtimeProjectionReset,
  RealtimeProjectionRoom,
  RealtimeProjectionRoomGroupsReplace,
  RealtimeProjectionRoomViewerStateReplace,
  RealtimeProjectionRoomRemove,
  RealtimeProjectionRoomTimelineEventRemove,
  RealtimeProjectionRoomTimelineEventUpsert,
  RealtimeProjectionRoomTimelineReplace,
  RealtimeProjectionNotificationsReplace,
  RealtimeProjectionServerState,
  RealtimeProjectionUserRemove
} from '@chatto/api-types/realtime/v1/realtime_pb';
import { ServerProjectionStore } from './projection.svelte';

function event(...operations: RealtimeProjectionOperation[]): RealtimeProjectionEvent {
  return new RealtimeProjectionEvent({ operations });
}

function operation(value: RealtimeProjectionOperation['operation']): RealtimeProjectionOperation {
  return new RealtimeProjectionOperation({ operation: value });
}

function timelineEvent(id: string, at: string): RoomTimelineEvent {
  return new RoomTimelineEvent({
    id,
    createdAt: Timestamp.fromDate(new Date(at)),
    event: { case: 'messagePosted', value: new RoomMessagePosted() }
  });
}

describe('ServerProjectionStore', () => {
  it('applies canonical resources, replacements, and removals', () => {
    const store = new ServerProjectionStore();
    const server = new ServerPublicProfile({ name: 'Projection Server' });
    const viewer = new GetViewerResponse();
    const user = new DirectoryMember({ user: new User({ id: 'U1', displayName: 'Ada' }) });
    const room = new RealtimeProjectionRoom({
      room: new RoomWithViewerState({
        room: new Room({ id: 'R1' }),
        viewerState: new RoomViewerState({ isMember: true })
      }),
      memberUserIds: ['U1'],
      viewerNotificationCount: 3
    });
    const group = new RoomGroup({ id: 'G1', name: 'General' });

    store.apply(
      event(
        operation({ case: 'serverUpsert', value: server }),
        operation({ case: 'viewerUpsert', value: viewer }),
        operation({ case: 'userUpsert', value: user }),
        operation({ case: 'roomUpsert', value: room }),
        operation({
          case: 'activeCallsReplace',
          value: new RealtimeProjectionActiveCallsReplace({
            calls: [new ActiveCall({ callId: 'call-1' })]
          })
        }),
        operation({
          case: 'roomGroupsReplace',
          value: new RealtimeProjectionRoomGroupsReplace({ groups: [group] })
        }),
        operation({
          case: 'roomViewerStateReplace',
          value: new RealtimeProjectionRoomViewerStateReplace({
            roomId: 'R1',
            viewerState: new RoomViewerState({ isMember: false })
          })
        })
      )
    );

    expect(store.activeCalls.map((call) => call.callId)).toEqual(['call-1']);

    expect(store.server).toBe(server);
    expect(store.viewer).toBe(viewer);
    expect(store.users.get('U1')).toBe(user);
    expect(store.roomGroups).toEqual([group]);
    expect(store.rooms.get('R1')?.room?.viewerState?.isMember).toBe(false);
    expect(store.rooms.get('R1')?.memberUserIds).toEqual(['U1']);
    expect(store.rooms.get('R1')?.viewerNotificationCount).toBe(3);

    store.apply(
      event(
        operation({
          case: 'userRemove',
          value: new RealtimeProjectionUserRemove({ userId: 'U1' })
        }),
        operation({
          case: 'roomGroupsReplace',
          value: new RealtimeProjectionRoomGroupsReplace()
        })
      )
    );
    expect(store.users.has('U1')).toBe(false);
    expect(store.roomGroups).toEqual([]);
  });

  it('applies idempotent resource and timeline mutations across every room', () => {
    const store = new ServerProjectionStore();
    const user = new DirectoryMember({ user: new User({ id: 'U1', displayName: 'Ada' }) });
    const room = new RealtimeProjectionRoom({
      room: new RoomWithViewerState({ room: new Room({ id: 'R1' }) }),
      memberUserIds: ['U1']
    });

    store.apply(
      event(
        operation({
          case: 'serverStateUpsert',
          value: new RealtimeProjectionServerState({ motd: 'Hello' })
        }),
        operation({ case: 'userUpsert', value: user }),
        operation({ case: 'roomUpsert', value: room }),
        operation({
          case: 'roomTimelineReplace',
          value: new RealtimeProjectionRoomTimelineReplace({
            roomId: 'R1',
            page: new RoomTimelinePage({ events: [timelineEvent('M2', '2026-01-02')] })
          })
        }),
        operation({
          case: 'roomTimelineEventUpsert',
          value: new RealtimeProjectionRoomTimelineEventUpsert({
            roomId: 'R1',
            event: timelineEvent('M1', '2026-01-01')
          })
        })
      )
    );

    expect(store.users.get('U1')).toBe(user);
    expect(store.serverState?.motd).toBe('Hello');
    expect(store.rooms.get('R1')).toBe(room);
    expect(store.timelines.get('R1')?.events.map(({ id }) => id)).toEqual(['M1', 'M2']);

    store.apply(
      event(
        operation({
          case: 'roomTimelineEventUpsert',
          value: new RealtimeProjectionRoomTimelineEventUpsert({
            roomId: 'R1',
            event: timelineEvent('M1', '2026-01-01')
          })
        })
      )
    );
    expect(store.timelines.get('R1')?.events.map(({ id }) => id)).toEqual(['M1', 'M2']);

    store.apply(
      event(
        operation({
          case: 'roomTimelineEventRemove',
          value: new RealtimeProjectionRoomTimelineEventRemove({ roomId: 'R1', eventId: 'M1' })
        })
      )
    );
    expect(store.timelines.get('R1')?.events.map(({ id }) => id)).toEqual(['M2']);
  });

  it('purges room state on authorization loss and clears all state on reset', () => {
    const store = new ServerProjectionStore();
    store.apply(
      event(
        operation({
          case: 'roomUpsert',
          value: new RealtimeProjectionRoom({
            room: new RoomWithViewerState({ room: new Room({ id: 'R1' }) })
          })
        }),
        operation({
          case: 'roomTimelineReplace',
          value: new RealtimeProjectionRoomTimelineReplace({
            roomId: 'R1',
            page: new RoomTimelinePage({ events: [timelineEvent('M1', '2026-01-01')] })
          })
        })
      )
    );

    store.apply(
      event(
        operation({
          case: 'roomRemove',
          value: new RealtimeProjectionRoomRemove({ roomId: 'R1' })
        })
      )
    );
    expect(store.rooms.has('R1')).toBe(false);
    expect(store.timelines.has('R1')).toBe(false);

    store.apply(
      event(
        operation({
          case: 'userUpsert',
          value: new DirectoryMember({ user: new User({ id: 'U1' }) })
        }),
        operation({ case: 'reset', value: new RealtimeProjectionReset() })
      )
    );
    expect(store.users.size).toBe(0);
    expect(store.serverState).toBeNull();
    expect(store.rooms.size).toBe(0);
    expect(store.timelines.size).toBe(0);
  });

  it('bounds retained room timelines and replaces current notification counts', () => {
    const store = new ServerProjectionStore();
    store.apply(
      event(
        operation({
          case: 'roomUpsert',
          value: new RealtimeProjectionRoom({
            room: new RoomWithViewerState({ room: new Room({ id: 'R1' }) }),
            viewerNotificationCount: 9
          })
        }),
        ...Array.from({ length: 55 }, (_, index) =>
          operation({
            case: 'roomTimelineEventUpsert',
            value: new RealtimeProjectionRoomTimelineEventUpsert({
              roomId: 'R1',
              event: timelineEvent(
                `M${index}`,
                `2026-01-01T00:00:${String(index).padStart(2, '0')}Z`
              ),
              eventCursor: `cursor-${index}`
            })
          })
        ),
        operation({
          case: 'notificationsReplace',
          value: new RealtimeProjectionNotificationsReplace({
            page: new ListNotificationsResponse(),
            roomCounts: [new RoomNotificationCount({ roomId: 'R1', totalCount: 2 })]
          })
        })
      )
    );

    expect(store.timelines.get('R1')?.events).toHaveLength(50);
    expect(store.timelines.get('R1')?.events[0]?.id).toBe('M5');
    expect(store.timelines.get('R1')?.startCursor).toBe('cursor-5');
    expect(store.timelines.get('R1')?.endCursor).toBe('cursor-54');
    expect(store.rooms.get('R1')?.viewerNotificationCount).toBe(2);
    expect(store.notifications).not.toBeNull();

    store.apply(
      event(
        operation({
          case: 'roomViewerStateReplace',
          value: new RealtimeProjectionRoomViewerStateReplace({
            roomId: 'R1'
          })
        })
      )
    );
    expect(store.rooms.get('R1')?.viewerNotificationCount).toBe(2);
  });

  it('retains root-message room activity order across viewer-state replacements', () => {
    const store = new ServerProjectionStore();
    store.apply(
      event(
        operation({
          case: 'roomUpsert',
          value: new RealtimeProjectionRoom({
            room: new RoomWithViewerState({ room: new Room({ id: 'R1' }) })
          })
        }),
        operation({
          case: 'roomUpsert',
          value: new RealtimeProjectionRoom({
            room: new RoomWithViewerState({ room: new Room({ id: 'R2' }) })
          })
        }),
        operation({
          case: 'roomTimelineEventUpsert',
          value: new RealtimeProjectionRoomTimelineEventUpsert({
            roomId: 'R2',
            event: timelineEvent('M1', '2026-01-01T00:00:00Z')
          })
        }),
        operation({
          case: 'roomViewerStateReplace',
          value: new RealtimeProjectionRoomViewerStateReplace({
            roomId: 'R2',
            viewerState: new RoomViewerState({ hasUnread: false })
          })
        })
      )
    );

    expect([...store.rooms.keys()]).toEqual(['R2', 'R1']);
  });

  it('advances a compacted timeline cursor using only streamed row cursors', () => {
    const store = new ServerProjectionStore();
    const prefix = Array.from({ length: 50 }, (_, index) =>
      timelineEvent(`P${index}`, `2026-01-01T00:00:${String(index).padStart(2, '0')}Z`)
    );
    store.apply(
      event(
        operation({
          case: 'roomTimelineReplace',
          value: new RealtimeProjectionRoomTimelineReplace({
            roomId: 'R1',
            page: new RoomTimelinePage({
              events: prefix,
              startCursor: 'prefix-start',
              endCursor: 'prefix-end'
            }),
            eventCursors: Object.fromEntries(
              prefix.map((timelineEvent, index) => [timelineEvent.id, `prefix-${index}`])
            )
          })
        })
      )
    );

    store.apply(
      event(
        ...Array.from({ length: 1 }, (_, index) =>
          operation({
            case: 'roomTimelineEventUpsert',
            value: new RealtimeProjectionRoomTimelineEventUpsert({
              roomId: 'R1',
              event: timelineEvent(
                `L${index}`,
                `2026-01-02T00:00:${String(index).padStart(2, '0')}Z`
              ),
              eventCursor: `live-${index}`
            })
          })
        )
      )
    );

    expect(store.timelines.get('R1')?.events).toHaveLength(50);
    expect(store.timelines.get('R1')?.events[0]?.id).toBe('P1');
    expect(store.timelines.get('R1')?.startCursor).toBe('prefix-1');
    expect(store.timelines.get('R1')?.endCursor).toBe('live-0');
    expect(store.timelines.get('R1')?.hasOlder).toBe(true);
  });
});
