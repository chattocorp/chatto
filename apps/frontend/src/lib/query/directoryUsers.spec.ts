import { afterEach, describe, expect, it, vi } from 'vitest';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { queryClient, removeServerQueries } from './client';
import {
  createDirectoryUserLoader,
  primeDirectoryUsers,
  removeDirectoryUser
} from './directoryUsers';

function user(id: string, displayName = id) {
  return {
    id,
    displayName,
    login: id,
    deleted: false,
    avatarUrl: null,
    roles: [],
    createdAt: null,
    customStatus: null,
    presenceStatus: PresenceStatus.ONLINE
  };
}

afterEach(() => queryClient.clear());

describe('directory user hydration', () => {
  it('coalesces concurrent rooms and fetches only unknown users in bounded batches', async () => {
    const fetch = vi.fn(async (ids: string[]) => ids.map((id) => user(id)));
    const load = createDirectoryUserLoader('server', 'session', fetch);
    primeDirectoryUsers('server', 'session', [user('known')]);
    const ids = Array.from({ length: 205 }, (_, i) => `user-${i}`);
    const [a, b] = await Promise.all([load(['known', ...ids]), load(['known', ...ids.slice(50)])]);
    expect(a).toHaveLength(206);
    expect(b).toHaveLength(156);
    expect(fetch.mock.calls.map(([batch]) => batch.length)).toEqual([100, 100, 5]);
    expect(fetch.mock.calls.flatMap(([batch]) => batch)).toEqual(ids);
    await load(['known', ...ids]);
    expect(fetch).toHaveBeenCalledTimes(3);
  });

  it('keeps sessions separate and clears entries when the server is removed', async () => {
    const fetch = vi.fn(async (ids: string[]) => ids.map((id) => user(id)));
    const first = createDirectoryUserLoader('server', 'first', fetch);
    const second = createDirectoryUserLoader('server', 'second', fetch);
    await first(['u']);
    await second(['u']);
    expect(fetch).toHaveBeenCalledTimes(2);
    removeServerQueries('server');
    await first(['u']);
    expect(fetch).toHaveBeenCalledTimes(3);
  });

  it('seeds timeline profiles without replacing newer profiles or deletion markers', async () => {
    const fetch = vi.fn(async (ids: string[]) => ids.map((id) => user(id)));
    const load = createDirectoryUserLoader('server', 'session', fetch);
    primeDirectoryUsers('server', 'session', [user('updated', 'New')]);
    removeDirectoryUser('server', 'session', 'deleted');
    primeDirectoryUsers(
      'server',
      'session',
      [user('updated', 'Old'), user('deleted'), user('missing')],
      false
    );
    expect(await load(['updated', 'deleted', 'missing'])).toEqual([
      user('updated', 'New'),
      user('missing')
    ]);
    expect(fetch).not.toHaveBeenCalled();
  });

  it('does not let an old response replace a realtime profile or restore a deleted user', async () => {
    let finish!: (users: ReturnType<typeof user>[]) => void;
    const fetch = vi.fn(
      () =>
        new Promise<ReturnType<typeof user>[]>((resolve) => {
          finish = resolve;
        })
    );
    const load = createDirectoryUserLoader('server', 'session', fetch);
    const result = load(['updated', 'deleted']);
    await vi.waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
    primeDirectoryUsers('server', 'session', [user('updated', 'New name')]);
    removeDirectoryUser('server', 'session', 'deleted');
    finish([user('updated', 'Old name'), user('deleted')]);
    expect(await result).toEqual([user('updated', 'New name')]);
    expect(await load(['updated', 'deleted'])).toEqual([user('updated', 'New name')]);
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it('does not restore profiles after session cleanup while a batch is pending', async () => {
    let finish!: (users: ReturnType<typeof user>[]) => void;
    const fetch = vi.fn(
      () =>
        new Promise<ReturnType<typeof user>[]>((resolve) => {
          finish = resolve;
        })
    );
    const load = createDirectoryUserLoader('server', 'session', fetch);
    const result = load(['u']);
    const rejected = expect(result).rejects.toBeDefined();
    await vi.waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
    removeServerQueries('server');
    finish([user('u')]);
    await rejected;
    expect(queryClient.getQueryCache().getAll()).toHaveLength(0);
  });
});
