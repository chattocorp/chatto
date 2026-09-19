import { afterEach, describe, expect, it, vi } from 'vitest';
import type { AdminMember } from '$lib/api-client/adminUsers';
import { adminMemberRowKey, createAdminMemberLoader, type AdminMemberBatch } from './adminMembers';
import {
  queryClient,
  removeAdminQueries,
  removeAdminUserQueries,
  removeServerQueries,
  refreshRoleQueries
} from './client';

function member(id: string): AdminMember {
  return {
    id,
    login: id,
    displayName: id,
    roles: [],
    deleted: false,
    hasVerifiedEmail: true,
    verifiedEmails: ['private@example.test'],
    primaryVerifiedEmail: 'private@example.test',
    viewerCanDeleteAccount: false
  };
}

function batch(ids: string[], label = 'Admin'): AdminMemberBatch {
  return { users: ids.map(member), roles: [{ name: 'admin', displayName: label }] };
}

afterEach(() => queryClient.clear());

describe('private admin member hydration', () => {
  it('coalesces concurrent requests, bounds batches, and reuses warm rows and labels', async () => {
    const fetch = vi.fn(async (ids: string[]) => batch(ids));
    const load = createAdminMemberLoader('server', 'session', fetch);
    const ids = Array.from({ length: 205 }, (_, i) => `u-${i}`);
    const [first, second] = await Promise.all([load(ids), load(ids.slice(50))]);
    expect(first.users.map((u) => u.id)).toEqual(ids);
    expect(second.users.map((u) => u.id)).toEqual(ids.slice(50));
    expect(fetch.mock.calls.map(([ids]) => ids.length)).toEqual([100, 100, 5]);
    expect(await load(ids)).toEqual(first);
    expect(fetch).toHaveBeenCalledTimes(3);
  });

  it('preserves requested order, deduplicates IDs, and omits missing and deleted users', async () => {
    const load = createAdminMemberLoader('s', 'a', async () => ({
      ...batch(['b', 'a', 'deleted']),
      users: [member('b'), member('a'), { ...member('deleted'), deleted: true }]
    }));
    expect((await load(['a', 'missing', 'b', 'a', 'deleted'])).users.map((u) => u.id)).toEqual([
      'a',
      'b'
    ]);
  });

  it('separates servers and sessions and refreshes rows and labels after role events', async () => {
    const fetch = vi.fn(async (ids: string[]) => batch(ids));
    const load = createAdminMemberLoader('s', 'a', fetch);
    await load(['u']);
    await createAdminMemberLoader('s', 'b', fetch)(['u']);
    await createAdminMemberLoader('other', 'a', fetch)(['u']);
    expect(fetch).toHaveBeenCalledTimes(3);
    fetch.mockImplementation(async (ids) => batch(ids, 'Renamed'));
    refreshRoleQueries('s');
    await vi.waitFor(() =>
      expect(queryClient.getQueryState(adminMemberRowKey('s', 'a', 'u'))?.isInvalidated).toBe(true)
    );
    expect((await load(['u'])).roles[0].displayName).toBe('Renamed');
    expect(fetch).toHaveBeenCalledTimes(4);
  });

  it.each(['logout', 'permission loss', 'deletion'] as const)(
    'fences late rows and labels after %s',
    async (boundary) => {
      let finish!: (value: AdminMemberBatch) => void;
      const fetch = vi.fn(
        (_ids: string[], _signal: AbortSignal) =>
          new Promise<AdminMemberBatch>((resolve) => {
            finish = resolve;
          })
      );
      const load = createAdminMemberLoader('s', 'a', fetch);
      const result = load(['u']);
      const rejected = expect(result).rejects.toBeDefined();
      await vi.waitFor(() => expect(fetch).toHaveBeenCalledOnce());
      if (boundary === 'logout') removeServerQueries('s');
      else if (boundary === 'permission loss') removeAdminQueries('s');
      else removeAdminUserQueries('s', 'u');
      finish(batch(['u']));
      await rejected;
      expect(fetch.mock.calls[0][1].aborted).toBe(true);
      expect(queryClient.getQueryData(adminMemberRowKey('s', 'a', 'u'))).toBeUndefined();
    }
  );

  it('rejects an aborted caller without cancelling another consumer of its batch', async () => {
    let finish!: (value: AdminMemberBatch) => void;
    const fetch = vi.fn(
      () =>
        new Promise<AdminMemberBatch>((resolve) => {
          finish = resolve;
        })
    );
    const load = createAdminMemberLoader('s', 'a', fetch);
    const controller = new AbortController();
    const first = load(['u'], controller.signal);
    const second = load(['u']);
    const rejected = expect(first).rejects.toBeDefined();
    await vi.waitFor(() => expect(fetch).toHaveBeenCalledOnce());
    controller.abort();
    finish(batch(['u']));
    await rejected;
    expect(await second).toEqual(batch(['u']));
  });
});
