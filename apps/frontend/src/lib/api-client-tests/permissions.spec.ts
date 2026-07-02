import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createPermissionAPI } from '$lib/api-client/permissions';
import { PermissionDecision, PermissionScopeKind } from '@chatto/api-types/admin/v1/permissions_pb';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  getRolePermissionTierMatrix: vi.fn(),
  getRolePermissionMatrix: vi.fn(),
  listRolePermissionDecisions: vi.fn(),
  getUserPermissionMatrix: vi.fn(),
  listUserPermissionDecisions: vi.fn(),
  setRolePermission: vi.fn(),
  setUserPermission: vi.fn()
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

describe('createPermissionAPI', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.getRolePermissionTierMatrix.mockReset();
    mocks.getRolePermissionMatrix.mockReset();
    mocks.listRolePermissionDecisions.mockReset();
    mocks.getUserPermissionMatrix.mockReset();
    mocks.listUserPermissionDecisions.mockReset();
    mocks.setRolePermission.mockReset();
    mocks.setUserPermission.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      getRolePermissionTierMatrix: mocks.getRolePermissionTierMatrix,
      getRolePermissionMatrix: mocks.getRolePermissionMatrix,
      listRolePermissionDecisions: mocks.listRolePermissionDecisions,
      getUserPermissionMatrix: mocks.getUserPermissionMatrix,
      listUserPermissionDecisions: mocks.listUserPermissionDecisions,
      setRolePermission: mocks.setRolePermission,
      setUserPermission: mocks.setUserPermission
    });
  });

  it('loads the tier matrix with auth headers', async () => {
    mocks.getRolePermissionTierMatrix.mockResolvedValue({
      matrix: {
        applicablePermissions: ['message.post'],
        roles: [
          {
            roleName: 'moderator',
            displayName: 'Moderator',
            description: '',
            isSystem: true,
            position: 100,
            override: { permissions: ['message.post'], permissionDenials: [] },
            inheritedAllows: [],
            inheritedDenials: ['message.react']
          }
        ]
      }
    });
    const api = createPermissionAPI({ baseUrl: '/api/connect', bearerToken: 'token' });

    const result = await api.getRolePermissionTierMatrix({ roomId: 'R1', groupId: null });

    expect(mocks.getRolePermissionTierMatrix).toHaveBeenCalledWith(
      { scope: { kind: PermissionScopeKind.ROOM, id: 'R1' } },
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(result).toEqual({
      applicablePermissions: ['message.post'],
      roles: [
        {
          roleName: 'moderator',
          displayName: 'Moderator',
          description: '',
          isSystem: true,
          position: 100,
          override: { permissions: ['message.post'], permissionDenials: [] },
          inheritedAllows: [],
          inheritedDenials: ['message.react']
        }
      ]
    });
  });

  it('maps role matrix enum values to frontend strings', async () => {
    mocks.getRolePermissionMatrix.mockResolvedValue({
      matrix: {
        roleName: 'admin',
        applicablePermissions: ['message.post'],
        scopes: [
          {
            id: 'server',
            label: 'Server',
            kind: PermissionScopeKind.SERVER,
            parentGroupId: ''
          }
        ],
        cells: [
          {
            permission: 'message.post',
            scopeId: 'server',
            override: PermissionDecision.ALLOW,
            effective: PermissionDecision.DENY
          }
        ]
      }
    });
    const api = createPermissionAPI({ baseUrl: '/api/connect', bearerToken: null });

    const result = await api.getRolePermissionMatrix('admin');

    expect(mocks.getRolePermissionMatrix).toHaveBeenCalledWith(
      { roleName: 'admin' },
      { headers: undefined }
    );
    expect(result).toEqual({
      roleName: 'admin',
      applicablePermissions: ['message.post'],
      scopes: [{ id: 'server', label: 'Server', kind: 'SERVER', parentGroupId: '' }],
      cells: [
        {
          permission: 'message.post',
          scopeId: 'server',
          override: 'ALLOW',
          effective: 'DENY'
        }
      ]
    });
  });

  it('loads role permission decisions as scoped entries', async () => {
    mocks.listRolePermissionDecisions.mockResolvedValue({
      roleName: 'admin',
      decisions: [
        {
          permission: 'message.post',
          scope: { kind: PermissionScopeKind.SERVER, id: '' },
          override: PermissionDecision.ALLOW,
          effective: PermissionDecision.ALLOW
        },
        {
          permission: 'message.react',
          scope: { kind: PermissionScopeKind.ROOM, id: 'R1' },
          override: PermissionDecision.NONE,
          effective: PermissionDecision.DENY
        }
      ]
    });
    const api = createPermissionAPI({ baseUrl: '/api/connect', bearerToken: 'token' });

    const result = await api.listRolePermissionDecisions('admin');

    expect(mocks.listRolePermissionDecisions).toHaveBeenCalledWith(
      { roleName: 'admin' },
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(result).toEqual({
      roleName: 'admin',
      decisions: [
        {
          permission: 'message.post',
          scope: { tier: 'server' },
          override: 'ALLOW',
          effective: 'ALLOW'
        },
        {
          permission: 'message.react',
          scope: { tier: 'room', roomId: 'R1' },
          override: 'NONE',
          effective: 'DENY'
        }
      ]
    });
  });

  it('loads user matrices and maps missing decisions to NONE', async () => {
    mocks.getUserPermissionMatrix.mockResolvedValue({
      matrix: {
        userId: 'U1',
        applicablePermissions: ['room.create'],
        scopes: [
          { id: 'group:G1', label: 'Lobby', kind: PermissionScopeKind.GROUP, parentGroupId: '' }
        ],
        cells: [
          {
            permission: 'room.create',
            scopeId: 'group:G1',
            override: PermissionDecision.NONE,
            effective: PermissionDecision.NONE
          }
        ]
      }
    });
    const api = createPermissionAPI({ baseUrl: '/api/connect', bearerToken: null });

    const result = await api.getUserPermissionMatrix('U1');

    expect(result).toEqual({
      userId: 'U1',
      applicablePermissions: ['room.create'],
      scopes: [{ id: 'group:G1', label: 'Lobby', kind: 'GROUP', parentGroupId: '' }],
      cells: [
        {
          permission: 'room.create',
          scopeId: 'group:G1',
          override: 'NONE',
          effective: 'NONE'
        }
      ]
    });
  });

  it('loads user permission decisions as scoped entries', async () => {
    mocks.listUserPermissionDecisions.mockResolvedValue({
      userId: 'U1',
      decisions: [
        {
          permission: 'room.create',
          scope: { kind: PermissionScopeKind.GROUP, id: 'G1' },
          override: PermissionDecision.DENY,
          effective: PermissionDecision.DENY
        }
      ]
    });
    const api = createPermissionAPI({ baseUrl: '/api/connect', bearerToken: null });

    const result = await api.listUserPermissionDecisions('U1');

    expect(mocks.listUserPermissionDecisions).toHaveBeenCalledWith(
      { userId: 'U1' },
      { headers: undefined }
    );
    expect(result).toEqual({
      userId: 'U1',
      decisions: [
        {
          permission: 'room.create',
          scope: { tier: 'group', groupId: 'G1' },
          override: 'DENY',
          effective: 'DENY'
        }
      ]
    });
  });

  it('sets role and user permissions with protobuf enums', async () => {
    mocks.setRolePermission.mockResolvedValue({
      decision: {
        permission: 'message.post',
        scope: { kind: PermissionScopeKind.ROOM, id: 'R1' },
        decision: PermissionDecision.DENY
      }
    });
    mocks.setUserPermission.mockResolvedValue({
      decision: {
        permission: 'room.create',
        scope: { kind: PermissionScopeKind.GROUP, id: 'G1' },
        decision: PermissionDecision.NONE
      }
    });
    const api = createPermissionAPI({ baseUrl: '/api/connect', bearerToken: 'token' });

    await expect(
      api.setRolePermission({
        roleName: 'admin',
        scope: { tier: 'room', roomId: 'R1' },
        permission: 'message.post',
        state: 'deny'
      })
    ).resolves.toEqual({
      permission: 'message.post',
      scope: { tier: 'room', roomId: 'R1' },
      decision: 'DENY'
    });
    await expect(
      api.setUserPermission({
        userId: 'U1',
        scope: { tier: 'group', groupId: 'G1' },
        permission: 'room.create',
        state: 'neutral'
      })
    ).resolves.toEqual({
      permission: 'room.create',
      scope: { tier: 'group', groupId: 'G1' },
      decision: 'NONE'
    });

    expect(mocks.setRolePermission).toHaveBeenCalledWith(
      {
        roleName: 'admin',
        permission: 'message.post',
        decision: PermissionDecision.DENY,
        scope: { kind: PermissionScopeKind.ROOM, id: 'R1' }
      },
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(mocks.setUserPermission).toHaveBeenCalledWith(
      {
        userId: 'U1',
        permission: 'room.create',
        decision: PermissionDecision.NONE,
        scope: { kind: PermissionScopeKind.GROUP, id: 'G1' }
      },
      { headers: { Authorization: 'Bearer token' } }
    );
  });
});
