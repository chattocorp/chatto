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

describe('ServerProjectionStore', () => {
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
});
