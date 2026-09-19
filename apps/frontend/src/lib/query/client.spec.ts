import { Code, ConnectError } from '@connectrpc/connect';
import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  reconcileRegisteredAdminRoomGroupQueries,
  reconcileRegisteredAdminRoomQueries,
  refreshRegisteredAdminQueries,
  refreshRegisteredServerQueries,
  removeRegisteredAdminQueries,
  removeRegisteredAdminUserQueries,
  removeRegisteredServerQueries,
  registerQueryCacheRemovalListener,
  registerServerQueryCacheRemovalListener
} from './cacheRegistry';
import { queryClient } from './client';
import { QueryObserver } from '@tanstack/svelte-query';

describe('server query cache', () => {
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
    queryClient.setQueryData(['server', 'one', 'session', 'scope', 'admin', 'bans'], {
      pages: [
        {
          bans: [
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
          bans: Array<{ user: unknown; moderator: unknown }>;
        }>;
      }>(['server', 'one', 'session', 'scope', 'admin', 'bans'])
    ).toMatchObject({
      pages: [
        {
          bans: [
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
