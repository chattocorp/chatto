import { describe, expect, it } from 'vitest';
import { RealtimeProjectionUpdate } from '$lib/eventBus.svelte';
import { ServerSnapshotChunk } from '@chatto/api-types/api/v1/server_snapshot_pb';
import { ListRoomsResponse, RoomWithViewerState } from '@chatto/api-types/api/v1/room_directory_pb';
import { Room } from '@chatto/api-types/api/v1/rooms_pb';
import { Event } from '@chatto/api-types/core/evt/v1/event_pb';
import { MessagePostedEvent } from '@chatto/api-types/core/evt/v1/message_events_pb';
import { ServerProjectionStore } from './projection.svelte';

describe('ServerProjectionStore', () => {
  it('applies canonical room response snapshots as complete replacements', () => {
    const store = new ServerProjectionStore();
    store.rooms.set('removed', new RoomWithViewerState({ room: new Room({ id: 'removed' }) }));

    store.apply(
      new RealtimeProjectionUpdate({
        snapshot: new ServerSnapshotChunk({
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
        event: new Event({
          event: { case: 'messagePosted', value: new MessagePostedEvent({ roomId: 'dm' }) }
        })
      })
    );
    expect(store.rooms.get('dm')?.hasMessageHistory).toBe(true);
  });
});
