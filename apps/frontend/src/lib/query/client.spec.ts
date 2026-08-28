import { Code, ConnectError } from '@connectrpc/connect';
import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  reconcileRegisteredAdminRoomGroupQueries,
  reconcileRegisteredAdminRoomQueries,
  refreshRegisteredAdminQueries,
  removeRegisteredAdminQueries,
  removeRegisteredAdminUserQueries,
  removeRegisteredServerQueries,
  registerQueryCacheRemovalListener,
  registerServerQueryCacheRemovalListener
} from './cacheRegistry';
import { queryClient } from './client';

describe('server query cache', () => {
  afterEach(() => queryClient.clear());

  it('removes only the selected server cache', () => {
    queryClient.setQueryData(['server', 'one', 'resource'], 'private-one');
    queryClient.setQueryData(['server', 'two', 'resource'], 'private-two');

    removeRegisteredServerQueries('one');

    expect(queryClient.getQueryData(['server', 'one', 'resource'])).toBeUndefined();
    expect(queryClient.getQueryData(['server', 'two', 'resource'])).toBe('private-two');
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
    queryClient.setQueryData(['server', 'one', 'session', 'scope', 'admin', 'role', 'moderator'], {
      role: { name: 'moderator' },
      users: [
        { id: 'removed', login: 'removed', displayName: 'Removed User' },
        { id: 'retained', login: 'retained', displayName: 'Retained User' }
      ],
      roles: [],
      viewerCanManageRoles: true,
      viewerCanAssignRoles: true
    });

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
      queryClient.getQueryData<{ users: Array<{ id: string }> }>([
        'server',
        'one',
        'session',
        'scope',
        'admin',
        'role',
        'moderator'
      ])?.users
    ).toEqual([{ id: 'retained', login: 'retained', displayName: 'Retained User' }]);
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

  it('does not retry authentication or permission failures', async () => {
    const queryFn = vi.fn().mockRejectedValue(new ConnectError('denied', Code.PermissionDenied));

    await expect(
      queryClient.fetchQuery({ queryKey: ['server', 'one', 'denied'], queryFn })
    ).rejects.toMatchObject({ code: Code.PermissionDenied });
    expect(queryFn).toHaveBeenCalledOnce();
  });

  it('retries one transient failure', async () => {
    const queryFn = vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValue('ok');

    await expect(
      queryClient.fetchQuery({ queryKey: ['server', 'one', 'transient'], queryFn })
    ).resolves.toBe('ok');
    expect(queryFn).toHaveBeenCalledTimes(2);
  });
});
