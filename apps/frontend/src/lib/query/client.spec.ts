import { Code, ConnectError } from '@connectrpc/connect';
import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  removeRegisteredAdminQueries,
  removeRegisteredAdminUserQueries,
  removeRegisteredServerQueries,
  registerQueryCacheRemovalListener
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
    const unregister = registerQueryCacheRemovalListener(listener);

    removeRegisteredAdminQueries('one');
    removeRegisteredServerQueries('two');

    expect(listener).toHaveBeenNthCalledWith(1, 'one');
    expect(listener).toHaveBeenNthCalledWith(2, 'two');
    unregister();
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
