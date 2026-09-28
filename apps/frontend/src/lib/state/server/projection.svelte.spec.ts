import { describe, expect, it } from 'vitest';
import { RealtimeProjectionUpdate } from '$lib/eventBus.svelte';
import { RealtimeResourceUpdate } from '$lib/api-client/realtimeResources';
import { ListRoomsResponse, RoomWithViewerState } from '@chatto/api-types/api/v1/room_directory_pb';
import { Room } from '@chatto/api-types/api/v1/rooms_pb';
import { MessagePostedEvent } from '@chatto/api-types/realtime/v1/events_pb';
import { RealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
import { ServerProjectionStore } from './projection.svelte';
import { ListUsersResponse } from '@chatto/api-types/api/v1/user_service_pb';
import { DirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import { User } from '@chatto/api-types/api/v1/users_pb';
import { ActiveCall, CallParticipant } from '@chatto/api-types/api/v1/voice_calls_pb';
import { RoomSummary } from '@chatto/api-types/api/v1/rooms_pb';

describe('ServerProjectionStore', () => {
  it('keeps a deleted account in DM membership as a tombstoned participant', () => {
    const store = new ServerProjectionStore();
    store.users.set('gone', new DirectoryMember({ user: new User({ id: 'gone', login: 'gone' }) }));
    store.rooms.set('dm', new RoomWithViewerState({ memberUserIds: ['viewer', 'gone'] }));

    store.removeUser('gone');

    expect(store.rooms.get('dm')?.memberUserIds).toEqual(['viewer', 'gone']);
    expect(store.users.get('gone')).toBeUndefined();
    expect(store.users.isDeleted('gone')).toBe(true);
  });

  it('merges a partial snapshot user list into cached profiles', () => {
    const store = new ServerProjectionStore();
    store.users.set(
      'cached',
      new DirectoryMember({
        user: new User({ id: 'cached', displayName: 'Retained name' })
      })
    );
    store.apply(
      new RealtimeProjectionUpdate({
        resource: new RealtimeResourceUpdate({
          resource: {
            case: 'users',
            value: new ListUsersResponse({
              users: [
                new DirectoryMember({ user: new User({ id: 'viewer', displayName: 'Viewer' }) })
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
    store.rooms.set('removed', new RoomWithViewerState({ room: new Room({ id: 'removed' }) }));

    store.apply(
      new RealtimeProjectionUpdate({
        resource: new RealtimeResourceUpdate({
          resource: {
            case: 'rooms',
            value: new ListRoomsResponse({
              rooms: [
                new RoomWithViewerState({
                  room: new Room({ id: 'dm' }),
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
      new RoomWithViewerState({ room: new Room({ id: 'dm' }), hasMessageHistory: false })
    );
    store.apply(
      new RealtimeProjectionUpdate({
        event: new RealtimeEvent({
          event: { case: 'messagePosted', value: new MessagePostedEvent({ roomId: 'dm' }) }
        })
      })
    );
    expect(store.rooms.get('dm')?.hasMessageHistory).toBe(true);
  });

  describe('active calls', () => {
    const call = (roomId: string, userIds: string[]) =>
      new ActiveCall({
        room: new RoomSummary({ id: roomId }),
        callId: `call-${roomId}`,
        participants: userIds.map((id) => new CallParticipant({ user: new User({ id }) }))
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
