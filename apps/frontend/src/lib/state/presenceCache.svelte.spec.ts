import { describe, expect, it } from 'vitest';
import { PresenceStatus } from '$lib/render/types';
import {
  authenticatedCurrentUserPresenceEntries,
  PresenceCache,
  updateAuthenticatedCurrentUserPresenceEntries
} from './presenceCache.svelte';

describe('PresenceCache', () => {
  it('replaces one server snapshot without disturbing another server', () => {
    const cache = new PresenceCache();
    cache.update({ serverId: 'origin', userId: 'old' }, PresenceStatus.Online);
    cache.update({ serverId: 'remote', userId: 'same' }, PresenceStatus.Away);

    cache.replaceServer(
      'origin',
      new Map([
        ['same', PresenceStatus.DoNotDisturb],
        ['offline', PresenceStatus.Offline]
      ])
    );

    expect(cache.get({ serverId: 'origin', userId: 'old' }, PresenceStatus.Offline)).toBe(
      PresenceStatus.Offline
    );
    expect(cache.get({ serverId: 'origin', userId: 'same' }, PresenceStatus.Offline)).toBe(
      PresenceStatus.DoNotDisturb
    );
    expect(cache.get({ serverId: 'remote', userId: 'same' }, PresenceStatus.Offline)).toBe(
      PresenceStatus.Away
    );
  });

  it('isolates entries by server id and user id', () => {
    const cache = new PresenceCache();

    cache.update({ serverId: 'origin', userId: 'same-user-id' }, PresenceStatus.Away);
    cache.update({ serverId: 'remote', userId: 'same-user-id' }, PresenceStatus.DoNotDisturb);

    expect(cache.get({ serverId: 'origin', userId: 'same-user-id' }, PresenceStatus.Online)).toBe(
      PresenceStatus.Away
    );
    expect(cache.get({ serverId: 'remote', userId: 'same-user-id' }, PresenceStatus.Online)).toBe(
      PresenceStatus.DoNotDisturb
    );
  });

  it('clears stale entries while retaining provided current-user presence', () => {
    const cache = new PresenceCache();
    cache.update({ serverId: 'origin', userId: 'current-user' }, PresenceStatus.Online);
    cache.update({ serverId: 'origin', userId: 'other-user' }, PresenceStatus.Away);

    cache.clear([[{ serverId: 'origin', userId: 'current-user' }, PresenceStatus.DoNotDisturb]]);

    expect(cache.get({ serverId: 'origin', userId: 'current-user' }, PresenceStatus.Online)).toBe(
      PresenceStatus.DoNotDisturb
    );
    expect(cache.get({ serverId: 'origin', userId: 'other-user' }, PresenceStatus.Online)).toBe(
      PresenceStatus.Online
    );
  });

  it('updates current-user presence entries across authenticated servers', () => {
    const cache = new PresenceCache();
    const lateStore = {
      serverId: 'late-remote',
      isAuthenticated: true,
      currentUser: { user: null as { id: string } | null }
    };
    cache.update({ serverId: 'origin', userId: 'origin-user' }, PresenceStatus.Online);
    cache.update({ serverId: 'remote', userId: 'remote-user' }, PresenceStatus.Online);
    cache.update({ serverId: 'late-remote', userId: 'late-remote-user' }, PresenceStatus.Online);
    cache.update({ serverId: 'signed-out', userId: 'signed-out-user' }, PresenceStatus.Online);

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
      PresenceStatus.Offline
    );

    expect(cache.get({ serverId: 'origin', userId: 'origin-user' }, PresenceStatus.Online)).toBe(
      PresenceStatus.Offline
    );
    expect(cache.get({ serverId: 'remote', userId: 'remote-user' }, PresenceStatus.Online)).toBe(
      PresenceStatus.Offline
    );
    expect(
      cache.get({ serverId: 'late-remote', userId: 'late-remote-user' }, PresenceStatus.Away)
    ).toBe(PresenceStatus.Online);
    expect(
      cache.get({ serverId: 'signed-out', userId: 'signed-out-user' }, PresenceStatus.Away)
    ).toBe(PresenceStatus.Online);

    lateStore.currentUser.user = { id: 'late-remote-user' };
    updateAuthenticatedCurrentUserPresenceEntries(cache, [lateStore], PresenceStatus.Offline);

    expect(
      cache.get({ serverId: 'late-remote', userId: 'late-remote-user' }, PresenceStatus.Online)
    ).toBe(PresenceStatus.Offline);
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

    cache.update({ serverId: 'origin', userId: 'origin-user' }, PresenceStatus.Online);
    cache.update({ serverId: 'remote', userId: 'remote-user' }, PresenceStatus.Online);
    cache.update({ serverId: 'signed-out', userId: 'signed-out-user' }, PresenceStatus.Online);
    cache.update({ serverId: 'origin', userId: 'other-user' }, PresenceStatus.Away);

    cache.clear(authenticatedCurrentUserPresenceEntries(stores, PresenceStatus.DoNotDisturb));

    expect(cache.get({ serverId: 'origin', userId: 'origin-user' }, PresenceStatus.Online)).toBe(
      PresenceStatus.DoNotDisturb
    );
    expect(cache.get({ serverId: 'remote', userId: 'remote-user' }, PresenceStatus.Online)).toBe(
      PresenceStatus.DoNotDisturb
    );
    expect(
      cache.get({ serverId: 'signed-out', userId: 'signed-out-user' }, PresenceStatus.Away)
    ).toBe(PresenceStatus.Away);
    expect(cache.get({ serverId: 'origin', userId: 'other-user' }, PresenceStatus.Online)).toBe(
      PresenceStatus.Online
    );
  });
});
