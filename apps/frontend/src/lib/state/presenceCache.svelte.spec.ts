import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { describe, expect, it } from 'vitest';

import { PresenceCache } from './presenceCache.svelte';

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
});
