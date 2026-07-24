import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { describe, expect, it } from 'vitest';

import {
  authenticatedCurrentUserPresenceEntries,
  PresenceCache,
  updateAuthenticatedCurrentUserPresenceEntries
} from './presenceCache.svelte';

describe('PresenceCache', () => {
  it('replaces one server snapshot without disturbing another server', () => {
    const cache = new PresenceCache();
    cache.update({ serverId: 'origin', userId: 'old' }, PresenceStatus.ONLINE);
    cache.update({ serverId: 'remote', userId: 'same' }, PresenceStatus.AWAY);

    cache.replaceServer(
      'origin',
      new Map([
        ['same', PresenceStatus.DO_NOT_DISTURB],
        ['offline', PresenceStatus.OFFLINE]
      ])
    );

    expect(cache.get({ serverId: 'origin', userId: 'old' }, PresenceStatus.OFFLINE)).toBe(
      PresenceStatus.OFFLINE
    );
    expect(cache.get({ serverId: 'origin', userId: 'same' }, PresenceStatus.OFFLINE)).toBe(
      PresenceStatus.DO_NOT_DISTURB
    );
    expect(cache.get({ serverId: 'remote', userId: 'same' }, PresenceStatus.OFFLINE)).toBe(
      PresenceStatus.AWAY
    );
  });

  it('isolates entries by server id and user id', () => {
    const cache = new PresenceCache();

    cache.update({ serverId: 'origin', userId: 'same-user-id' }, PresenceStatus.AWAY);
    cache.update({ serverId: 'remote', userId: 'same-user-id' }, PresenceStatus.DO_NOT_DISTURB);

    expect(cache.get({ serverId: 'origin', userId: 'same-user-id' }, PresenceStatus.ONLINE)).toBe(
      PresenceStatus.AWAY
    );
    expect(cache.get({ serverId: 'remote', userId: 'same-user-id' }, PresenceStatus.ONLINE)).toBe(
      PresenceStatus.DO_NOT_DISTURB
    );
  });

  it('clears stale entries while retaining provided current-user presence', () => {
    const cache = new PresenceCache();
    cache.update({ serverId: 'origin', userId: 'current-user' }, PresenceStatus.ONLINE);
    cache.update({ serverId: 'origin', userId: 'other-user' }, PresenceStatus.AWAY);

    cache.clear([[{ serverId: 'origin', userId: 'current-user' }, PresenceStatus.DO_NOT_DISTURB]]);

    expect(cache.get({ serverId: 'origin', userId: 'current-user' }, PresenceStatus.ONLINE)).toBe(
      PresenceStatus.DO_NOT_DISTURB
    );
    expect(cache.get({ serverId: 'origin', userId: 'other-user' }, PresenceStatus.ONLINE)).toBe(
      PresenceStatus.ONLINE
    );
  });

  it('updates current-user presence entries across authenticated servers', () => {
    const cache = new PresenceCache();
    const lateStore = {
      serverId: 'late-remote',
      isAuthenticated: true,
      currentUser: { user: null as { id: string } | null }
    };
    cache.update({ serverId: 'origin', userId: 'origin-user' }, PresenceStatus.ONLINE);
    cache.update({ serverId: 'remote', userId: 'remote-user' }, PresenceStatus.ONLINE);
    cache.update({ serverId: 'late-remote', userId: 'late-remote-user' }, PresenceStatus.ONLINE);
    cache.update({ serverId: 'signed-out', userId: 'signed-out-user' }, PresenceStatus.ONLINE);

    updateAuthenticatedCurrentUserPresenceEntries(
      cache,
      [
        {
          serverId: 'origin',
          isAuthenticated: true,
          currentUser: { user: { id: 'origin-user' } }
        },
        {
          serverId: 'remote',
          isAuthenticated: true,
          currentUser: { user: { id: 'remote-user' } }
        },
        {
          serverId: 'signed-out',
          isAuthenticated: false,
          currentUser: { user: { id: 'signed-out-user' } }
        },
        lateStore
      ],
      PresenceStatus.OFFLINE
    );

    expect(cache.get({ serverId: 'origin', userId: 'origin-user' }, PresenceStatus.ONLINE)).toBe(
      PresenceStatus.OFFLINE
    );
    expect(cache.get({ serverId: 'remote', userId: 'remote-user' }, PresenceStatus.ONLINE)).toBe(
      PresenceStatus.OFFLINE
    );
    expect(
      cache.get({ serverId: 'late-remote', userId: 'late-remote-user' }, PresenceStatus.AWAY)
    ).toBe(PresenceStatus.ONLINE);
    expect(
      cache.get({ serverId: 'signed-out', userId: 'signed-out-user' }, PresenceStatus.AWAY)
    ).toBe(PresenceStatus.ONLINE);

    lateStore.currentUser.user = { id: 'late-remote-user' };
    updateAuthenticatedCurrentUserPresenceEntries(cache, [lateStore], PresenceStatus.OFFLINE);

    expect(
      cache.get({ serverId: 'late-remote', userId: 'late-remote-user' }, PresenceStatus.ONLINE)
    ).toBe(PresenceStatus.OFFLINE);
  });

  it('retains all authenticated current-user entries when clearing stale presence', () => {
    const cache = new PresenceCache();
    const stores = [
      {
        serverId: 'origin',
        isAuthenticated: true,
        currentUser: { user: { id: 'origin-user' } }
      },
      {
        serverId: 'remote',
        isAuthenticated: true,
        currentUser: { user: { id: 'remote-user' } }
      },
      {
        serverId: 'signed-out',
        isAuthenticated: false,
        currentUser: { user: { id: 'signed-out-user' } }
      }
    ];

    cache.update({ serverId: 'origin', userId: 'origin-user' }, PresenceStatus.ONLINE);
    cache.update({ serverId: 'remote', userId: 'remote-user' }, PresenceStatus.ONLINE);
    cache.update({ serverId: 'signed-out', userId: 'signed-out-user' }, PresenceStatus.ONLINE);
    cache.update({ serverId: 'origin', userId: 'other-user' }, PresenceStatus.AWAY);

    cache.clear(authenticatedCurrentUserPresenceEntries(stores, PresenceStatus.DO_NOT_DISTURB));

    expect(cache.get({ serverId: 'origin', userId: 'origin-user' }, PresenceStatus.ONLINE)).toBe(
      PresenceStatus.DO_NOT_DISTURB
    );
    expect(cache.get({ serverId: 'remote', userId: 'remote-user' }, PresenceStatus.ONLINE)).toBe(
      PresenceStatus.DO_NOT_DISTURB
    );
    expect(
      cache.get({ serverId: 'signed-out', userId: 'signed-out-user' }, PresenceStatus.AWAY)
    ).toBe(PresenceStatus.AWAY);
    expect(cache.get({ serverId: 'origin', userId: 'other-user' }, PresenceStatus.ONLINE)).toBe(
      PresenceStatus.ONLINE
    );
  });
});
