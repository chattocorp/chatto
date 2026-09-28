import { describe, expect, it } from 'vitest';
import { RealtimeProjectionUpdate } from '$lib/eventBus.svelte';
import { RealtimeResourceUpdate } from '$lib/api-client/realtimeResources';
import {
  ListRoomsResponseSchema,
  RoomWithViewerStateSchema
} from '@chatto/api-types/api/v1/room_directory_pb';
import { RoomSchema, RoomSummarySchema } from '@chatto/api-types/api/v1/rooms_pb';
import { MessagePostedEventSchema } from '@chatto/api-types/realtime/v1/events_pb';
import { RealtimeEventSchema } from '@chatto/api-types/realtime/v1/realtime_pb';
import { ServerProjectionStore } from './projection.svelte';
import { ListUsersResponseSchema } from '@chatto/api-types/api/v1/user_service_pb';
import { DirectoryMemberSchema } from '@chatto/api-types/api/v1/member_directory_pb';
import { UserSchema } from '@chatto/api-types/api/v1/users_pb';
import { ActiveCallSchema, CallParticipantSchema } from '@chatto/api-types/api/v1/voice_calls_pb';
import { create } from '@bufbuild/protobuf';

describe('ServerProjectionStore', () => {
  it('keeps a deleted account in DM membership as a tombstoned participant', () => {
    const store = new ServerProjectionStore();
    store.users.set(
      'gone',
      create(DirectoryMemberSchema, { user: create(UserSchema, { id: 'gone', login: 'gone' }) })
    );
    store.rooms.set('dm', create(RoomWithViewerStateSchema, { memberUserIds: ['viewer', 'gone'] }));

    store.removeUser('gone');

    expect(store.rooms.get('dm')?.memberUserIds).toEqual(['viewer', 'gone']);
    expect(store.users.get('gone')).toBeUndefined();
    expect(store.users.isDeleted('gone')).toBe(true);
  });

  it('merges a partial snapshot user list into cached profiles', () => {
    const store = new ServerProjectionStore();
    store.users.set(
      'cached',
      create(DirectoryMemberSchema, {
        user: create(UserSchema, { id: 'cached', displayName: 'Retained name' })
      })
    );
    store.apply(
      new RealtimeProjectionUpdate({
        resource: new RealtimeResourceUpdate({
          resource: {
            case: 'users',
            value: create(ListUsersResponseSchema, {
              users: [
                create(DirectoryMemberSchema, {
                  user: create(UserSchema, { id: 'viewer', displayName: 'Viewer' })
                })
              ]
            })
          }
        })
      })
    );

    expect(store.users.get('cached')?.user?.displayName).toBe('Retained name');
    expect(store.users.get('viewer')?.user?.displayName).toBe('Viewer');
    expect(store.users.isDeleted('cached')).toBe(false);
  });
  it('applies canonical room responses as complete replacements', () => {
    const store = new ServerProjectionStore();
    store.rooms.set(
      'removed',
      create(RoomWithViewerStateSchema, { room: create(RoomSchema, { id: 'removed' }) })
    );

    store.apply(
      new RealtimeProjectionUpdate({
        resource: new RealtimeResourceUpdate({
          resource: {
            case: 'rooms',
            value: create(ListRoomsResponseSchema, {
              rooms: [
                create(RoomWithViewerStateSchema, {
                  room: create(RoomSchema, { id: 'dm' }),
                  memberUserIds: ['viewer', 'peer'],
                  hasMessageHistory: false
                })
              ]
            })
          }
        })
      })
    );

    expect([...store.rooms.keys()]).toEqual(['dm']);
    expect(store.rooms.get('dm')?.memberUserIds).toEqual(['viewer', 'peer']);
    expect(store.rooms.get('dm')?.hasMessageHistory).toBe(false);
  });

  it('uses a canonical message fact to activate an empty DM', () => {
    const store = new ServerProjectionStore();
    store.rooms.set(
      'dm',
      create(RoomWithViewerStateSchema, {
        room: create(RoomSchema, { id: 'dm' }),
        hasMessageHistory: false
      })
    );
    store.apply(
      new RealtimeProjectionUpdate({
        event: create(RealtimeEventSchema, {
          event: {
            case: 'messagePosted',
            value: create(MessagePostedEventSchema, { roomId: 'dm' })
          }
        })
      })
    );
    expect(store.rooms.get('dm')?.hasMessageHistory).toBe(true);
  });

  describe('active calls', () => {
    const call = (roomId: string, userIds: string[]) =>
      create(ActiveCallSchema, {
        room: create(RoomSummarySchema, { id: roomId }),
        callId: `call-${roomId}`,
        participants: userIds.map((id) =>
          create(CallParticipantSchema, { user: create(UserSchema, { id }) })
        )
      });
    const participants = (projection: ServerProjectionStore, roomId: string) =>
      projection.activeCalls
        .find((candidate) => candidate.room?.id === roomId)
        ?.participants.map((participant) => participant.user?.id);

    it('removes only the given participant from a room call', () => {
      const projection = new ServerProjectionStore();
      projection.activeCalls = [call('R1', ['U1', 'U2']), call('R2', ['U1'])];

      projection.removeCallParticipant('R1', 'U1');

      expect(participants(projection, 'R1')).toEqual(['U2']);
      expect(participants(projection, 'R2')).toEqual(['U1']);
    });

    it('drops a room call when its last participant is removed optimistically', () => {
      const projection = new ServerProjectionStore();
      projection.activeCalls = [call('R1', ['U1'])];

      projection.removeCallParticipant('R1', 'U1');

      expect(projection.activeCalls).toEqual([]);
    });

    it('keeps a call whose last deleted participant is scrubbed', () => {
      const projection = new ServerProjectionStore();
      projection.activeCalls = [call('R1', ['U1'])];

      projection.removeUser('U1');

      expect(participants(projection, 'R1')).toEqual([]);
    });

    it('removes only the calls of the given room', () => {
      const projection = new ServerProjectionStore();
      projection.activeCalls = [call('R1', ['U1']), call('R2', ['U2'])];

      projection.removeRoomCalls('R1');

      expect(projection.activeCalls.map((candidate) => candidate.room?.id)).toEqual(['R2']);
    });
  });
});
