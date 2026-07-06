import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createRoleAPI } from '$lib/api-client/roles';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  listRoles: vi.fn(),
  getPublicRole: vi.fn(),
  batchGetRoles: vi.fn(),
  listAdminRoles: vi.fn(),
  getRole: vi.fn(),
  createRole: vi.fn(),
  updateRole: vi.fn(),
  deleteRole: vi.fn()
}));

vi.mock('@connectrpc/connect', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@connectrpc/connect')>();
  return {
    ...actual,
    createClient: mocks.createClient
  };
});

vi.mock('@connectrpc/connect-web', () => ({
  createConnectTransport: mocks.createConnectTransport
}));

describe('createRoleAPI', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.listRoles.mockReset();
    mocks.getPublicRole.mockReset();
    mocks.batchGetRoles.mockReset();
    mocks.listAdminRoles.mockReset();
    mocks.getRole.mockReset();
    mocks.createRole.mockReset();
    mocks.updateRole.mockReset();
    mocks.deleteRole.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockImplementation((service) => {
      if (service.typeName === 'chatto.api.v1.RoleService') {
        return {
          listRoles: mocks.listRoles,
          getRole: mocks.getPublicRole,
          batchGetRoles: mocks.batchGetRoles
        };
      }
      return {
        listRoles: mocks.listAdminRoles,
        getRole: mocks.getRole,
        createRole: mocks.createRole,
        updateRole: mocks.updateRole,
        deleteRole: mocks.deleteRole
      };
    });
  });

  it('lists public roles', async () => {
    mocks.listRoles.mockResolvedValue({
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
    const api = createRoleAPI({ baseUrl: '/api/connect', bearerToken: 'token' });

    const result = await api.listRoles();

    expect(mocks.listRoles).toHaveBeenCalledWith(
      {},
      { headers: { Authorization: 'Bearer token' } }
    );
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
    mocks.getPublicRole.mockResolvedValue({ role });
    mocks.batchGetRoles.mockResolvedValue({ roles: [role] });
    const api = createRoleAPI({ baseUrl: '/api/connect', bearerToken: 'token' });

    await expect(api.getPublicRole('moderator')).resolves.toMatchObject({
      name: 'moderator',
      permissions: []
    });
    await expect(api.batchGetPublicRoles(['moderator', 'missing'])).resolves.toMatchObject([
      { name: 'moderator' }
    ]);

    expect(mocks.getPublicRole).toHaveBeenCalledWith(
      { name: 'moderator' },
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(mocks.batchGetRoles).toHaveBeenCalledWith(
      { names: ['moderator', 'missing'] },
      { headers: { Authorization: 'Bearer token' } }
    );
  });

  it('lists admin roles with viewer capabilities', async () => {
    mocks.listAdminRoles.mockResolvedValue({
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
    const api = createRoleAPI({ baseUrl: '/api/connect', bearerToken: 'token' });

    const result = await api.listAdminRoles();

    expect(mocks.listAdminRoles).toHaveBeenCalledWith(
      {},
      { headers: { Authorization: 'Bearer token' } }
    );
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

  it('gets a role with users and no auth headers when no token is available', async () => {
    mocks.getRole.mockResolvedValue({
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
      users: [{ id: 'user-1', login: 'alice', displayName: 'Alice' }],
      viewerCanManageRoles: true,
      viewerCanAssignRoles: true
    });
    const api = createRoleAPI({ baseUrl: '/api/connect', bearerToken: null });

    const result = await api.getRole('helpdesk');

    expect(mocks.getRole).toHaveBeenCalledWith({ name: 'helpdesk' }, { headers: undefined });
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
      users: [{ id: 'user-1', login: 'alice', displayName: 'Alice' }],
      viewerCanManageRoles: true,
      viewerCanAssignRoles: true
    });
  });

  it('creates updates and deletes roles with auth headers', async () => {
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
    mocks.createRole.mockResolvedValue({ role: apiRole });
    mocks.updateRole.mockResolvedValue({
      role: { ...apiRole, role: { ...apiRole.role, displayName: 'Support' } }
    });
    mocks.deleteRole.mockResolvedValue({ deleted: true });
    const api = createRoleAPI({ baseUrl: '/api/connect', bearerToken: 'token' });

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

    expect(mocks.createRole).toHaveBeenCalledWith(role, {
      headers: { Authorization: 'Bearer token' }
    });
    expect(mocks.updateRole).toHaveBeenCalledWith(
      {
        name: 'helpdesk',
        displayName: 'Support',
        description: 'Support queue',
        pingable: false
      },
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(mocks.deleteRole).toHaveBeenCalledWith(
      { name: 'helpdesk' },
      { headers: { Authorization: 'Bearer token' } }
    );
  });
});
