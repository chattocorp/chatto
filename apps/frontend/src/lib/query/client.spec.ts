import { Code, ConnectError } from '@connectrpc/connect';
import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  reconcileRegisteredAdminRoomGroupQueries,
  reconcileRegisteredAdminRoomQueries,
  refreshRegisteredAdminQueries,
  refreshRegisteredServerQueries,
  refreshRegisteredAdminProfileQueries,
  removeRegisteredAdminQueries,
  removeRegisteredAdminUserQueries,
  removeRegisteredServerQueries,
  registerQueryCacheRemovalListener,
  registerServerQueryCacheRemovalListener
} from './cacheRegistry';
import { queryClient } from './client';
import { onlineManager, QueryObserver } from '@tanstack/svelte-query';

describe('server query cache', () => {
  it('refreshes a shared active dependency once before its parent uses the result', async () => {
    const parentKey = ['server', 'one', 'parent'];
    const childKey = ['server', 'one', 'child'];
    queryClient.setQueryData(parentKey, 'old-parent');
    queryClient.setQueryData(childKey, 'old-child');
    let resolveChild!: (value: string) => void;
    const readChild = vi.fn(() => new Promise<string>((resolve) => { resolveChild = resolve; }));
    const childOptions = { queryKey: childKey, queryFn: readChild, staleTime: Infinity };
    const parent = new QueryObserver(queryClient, {
      queryKey: parentKey,
      staleTime: Infinity,
      queryFn: () => queryClient.fetchQuery(childOptions)
    });
    const child = new QueryObserver(queryClient, childOptions);
    const unsubscribeParent = parent.subscribe(() => {});
    const unsubscribeChild = child.subscribe(() => {});
    try {
      const refreshing = refreshRegisteredServerQueries('one');
      await vi.waitFor(() => expect(readChild).toHaveBeenCalledOnce());
      resolveChild('fresh-child');
      await refreshing;
      expect(parent.getCurrentResult().data).toBe('fresh-child');
      expect(child.getCurrentResult().data).toBe('fresh-child');
      expect(readChild).toHaveBeenCalledOnce();
    } finally {
      unsubscribeParent();
      unsubscribeChild();
    }
  });

  it('combines simultaneous refreshes and shared observers into one read', async () => {
    const read = vi.fn(async () => 'fresh');
    const options = { queryKey: ['server', 'one', 'shared'], queryFn: read, staleTime: Infinity };
    queryClient.setQueryData(options.queryKey, 'before');
    const first = new QueryObserver(queryClient, options);
    const second = new QueryObserver(queryClient, options);
    const unsubscribeFirst = first.subscribe(() => {});
    const unsubscribeSecond = second.subscribe(() => {});
    const inactiveRead = vi.fn(async () => 'inactive');
    const inactiveKey = ['server', 'one', 'inactive'];
    await queryClient.prefetchQuery({ queryKey: inactiveKey, queryFn: inactiveRead });
    inactiveRead.mockClear();
    try {
      await Promise.all([
        refreshRegisteredServerQueries('one'),
        refreshRegisteredServerQueries('one'),
        refreshRegisteredServerQueries('one')
      ]);
      expect(read).toHaveBeenCalledOnce();
      expect(first.getCurrentResult().data).toBe('fresh');
      expect(second.getCurrentResult().data).toBe('fresh');
      expect(inactiveRead).not.toHaveBeenCalled();
      expect(queryClient.getQueryData(inactiveKey)).toBeUndefined();
    } finally {
      unsubscribeFirst();
      unsubscribeSecond();
    }
  });

  it.each([0, Infinity])('discards inactive dependencies before nested reads with staleTime=%s', async (staleTime) => {
    const parentKey = ['server', 'one', 'parent'];
    const childKey = ['server', 'one', 'child'];
    // Keep the parent first in cache iteration order, as on its initial load.
    queryClient.setQueryData(parentKey, 'old-parent');
    queryClient.setQueryData(childKey, 'old-child');
    const observer = new QueryObserver(queryClient, {
      queryKey: parentKey,
      staleTime: Infinity,
      retry: false,
      queryFn: () => queryClient.fetchQuery({
        queryKey: childKey, queryFn: async () => 'fresh-child', staleTime, retry: false
      })
    });
    const unsubscribe = observer.subscribe(() => {});
    await refreshRegisteredServerQueries('one');
    expect(observer.getCurrentResult().data).toBe('fresh-child');
    expect(queryClient.getQueryData(childKey)).toBe('fresh-child');
    unsubscribe();
  });

  it('keeps the latest authorization result when refreshes overlap', async () => {
    const queryKey = ['server', 'one', 'resource'];
    let resolveOld!: (value: string) => void;
    const read = vi.fn()
      .mockImplementationOnce(() => new Promise<string>((resolve) => { resolveOld = resolve; }))
      .mockResolvedValueOnce('latest');
    queryClient.setQueryData(queryKey, 'before');
    const observer = new QueryObserver(queryClient, { queryKey, queryFn: read, staleTime: Infinity });
    const unsubscribe = observer.subscribe(() => {});
    const first = refreshRegisteredServerQueries('one');
    await vi.waitFor(() => expect(read).toHaveBeenCalledOnce());
    const second = refreshRegisteredServerQueries('one');
    await second;
    resolveOld('stale-private');
    await first;
    expect(observer.getCurrentResult().data).toBe('latest');
    unsubscribe();
  });

  it('replaces a pending first load instead of accepting its old authority', async () => {
    let resolveOld!: (value: string) => void;
    const read = vi.fn()
      .mockImplementationOnce(() => new Promise<string>((resolve) => { resolveOld = resolve; }))
      .mockResolvedValueOnce('fresh');
    const observer = new QueryObserver(queryClient, {
      queryKey: ['server', 'one', 'first-load'], queryFn: read, retry: false
    });
    const unsubscribe = observer.subscribe(() => {});
    try {
      await refreshRegisteredServerQueries('one');
      resolveOld('stale-private');
      await Promise.resolve();
      expect(read).toHaveBeenCalledTimes(2);
      expect(observer.getCurrentResult().data).toBe('fresh');
    } finally {
      unsubscribe();
    }
  });

  it('clears an offline snapshot without sending requests until the client reconnects', async () => {
    const queryKey = ['server', 'one', 'offline'];
    const read = vi.fn(async () => 'fresh');
    queryClient.setQueryData(queryKey, 'private-before');
    const observer = new QueryObserver(queryClient, {
      queryKey, queryFn: read, staleTime: Infinity, retry: false
    });
    const unsubscribe = observer.subscribe(() => {});
    queryClient.mount();
    onlineManager.setOnline(false);
    try {
      await refreshRegisteredServerQueries('one');
      expect(observer.getCurrentResult().data).toBeUndefined();
      expect(observer.getCurrentResult().fetchStatus).toBe('paused');
      expect(read).not.toHaveBeenCalled();
      onlineManager.setOnline(true);
      await vi.waitFor(() => expect(observer.getCurrentResult().data).toBe('fresh'));
      expect(read).toHaveBeenCalledOnce();
    } finally {
      onlineManager.setOnline(true);
      unsubscribe();
      queryClient.unmount();
    }
  });

  it('clears a failed permission read before slower checks finish, then restores normal error behavior', async () => {
    const failedKey = ['server', 'one', 'failed'];
    const slowKey = ['server', 'one', 'slow'];
    queryClient.setQueryData(failedKey, 'private-before');
    queryClient.setQueryData(slowKey, 'retained');
    let resolveSlow!: (value: string) => void;
    const failed = new QueryObserver(queryClient, {
      queryKey: failedKey,
      queryFn: async () => { throw new ConnectError('offline', Code.Unavailable); },
      staleTime: Infinity,
      retry: false
    });
    const slow = new QueryObserver(queryClient, {
      queryKey: slowKey,
      queryFn: () => new Promise<string>((resolve) => { resolveSlow = resolve; }),
      staleTime: Infinity
    });
    const unsubscribeFailed = failed.subscribe(() => {});
    const unsubscribeSlow = slow.subscribe(() => {});
    try {
      const refreshing = refreshRegisteredServerQueries('one');
      await vi.waitFor(() => expect(failed.getCurrentResult().isError).toBe(true));
      expect(failed.getCurrentResult().data).toBeUndefined();
      expect(slow.getCurrentResult().data).toBe('retained');
      resolveSlow('fresh');
      await refreshing;

      queryClient.setQueryData(failedKey, 'authorized-after');
      await failed.refetch();
      expect(failed.getCurrentResult().data).toBe('authorized-after');
    } finally {
      unsubscribeFailed();
      unsubscribeSlow();
    }
  });

  it.each(['allowed', 'denied', 'offline'])('reauthorizes in place and fences older data: %s', async (outcome) => {
    const queryKey = ['server', 'one', 'session', 'scope', 'admin', 'permission-tier'];
    let resolveOld!: (value: string) => void;
    let resolveFresh!: (value: string) => void;
    let rejectFresh!: (error: Error) => void;
    const read = vi.fn()
      .mockImplementationOnce(() => new Promise<string>((resolve) => { resolveOld = resolve; }))
      .mockImplementationOnce(() => new Promise<string>((resolve, reject) => {
        resolveFresh = resolve;
        rejectFresh = reject;
      }));
    queryClient.setQueryData(queryKey, 'authorized-before');
    queryClient.setQueryData(['server', 'one', 'inactive'], 'inactive-private');
    queryClient.setQueryData(['server', 'two', 'resource'], 'unrelated');
    const observer = new QueryObserver(queryClient, { queryKey, queryFn: read, staleTime: 0, retry: false });
    const unsubscribe = observer.subscribe(() => {});
    const query = observer.getCurrentQuery();
    const refreshing = refreshRegisteredServerQueries('one');
    await vi.waitFor(() => expect(read).toHaveBeenCalledTimes(2));
    expect(observer.getCurrentResult().data).toBe('authorized-before');
    expect(queryClient.getQueryData(['server', 'one', 'inactive'])).toBeUndefined();
    resolveOld('stale-response');
    await Promise.resolve();
    expect(observer.getCurrentResult().data).toBe('authorized-before');
    if (outcome === 'allowed') resolveFresh('authorized-after');
    else rejectFresh(new ConnectError(outcome, outcome === 'denied' ? Code.PermissionDenied : Code.Unavailable));
    await refreshing;
    expect(observer.getCurrentQuery()).toBe(query);
    expect(observer.getCurrentResult().data).toBe(outcome === 'allowed' ? 'authorized-after' : undefined);
    expect(queryClient.getQueryData(['server', 'two', 'resource'])).toBe('unrelated');
    unsubscribe();
  });

  it('clears private data and fences late reads when the session is removed', async () => {
    const queryKey = ['server', 'one', 'resource'];
    let resolveOld!: (value: string) => void;
    const read = vi
      .fn()
      .mockImplementationOnce(
        () =>
          new Promise<string>((resolve) => {
            resolveOld = resolve;
          })
      );
    queryClient.setQueryData(queryKey, 'private-before');
    const observer = new QueryObserver(queryClient, { queryKey, queryFn: read, staleTime: 0 });
    const unsubscribe = observer.subscribe(() => {});
    const query = observer.getCurrentQuery();
    expect(read).toHaveBeenCalledTimes(1);
    removeRegisteredServerQueries('one');
    expect(observer.getCurrentResult().data).toBeUndefined();
    expect(read).toHaveBeenCalledTimes(1);
    resolveOld('revoked-data');
    await Promise.resolve();
    expect(observer.getCurrentResult().data).toBeUndefined();
    expect(queryClient.getQueryData(queryKey)).toBeUndefined();
    expect(observer.getCurrentQuery()).toBe(query);
    unsubscribe();
  });

  afterEach(() => queryClient.clear());

  it('removes only the selected server cache', () => {
    queryClient.setQueryData(['server', 'one', 'resource'], 'private-one');
    queryClient.setQueryData(['server', 'two', 'resource'], 'private-two');

    removeRegisteredServerQueries('one');

    expect(queryClient.getQueryData(['server', 'one', 'resource'])).toBeUndefined();
    expect(queryClient.getQueryData(['server', 'two', 'resource'])).toBe('private-two');
  });

  it('reports a failed privacy fence but still clears private cached data', () => {
    const key = ['server', 'one', 'resource'];
    queryClient.setQueryData(key, 'private');
    const unregister = registerQueryCacheRemovalListener(() => {
      throw new Error('fence failed');
    });
    const log = vi.spyOn(console, 'error').mockImplementation(() => {});
    try {
      expect(removeRegisteredServerQueries('one')).toBe(false);
      expect(queryClient.getQueryData(key)).toBeUndefined();
    } finally {
      unregister();
      log.mockRestore();
    }
  });

  it('notifies late-mutation fences before server and admin cache removal', () => {
    const listener = vi.fn();
    const serverListener = vi.fn();
    const unregister = registerQueryCacheRemovalListener(listener);
    const unregisterServer = registerServerQueryCacheRemovalListener(serverListener);

    removeRegisteredAdminQueries('one');
    removeRegisteredServerQueries('two');

    expect(listener).toHaveBeenNthCalledWith(1, 'one');
    expect(listener).toHaveBeenNthCalledWith(2, 'two');
    expect(serverListener).toHaveBeenCalledOnce();
    expect(serverListener).toHaveBeenCalledWith('two');
    unregister();
    unregisterServer();
  });

  it('removes admin data without discarding unrelated server queries', () => {
    queryClient.setQueryData(
      ['server', 'one', 'session', 'scope', 'admin', 'members'],
      'private-admin'
    );
    queryClient.setQueryData(['server', 'one', 'resource'], 'ordinary-snapshot');

    removeRegisteredAdminQueries('one');

    expect(
      queryClient.getQueryData(['server', 'one', 'session', 'scope', 'admin', 'members'])
    ).toBeUndefined();
    expect(queryClient.getQueryData(['server', 'one', 'resource'])).toBe('ordinary-snapshot');
  });

  it('keeps mounted admin data while scheduling its authoritative refresh', () => {
    const key = ['server', 'one', 'session', 'scope', 'admin', 'permission-tier'] as const;
    queryClient.setQueryData(key, 'stable-matrix');

    refreshRegisteredAdminQueries('one');

    expect(queryClient.getQueryData(key)).toBe('stable-matrix');
  });

  it('refreshes profile data without treating an edit as an authorization reset', async () => {
    const key = ['server', 'one', 'session', 'scope', 'admin', 'members', 'row', 'user'];
    queryClient.setQueryData(key, 'profile');
    const removed = vi.fn();
    const unregister = registerQueryCacheRemovalListener(removed);
    try {
      refreshRegisteredAdminProfileQueries('one');
      await vi.waitFor(() => expect(queryClient.getQueryState(key)?.isInvalidated).toBe(true));
      expect(removed).not.toHaveBeenCalled();
      removeRegisteredAdminQueries('one');
      expect(removed).toHaveBeenCalledWith('one');
    } finally {
      unregister();
    }
  });

  it('scrubs member lists and the removed member detail only', () => {
    queryClient.setQueryData(
      ['server', 'one', 'session', 'scope', 'admin', 'members', { search: '' }],
      {
        pages: [{ users: [{ id: 'removed' }, { id: 'retained' }] }],
        pageParams: []
      }
    );
    queryClient.setQueryData(
      ['server', 'one', 'session', 'scope', 'admin', 'member', 'removed'],
      'private-removed'
    );
    queryClient.setQueryData(
      ['server', 'one', 'session', 'scope', 'admin', 'member', 'retained'],
      'private-retained'
    );
    queryClient.setQueryData(
      ['server', 'one', 'session', 'scope', 'admin', 'user-permissions', 'removed'],
      'private-removed-permissions'
    );
    queryClient.setQueryData(
      ['server', 'one', 'session', 'scope', 'admin', 'user-permissions', 'retained'],
      'private-retained-permissions'
    );
    queryClient.setQueryData(['server', 'one', 'session', 'scope', 'admin', 'suspensions'], {
      pages: [
        {
          suspensions: [
            {
              id: 'ban-1',
              userId: 'removed',
              user: { id: 'removed', displayName: 'Removed User' },
              moderatorId: 'retained',
              moderator: { id: 'retained', displayName: 'Retained Moderator' }
            },
            {
              id: 'ban-2',
              userId: 'retained',
              user: { id: 'retained', displayName: 'Retained User' },
              moderatorId: 'removed',
              moderator: { id: 'removed', displayName: 'Removed Moderator' }
            }
          ]
        }
      ],
      pageParams: [0]
    });
    queryClient.setQueryData(
      ['server', 'one', 'session', 'scope', 'admin', 'role-members', 'moderator'],
      {
        pages: [
          {
            users: [
              { id: 'removed', login: 'removed', displayName: 'Removed User' },
              { id: 'retained', login: 'retained', displayName: 'Retained User' }
            ],
            totalCount: 2,
            hasMore: false
          }
        ],
        pageParams: [0]
      }
    );

    removeRegisteredAdminUserQueries('one', 'removed');

    expect(
      queryClient.getQueryData([
        'server',
        'one',
        'session',
        'scope',
        'admin',
        'members',
        { search: '' }
      ])
    ).toEqual({ pages: [{ users: [{ id: 'retained' }] }], pageParams: [] });
    expect(
      queryClient.getQueryData(['server', 'one', 'session', 'scope', 'admin', 'member', 'removed'])
    ).toBeUndefined();
    expect(
      queryClient.getQueryData(['server', 'one', 'session', 'scope', 'admin', 'member', 'retained'])
    ).toBe('private-retained');
    expect(
      queryClient.getQueryData([
        'server',
        'one',
        'session',
        'scope',
        'admin',
        'user-permissions',
        'removed'
      ])
    ).toBeUndefined();
    expect(
      queryClient.getQueryData([
        'server',
        'one',
        'session',
        'scope',
        'admin',
        'user-permissions',
        'retained'
      ])
    ).toBe('private-retained-permissions');
    expect(
      queryClient.getQueryData<{
        pages: Array<{
          suspensions: Array<{ user: unknown; moderator: unknown }>;
        }>;
      }>(['server', 'one', 'session', 'scope', 'admin', 'suspensions'])
    ).toMatchObject({
      pages: [
        {
          suspensions: [
            { user: null, moderator: { id: 'retained' } },
            { user: { id: 'retained' }, moderator: null }
          ]
        }
      ]
    });
    expect(
      queryClient.getQueryData<{ pages: Array<{ users: Array<{ id: string }> }> }>([
        'server',
        'one',
        'session',
        'scope',
        'admin',
        'role-members',
        'moderator'
      ])?.pages[0].users
    ).toEqual([{ id: 'retained', login: 'retained', displayName: 'Retained User' }]);
  });

  it('fences a late role-member page after deleted-user cleanup', async () => {
    const key = ['server', 'one', 'session', 'scope', 'admin', 'role-members', 'moderator'];
    const data = { pages: [{ users: [{ id: 'removed' }, { id: 'retained' }] }], pageParams: [0] };
    queryClient.setQueryData(key, data);
    let finish!: (value: typeof data) => void;
    const oldRead = queryClient
      .fetchQuery({
        queryKey: key,
        staleTime: 0,
        queryFn: () =>
          new Promise<typeof data>((resolve) => {
            finish = resolve;
          })
      })
      .catch(() => undefined);
    removeRegisteredAdminUserQueries('one', 'removed');
    finish(data);
    await oldRead;
    expect(queryClient.getQueryData(key)).toEqual({
      pages: [{ users: [{ id: 'retained' }] }],
      pageParams: [0]
    });
  });

  it('invalidates room details across sessions and purges removed permission snapshots', () => {
    const roomOne = ['server', 'one', 'session', 'scope-a', 'admin', 'room', 'R1'] as const;
    const roomTwo = ['server', 'one', 'session', 'scope-b', 'admin', 'room', 'R1'] as const;
    const permissions = [
      'server',
      'one',
      'session',
      'scope-a',
      'admin',
      'permission-tier',
      { roomId: 'R1', groupId: null }
    ] as const;
    queryClient.setQueryData(roomOne, 'private-room-a');
    queryClient.setQueryData(roomTwo, 'private-room-b');
    queryClient.setQueryData(permissions, 'private-permissions');

    reconcileRegisteredAdminRoomQueries('one', 'R1');
    expect(queryClient.getQueryState(roomOne)?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(roomTwo)?.isInvalidated).toBe(true);
    expect(queryClient.getQueryData(permissions)).toBe('private-permissions');

    reconcileRegisteredAdminRoomQueries('one', 'R1', true);
    expect(queryClient.getQueryData(roomOne)).toBeNull();
    expect(queryClient.getQueryData(roomTwo)).toBeNull();
    expect(queryClient.getQueryData(permissions)).toBeUndefined();
  });

  it('invalidates visible groups and purges groups omitted from a replacement', () => {
    const visibleGroup = [
      'server',
      'one',
      'session',
      'scope',
      'admin',
      'room-group',
      'G1'
    ] as const;
    const removedGroup = [
      'server',
      'one',
      'session',
      'scope',
      'admin',
      'room-group',
      'G2'
    ] as const;
    const removedPermissions = [
      'server',
      'one',
      'session',
      'scope',
      'admin',
      'permission-tier',
      { roomId: null, groupId: 'G2' }
    ] as const;
    const orphanedPermissions = [
      'server',
      'one',
      'session',
      'scope',
      'admin',
      'permission-tier',
      { roomId: null, groupId: 'G3' }
    ] as const;
    queryClient.setQueryData(visibleGroup, 'visible-group');
    queryClient.setQueryData(removedGroup, 'removed-group');
    queryClient.setQueryData(removedPermissions, 'private-permissions');
    queryClient.setQueryData(orphanedPermissions, 'orphaned-private-permissions');

    reconcileRegisteredAdminRoomGroupQueries('one', ['G1']);

    expect(queryClient.getQueryState(visibleGroup)?.isInvalidated).toBe(true);
    expect(queryClient.getQueryData(removedGroup)).toBeNull();
    expect(queryClient.getQueryData(removedPermissions)).toBeUndefined();
    expect(queryClient.getQueryData(orphanedPermissions)).toBeUndefined();
  });

  it.each([Code.FailedPrecondition, Code.PermissionDenied, Code.Unauthenticated])(
    'does not retry permanent Connect failure %s',
    async (code) => {
      const queryFn = vi.fn().mockRejectedValue(new ConnectError('permanent failure', code));

      await expect(
        queryClient.fetchQuery({ queryKey: ['server', 'one', 'permanent', code], queryFn })
      ).rejects.toMatchObject({ code });
      expect(queryFn).toHaveBeenCalledOnce();
    }
  );

  it('retries one transient failure', async () => {
    const queryFn = vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValue('ok');

    await expect(
      queryClient.fetchQuery({ queryKey: ['server', 'one', 'transient'], queryFn })
    ).resolves.toBe('ok');
    expect(queryFn).toHaveBeenCalledTimes(2);
  });
});
