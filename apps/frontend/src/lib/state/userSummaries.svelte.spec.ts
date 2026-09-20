import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  __resetUserSummaryCachesForTests,
  clearUserSummaryCache,
  getUserSummaryCache,
  primeUserSummaryCache,
  removeUserSummaryCacheEntry
} from './userSummaries.svelte';

describe('user summary cache', () => {
  const user = (id: string, displayName = id) => ({ id, login: id, displayName, deleted: false, avatarUrl: null });
  beforeEach(() => {
    __resetUserSummaryCachesForTests();
  });

  it('scopes summaries by server id', () => {
    primeUserSummaryCache('server-a', [
      {
        id: 'U1',
        login: 'alice',
        displayName: 'Alice',
        deleted: false,
        avatarUrl: null
      }
    ]);

    expect(getUserSummaryCache('server-a').get('U1')?.login).toBe('alice');
    expect(getUserSummaryCache('server-b').get('U1')).toBeNull();
  });

  it('removes erased users and clears stale entries on projection reset', () => {
    primeUserSummaryCache('server-a', [
      { id: 'U1', login: 'ada', displayName: 'Ada', deleted: false, avatarUrl: null },
      { id: 'U2', login: 'grace', displayName: 'Grace', deleted: false, avatarUrl: null }
    ]);

    removeUserSummaryCacheEntry('server-a', 'U1');
    expect(getUserSummaryCache('server-a').get('U1')).toBeNull();
    expect(getUserSummaryCache('server-a').get('U2')?.displayName).toBe('Grace');

    clearUserSummaryCache('server-a');
    expect(getUserSummaryCache('server-a').get('U2')).toBeNull();
  });

  it('loads only deduped cache misses through the batch API', async () => {
    const cache = getUserSummaryCache('server-a');
    cache.prime([
      {
        id: 'U1',
        login: 'alice',
        displayName: 'Alice',
        deleted: false,
        avatarUrl: null
      }
    ]);
    const batchGetUsers = vi.fn().mockResolvedValue([
      {
        id: 'U2',
        login: 'bob',
        displayName: 'Bob',
        deleted: false,
        avatarUrl: 'https://cdn/bob.webp'
      }
    ]);

    await cache.loadMissing({ batchGetUsers }, ['U1', 'U2', 'U2', '', 'U3']);

    expect(batchGetUsers).toHaveBeenCalledWith(['U2', 'U3']);
    expect(cache.get('U1')?.login).toBe('alice');
    expect(cache.get('U2')?.avatarUrl).toBe('https://cdn/bob.webp');
    expect(cache.get('U3')).toBeNull();
  });

  it('shares missing author reads across concurrent consumers and reuses cached authors', async () => {
    const cache = getUserSummaryCache('server-a');
    cache.prime([user('known')]);
    const read = vi.fn().mockResolvedValue([user('missing')]);
    const first = cache.resolve(['known', 'missing'], read, 'first-cursor');
    const second = cache.resolve(['missing'], read, 'later-cursor');
    expect(await first).toEqual([user('known'), user('missing')]);
    expect(await second).toEqual([user('missing')]);
    await cache.resolve(['known', 'missing'], read);
    expect(read).toHaveBeenCalledExactlyOnceWith(['missing'], 'first-cursor');
  });

  it.each(['remove', 'clear'] as const)('fences pending authors after %s, even with an empty cache', async (boundary) => {
    const cache = getUserSummaryCache('server-a');
    let finish!: (users: ReturnType<typeof user>[]) => void;
    const read = vi.fn(() => new Promise<ReturnType<typeof user>[]>((resolve) => { finish = resolve; }));
    const pending = cache.resolve(['U1'], read);
    const rejected = expect(pending).rejects.toThrow('Response discarded');
    await Promise.resolve();
    if (boundary === 'remove') cache.remove('U1');
    else cache.clear();
    finish([user('U1')]);
    await rejected;
    expect(cache.get('U1')).toBeNull();
  });

  it('preserves a newer profile when an older author read completes', async () => {
    const cache = getUserSummaryCache('server-a');
    let finish!: (users: ReturnType<typeof user>[]) => void;
    const pending = cache.resolve(['U1'], () => new Promise((resolve) => { finish = resolve; }));
    await Promise.resolve();
    cache.remove('U1');
    cache.prime([user('U1', 'new')]);
    finish([user('U1', 'old')]);
    expect(await pending).toEqual([user('U1', 'new')]);
  });

  it('does not start an obsolete user request after a synchronous reset', async () => {
    const cache = getUserSummaryCache('server-a');
    const read = vi.fn().mockResolvedValue([user('U1')]);
    const pending = cache.resolve(['U1'], read);
    cache.clear();
    await expect(pending).rejects.toThrow('Response discarded');
    expect(read).not.toHaveBeenCalled();
  });

  it('retries an omitted user when a shared read had a different cursor', async () => {
    const cache = getUserSummaryCache('server-a');
    const read = vi.fn().mockResolvedValueOnce([]).mockResolvedValueOnce([user('U1')]);
    const first = cache.resolve(['U1'], read, 'z-first');
    const later = cache.resolve(['U1'], read, 'a-later');
    expect(await first).toEqual([]);
    expect(await later).toEqual([user('U1')]);
    expect(read.mock.calls).toEqual([[['U1'], 'z-first'], [['U1'], 'a-later']]);
  });

  it('bounds missing-user batches and permits retry after a failed read', async () => {
    const cache = getUserSummaryCache('server-a');
    const ids = Array.from({ length: 201 }, (_, index) => `U${index}`);
    const read = vi.fn(async (batch: string[]) => batch.map((id) => user(id)));
    read.mockRejectedValueOnce(new Error('offline'));
    await expect(cache.resolve(ids, read, 'cursor')).rejects.toThrow('offline');
    await cache.resolve(ids, read, 'retry');
    expect(read.mock.calls.map(([batch]) => batch.length)).toEqual([100, 100, 1, 100]);
    expect(cache.missing(ids)).toEqual([]);
  });
});
