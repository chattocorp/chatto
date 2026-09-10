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
            role: {
              name: 'moderator',
              displayName: 'Moderator',
              description: '',
              isSystem: true,
              position: 100,
              pingable: true
            },
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
          pingable: true,
          override: { permissions: ['message.post'], permissionDenials: [] },
          inheritedAllows: [],
          inheritedDenials: ['message.react']
        }
      ]
    });
  });

  it('rejects tier matrix roles without shared role metadata', async () => {
    mocks.getRolePermissionTierMatrix.mockResolvedValue({
      matrix: {
        applicablePermissions: ['message.post'],
        roles: [{ override: { permissions: [], permissionDenials: [] } }]
      }
    });
    const api = createPermissionAPI({ baseUrl: '/api/connect', bearerToken: 'token' });

    await expect(api.getRolePermissionTierMatrix({ roomId: 'R1' })).rejects.toThrow(
      'permission tier role response did not include role metadata'
    );
  });

  it('maps role matrix enum values to frontend strings', async () => {
    mocks.getRolePermissionMatrix.mockResolvedValue({
      page: { totalCount: 2n, hasMore: false },
      matrix: {
        roleName: 'admin',
        applicablePermissions: ['message.post'],
        scopes: [
          {
            id: 'server',
            label: 'Server',
            kind: PermissionScopeKind.SERVER,
            parentGroupId: ''
          },
          {
            id: 'dm',
            label: 'Direct messages',
            kind: PermissionScopeKind.DM,
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
      { roleName: 'admin', includeDirectMessageScope: true },
      { headers: undefined }
    );
    expect(result).toEqual({
      page: { totalCount: 2, hasMore: false },
      roleName: 'admin',
      applicablePermissions: ['message.post'],
      scopes: [
        { id: 'server', label: 'Server', kind: 'SERVER', parentGroupId: '' },
        { id: 'dm', label: 'Direct messages', kind: 'DM', parentGroupId: '' }
      ],
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

  it('rejects unknown permission matrix scope kinds', async () => {
    mocks.getRolePermissionMatrix.mockResolvedValue({
      page: { totalCount: 2n, hasMore: false },
      matrix: {
        roleName: 'admin',
        applicablePermissions: [],
        scopes: [{ id: 'future', label: 'Future', kind: 99, parentGroupId: '' }],
        cells: []
      }
    });
    const api = createPermissionAPI({ baseUrl: '/api/connect', bearerToken: null });

    await expect(api.getRolePermissionMatrix('admin')).rejects.toThrow(
      'unsupported permission matrix scope kind: 99'
    );
  });

  it('loads role permission decisions as scoped entries', async () => {
    mocks.listRolePermissionDecisions.mockResolvedValue({
      page: { totalCount: 2n, hasMore: false },
      roleName: 'admin',
      scopes: [],
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
      { roleName: 'admin', includeDirectMessageScope: true },
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(result).toEqual({
      page: { totalCount: 2, hasMore: false },
      roleName: 'admin',
      scopes: [],
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
      page: { totalCount: 2n, hasMore: false },
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
            effective: PermissionDecision.NONE,
            allowPermitted: false
          }
        ]
      }
    });
    const api = createPermissionAPI({ baseUrl: '/api/connect', bearerToken: null });

    const result = await api.getUserPermissionMatrix('U1');

    expect(result).toEqual({
      page: { totalCount: 2, hasMore: false },
      userId: 'U1',
      applicablePermissions: ['room.create'],
      scopes: [{ id: 'group:G1', label: 'Lobby', kind: 'GROUP', parentGroupId: '' }],
      cells: [
        {
          permission: 'room.create',
          scopeId: 'group:G1',
          override: 'NONE',
          effective: 'NONE',
          allowPermitted: false
        }
      ]
    });
  });

  it('loads user permission decisions as scoped entries', async () => {
    mocks.listUserPermissionDecisions.mockResolvedValue({
      page: { totalCount: 2n, hasMore: false },
      userId: 'U1',
      scopes: [],
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
      { userId: 'U1', includeDirectMessageScope: true },
      { headers: undefined }
    );
    expect(result).toEqual({
      page: { totalCount: 2, hasMore: false },
      userId: 'U1',
      scopes: [],
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

it('sends scope pages and cancellation for JSON-compatible decision reads', async () => {
  mocks.listRolePermissionDecisions.mockResolvedValue({
    roleName: 'moderator', decisions: [],
    scopes: [{ kind: PermissionScopeKind.ROOM, id: 'room-1' }],
    page: { totalCount: 1n, hasMore: false }
  });
  const api = createPermissionAPI({ baseUrl: '/api/connect', bearerToken: 'token' });
  const signal = new AbortController().signal;
  const result = await api.listRolePermissionDecisions('moderator', {
    signal, page: { limit: 10, offset: 0 }, scope: { tier: 'room', roomId: 'room-1' }
  });
  expect(mocks.listRolePermissionDecisions).toHaveBeenCalledWith({
    roleName: 'moderator', includeDirectMessageScope: true,
    page: { limit: 10, offset: 0 }, scope: { kind: PermissionScopeKind.ROOM, id: 'room-1' }
  }, { headers: { Authorization: 'Bearer token' }, signal });
  expect(result.scopes).toEqual([{ tier: 'room', roomId: 'room-1' }]);
  expect(result.page).toEqual({ totalCount: 1, hasMore: false });
});

});
