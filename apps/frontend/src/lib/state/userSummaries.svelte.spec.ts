import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  __resetUserSummaryCachesForTests,
  clearUserSummaryCache,
  getUserSummaryCache,
  primeUserSummaryCache,
  removeUserSummaryCacheEntry
} from './userSummaries.svelte';

describe('user summary cache', () => {
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
});
