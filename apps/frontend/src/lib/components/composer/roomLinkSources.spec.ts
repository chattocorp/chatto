import { afterEach, describe, expect, it, vi } from 'vitest';
import { createClient } from '@chatto/client';
import { RoomWithViewerState } from '@chatto/api-types/api/v1/room_directory_pb';
import { RoomKind } from '@chatto/client/api/roomDirectory';
import { knownServers, serverRegistry } from '$lib/client';
import { roomLinkSources } from './roomLinkSources';

/** Exercise catalogue reads against real client room filtering and privacy resets. */
describe('roomLinkSources', () => {
  afterEach(() => vi.restoreAllMocks());

  it('merges known servers and uses retained stores without creating missing stores', () => {
    const client = createClient();
    try {
      client.registry.addServer({
        id: 'alpha',
        url: 'https://alpha.test',
        name: 'Alpha',
        iconUrl: '/assets/alpha.png',
        addedAt: 1
      });
      client.registry.addServer({
        id: 'beta',
        url: 'https://beta.test',
        name: 'Beta',
        iconUrl: null,
        addedAt: 1
      });
      const store = client.registry.getStore('alpha');
      store.realtimeSync.markCaughtUp(undefined);
      store.projection.rooms.set(
        'active',
        new RoomWithViewerState({
          room: {
            id: 'active',
            name: 'General',
            kind: RoomKind.CHANNEL
          }
        })
      );
      store.projection.rooms.set(
        'archived',
        new RoomWithViewerState({
          room: {
            id: 'archived',
            name: 'Old',
            kind: RoomKind.CHANNEL,
            archived: true
          }
        })
      );
      vi.spyOn(serverRegistry, 'servers', 'get').mockImplementation(() => client.registry.servers);
      vi.spyOn(serverRegistry, 'tryGetStore').mockImplementation((id) =>
        id === 'alpha' ? store : undefined
      );
      const createStore = vi.spyOn(serverRegistry, 'getStore');
      vi.spyOn(knownServers, 'servers', 'get').mockReturnValue([
        {
          origin: 'https://alpha.test',
          profile: {
            name: 'Old name',
            version: '0.5.0',
            description: null,
            iconUrl: null,
            bannerUrl: null
          }
        },
        {
          origin: 'https://discovered.test',
          profile: {
            name: 'Discovered',
            version: '0.5.0',
            description: null,
            iconUrl: '/assets/discovered.png',
            bannerUrl: null
          }
        }
      ]);
      const sources = roomLinkSources('alpha');
      expect(sources).toHaveLength(3);
      expect(sources.find((source) => source.name === 'Alpha')).toMatchObject({
        isCurrent: true,
        logoUrl: '/assets/alpha.png'
      });
      expect(sources.find((source) => source.name === 'Beta')?.isCurrent).toBe(false);
      expect(sources.find((source) => source.name === 'Discovered')?.logoUrl).toBe(
        '/assets/discovered.png'
      );
      expect(
        sources.find((source) => source.name === 'Alpha')?.rooms.map((room) => room.id)
      ).toEqual(['active']);
      expect(sources.find((source) => source.name === 'Beta')?.rooms).toEqual([]);
      expect(sources.find((source) => source.name === 'Discovered')?.rooms).toEqual([]);
      store.projection.rooms.delete('active');
      expect(roomLinkSources('alpha').find((source) => source.name === 'Alpha')?.rooms).toEqual([]);
      store.projection.rooms.set(
        'restored',
        new RoomWithViewerState({
          room: { id: 'restored', name: 'Restored', kind: RoomKind.CHANNEL }
        })
      );
      expect(
        roomLinkSources('alpha').find((source) => source.name === 'Alpha')?.rooms
      ).toHaveLength(1);
      store.projection.reset();
      store.realtimeSync.reset();
      expect(roomLinkSources('alpha').find((source) => source.name === 'Alpha')?.rooms).toEqual([]);
      expect(createStore).not.toHaveBeenCalled();
    } finally {
      client.close();
    }
  });
});
