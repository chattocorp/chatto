import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AdminRoleService } from '@chatto/api-types/admin/v1/roles_connect';
import { RoleService } from '@chatto/api-types/api/v1/roles_connect';
import { createRoleAPI } from '../roles.js';
import { fakeServer, mockService, receivedRequest } from '../../testing/fakeServer.js';

const roles = mockService(RoleService);
const adminRoles = mockService(AdminRoleService);

function roleAPI() {
  return createRoleAPI(
    fakeServer((router) => router.service(RoleService, roles).service(AdminRoleService, adminRoles))
  );
}

describe('createRoleAPI', () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it('lists public roles', async () => {
    roles.listRoles.mockReturnValue({
      roles: [
        {
          name: 'moderator',
          displayName: 'Moderator',
          description: 'Moderates rooms',
          isSystem: true,
          position: 100,
          pingable: true
        }
      ]
    });
    const api = roleAPI();

    const result = await api.listRoles();

    expect(roles.listRoles).toHaveBeenCalledOnce();
    expect(result).toEqual({
      roles: [
        {
          name: 'moderator',
          displayName: 'Moderator',
          description: 'Moderates rooms',
          permissions: [],
          permissionDenials: [],
          isSystem: true,
          position: 100,
          pingable: true
        }
      ],
      viewerCanManageRoles: false,
      viewerCanAssignRoles: false
    });
  });

  it('gets and batch gets public roles', async () => {
    const role = {
      name: 'moderator',
      displayName: 'Moderator',
      description: 'Moderates rooms',
      isSystem: true,
      position: 100,
      pingable: true
    };
    roles.getRole.mockReturnValue({ role });
    roles.batchGetRoles.mockReturnValue({ roles: [role] });
    const api = roleAPI();

    await expect(api.getPublicRole('moderator')).resolves.toMatchObject({
      name: 'moderator',
      permissions: []
    });
    await expect(api.batchGetPublicRoles(['moderator', 'missing'])).resolves.toMatchObject([
      { name: 'moderator' }
    ]);

    expect(receivedRequest(roles.getRole)).toMatchObject({ name: 'moderator' });
    expect(receivedRequest(roles.batchGetRoles)).toMatchObject({ names: ['moderator', 'missing'] });
  });

  it('lists admin roles with viewer capabilities', async () => {
    adminRoles.listRoles.mockReturnValue({
      roles: [
        {
          role: {
            name: 'moderator',
            displayName: 'Moderator',
            description: 'Moderates rooms',
            isSystem: true,
            position: 100,
            pingable: true
          },
          permissions: ['room.manage'],
          permissionDenials: ['message.post']
        }
      ],
      viewerCanManageRoles: true,
      viewerCanAssignRoles: false
    });
    const api = roleAPI();
    const result = await api.listAdminRoles();

    expect(adminRoles.listRoles).toHaveBeenCalledOnce();
    expect(result).toEqual({
      roles: [
        {
          name: 'moderator',
          displayName: 'Moderator',
          description: 'Moderates rooms',
          permissions: ['room.manage'],
          permissionDenials: ['message.post'],
          isSystem: true,
          position: 100,
          pingable: true
        }
      ],
      viewerCanManageRoles: true,
      viewerCanAssignRoles: false
    });
  });

  it('gets role metadata', async () => {
    adminRoles.getRole.mockReturnValue({
      role: {
        role: {
          name: 'helpdesk',
          displayName: 'Helpdesk',
          description: '',
          isSystem: false,
          position: 10,
          pingable: false
        },
        permissions: [],
        permissionDenials: []
      },
      viewerCanManageRoles: true,
      viewerCanAssignRoles: true
    });
    const api = roleAPI();
    const result = await api.getRole('helpdesk');

    expect(receivedRequest(adminRoles.getRole)).toMatchObject({ name: 'helpdesk' });
    expect(result).toEqual({
      roles: [],
      role: {
        name: 'helpdesk',
        displayName: 'Helpdesk',
        description: '',
        permissions: [],
        permissionDenials: [],
        isSystem: false,
        position: 10,
        pingable: false
      },
      viewerCanManageRoles: true,
      viewerCanAssignRoles: true
    });
  });

  it('loads an explicit role member page with cancellation', async () => {
    const user = { id: 'user-1', login: 'alice', displayName: 'Alice', isBot: true };
    adminRoles.listMembers.mockReturnValue({
      members: [
        { id: 'user-1', login: 'alice', displayName: 'Alice', bot: { ownerUserId: 'owner' } }
      ],
      page: { totalCount: 31n, hasMore: true }
    });
    const api = roleAPI();
    expect(await api.listMembers('helpdesk', { limit: 20, offset: 20 })).toEqual({
      users: [user],
      totalCount: 31,
      hasMore: true
    });
    expect(receivedRequest(adminRoles.listMembers)).toMatchObject({
      name: 'helpdesk',
      page: { limit: 20, offset: 20 }
    });

    await expect(
      api.listMembers('helpdesk', { limit: 20, offset: 20 }, { signal: AbortSignal.abort() })
    ).rejects.toMatchObject({ code: Code.Canceled });
  });

  it('creates updates and deletes roles', async () => {
    const role = {
      name: 'helpdesk',
      displayName: 'Helpdesk',
      description: 'Support queue',
      permissions: [],
      permissionDenials: [],
      isSystem: false,
      position: 10,
      pingable: true
    };
    const apiRole = {
      role: {
        name: role.name,
        displayName: role.displayName,
        description: role.description,
        isSystem: role.isSystem,
        position: role.position,
        pingable: role.pingable
      },
      permissions: role.permissions,
      permissionDenials: role.permissionDenials
    };
    adminRoles.createRole.mockReturnValue({ role: apiRole });
    adminRoles.updateRole.mockReturnValue({
      role: { ...apiRole, role: { ...apiRole.role, displayName: 'Support' } }
    });
    adminRoles.deleteRole.mockReturnValue({});
    const api = roleAPI();

    await expect(api.createRole(role)).resolves.toEqual(role);
    await expect(
      api.updateRole({
        name: 'helpdesk',
        displayName: 'Support',
        description: 'Support queue',
        pingable: false
      })
    ).resolves.toMatchObject({ displayName: 'Support' });
    await expect(api.deleteRole('helpdesk')).resolves.toBe(true);

    expect(receivedRequest(adminRoles.createRole)).toMatchObject({
      name: 'helpdesk',
      displayName: 'Helpdesk',
      description: 'Support queue',
      pingable: true
    });
    expect(receivedRequest(adminRoles.updateRole)).toMatchObject({
      name: 'helpdesk',
      displayName: 'Support',
      description: 'Support queue',
      pingable: false,
      updateMask: { paths: ['display_name', 'description', 'pingable'] }
    });
    expect(receivedRequest(adminRoles.deleteRole)).toMatchObject({ name: 'helpdesk' });
  });

  it('reads a public role and treats a missing role as null', async () => {
    roles.getRole.mockReturnValueOnce({
      role: { name: 'moderator', displayName: 'Moderator', description: '', position: 1 }
    });
    await expect(roleAPI().getPublicRole('moderator')).resolves.toMatchObject({
      name: 'moderator',
      displayName: 'Moderator'
    });

    roles.getRole.mockReturnValueOnce({});
    await expect(roleAPI().getPublicRole('empty')).resolves.toBeNull();

    roles.getRole.mockImplementationOnce(() => {
      throw new ConnectError('missing', Code.NotFound);
    });
    await expect(roleAPI().getPublicRole('missing')).resolves.toBeNull();

    roles.getRole.mockImplementationOnce(() => {
      throw new ConnectError('denied', Code.PermissionDenied);
    });
    await expect(roleAPI().getPublicRole('secret')).rejects.toMatchObject({
      code: Code.PermissionDenied
    });
  });
});
