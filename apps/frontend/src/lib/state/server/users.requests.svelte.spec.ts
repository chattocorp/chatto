import { beforeEach, describe, expect, it, vi } from 'vitest';
import { DirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import { UserStore, getUserStore, resetUserStoresForTests } from './users.svelte';
import { removeServerQueries } from '$lib/query/client';

const member = (id: string, displayName = id) => new DirectoryMember({
  user: { id, login: id, displayName }
});

beforeEach(resetUserStoresForTests);

describe('shared user requests', () => {
  it('coalesces deduplicated misses in batches of at most 100', async () => {
    const store = new UserStore();
    store.set('known', member('known'));
    const read = vi.fn(async (ids: string[]) => ids.map((id) => member(id)));
    const ids = Array.from({ length: 205 }, (_, i) => `user-${i}`);
    const [first, second] = await Promise.all([
      store.resolve(['known', '', ...ids, ids[0]], read),
      store.resolve(['known', ...ids.slice(50)], read)
    ]);
    expect(first).toHaveLength(206);
    expect(second).toHaveLength(156);
    expect(read.mock.calls.map(([batch]) => batch.length)).toEqual([100, 100, 5]);
    expect(read.mock.calls.flatMap(([batch]) => batch)).toEqual(ids);
    await store.resolve(['known', ...ids], read);
    expect(read).toHaveBeenCalledTimes(3);
  });

  it('retries only failed batches', async () => {
    const store = new UserStore();
    const ids = Array.from({ length: 201 }, (_, i) => `user-${i}`);
    const read = vi.fn(async (batch: string[]) => batch.map((id) => member(id)));
    read.mockRejectedValueOnce(new Error('offline'));
    await expect(store.resolve(ids, read)).rejects.toThrow('offline');
    await store.resolve(ids, read);
    expect(read.mock.calls.map(([batch]) => batch.length)).toEqual([100, 100, 1, 100]);
    expect(store.missing(ids)).toEqual([]);
  });

  it('retries omitted users at the caller boundary without ordering opaque cursors', async () => {
    const store = new UserStore();
    const read = vi.fn().mockResolvedValueOnce([]).mockResolvedValueOnce([member('id')]);
    const first = store.resolve(['id'], read, 'z-first');
    const later = store.resolve(['id'], read, 'a-later');
    expect(await first).toEqual([]);
    expect(await later).toEqual([member('id')]);
    expect(read.mock.calls).toEqual([[['id'], 'z-first'], [['id'], 'a-later']]);
  });

  it.each(['invalidate', 'clear'] as const)('fences pending reads after %s with an empty cache', async (boundary) => {
    const store = new UserStore();
    let finish!: (members: DirectoryMember[]) => void;
    const pending = store.resolve(['id'], () => new Promise((resolve) => { finish = resolve; }));
    const rejected = expect(pending).rejects.toThrow('Response discarded');
    await Promise.resolve();
    if (boundary === 'invalidate') store.invalidate('id');
    else store.clear();
    finish([member('id')]);
    await rejected;
    expect(store.has('id')).toBe(false);
  });

  it('does not send an obsolete request after a synchronous reset', async () => {
    const store = new UserStore();
    const read = vi.fn();
    const pending = store.resolve(['id'], read);
    store.clear();
    await expect(pending).rejects.toThrow('Response discarded');
    expect(read).not.toHaveBeenCalled();
  });

  it('preserves realtime replacements and deletion over pending reads', async () => {
    const store = new UserStore();
    let finish!: (members: DirectoryMember[]) => void;
    const pending = store.resolve(['updated', 'deleted'], () => new Promise((resolve) => { finish = resolve; }));
    await Promise.resolve();
    store.invalidate('updated');
    store.set('updated', member('updated', 'New'));
    store.delete('deleted');
    finish([member('updated', 'Old'), member('deleted')]);
    expect(await pending).toEqual([member('updated', 'New')]);
  });

  it('clears all connection scopes and fences pending reads when server queries are removed', async () => {
    const first = getUserStore('server', 'first');
    const second = getUserStore('server', 'second');
    first.set('known', member('known'));
    second.set('known', member('known'));
    let finish!: (members: DirectoryMember[]) => void;
    const pending = first.resolve(['missing'], () => new Promise((resolve) => { finish = resolve; }));
    const rejected = expect(pending).rejects.toThrow('Response discarded');
    await Promise.resolve();
    removeServerQueries('server');
    finish([member('missing')]);
    await rejected;
    expect(first.size).toBe(0);
    expect(second.size).toBe(0);
  });
});
