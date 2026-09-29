import { Code } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AdminPermissionService } from '@chatto/api-types/admin/v1/permissions_connect';
import { createPermissionAPI } from '../permissions.js';
import { PermissionDecision, PermissionScopeKind } from '@chatto/api-types/admin/v1/permissions_pb';
import { fakeServer, mockService, receivedRequest } from '../../testing/fakeServer.js';

const mocks = mockService(AdminPermissionService);

function permissionAPI() {
  return createPermissionAPI(fakeServer((router) => router.service(AdminPermissionService, mocks)));
}

describe('createPermissionAPI', () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it('loads the tier matrix', async () => {
    mocks.getRolePermissionTierMatrix.mockReturnValue({
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
    const api = permissionAPI();

    const result = await api.getRolePermissionTierMatrix({ roomId: 'R1', groupId: null });

    expect(receivedRequest(mocks.getRolePermissionTierMatrix)).toMatchObject({
      scope: { kind: PermissionScopeKind.ROOM, id: 'R1' }
    });
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
    mocks.getRolePermissionTierMatrix.mockReturnValue({
      matrix: {
        applicablePermissions: ['message.post'],
        roles: [{ override: { permissions: [], permissionDenials: [] } }]
      }
    });
    const api = permissionAPI();

    await expect(api.getRolePermissionTierMatrix({ roomId: 'R1' })).rejects.toThrow(
      'permission tier role response did not include role metadata'
    );
  });

  it('maps role matrix enum values to frontend strings', async () => {
    mocks.getRolePermissionMatrix.mockReturnValue({
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
    const api = permissionAPI();

    const result = await api.getRolePermissionMatrix('admin');

    expect(receivedRequest(mocks.getRolePermissionMatrix)).toMatchObject({
      roleName: 'admin',
      includeDirectMessageScope: true
    });
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
    mocks.getRolePermissionMatrix.mockReturnValue({
      page: { totalCount: 2n, hasMore: false },
      matrix: {
        roleName: 'admin',
        applicablePermissions: [],
        scopes: [
          { id: 'future', label: 'Future', kind: 99 as PermissionScopeKind, parentGroupId: '' }
        ],
        cells: []
      }
    });
    const api = permissionAPI();

    await expect(api.getRolePermissionMatrix('admin')).rejects.toThrow(
      'unsupported permission matrix scope kind: 99'
    );
  });

  it('loads role permission decisions as scoped entries', async () => {
    mocks.listRolePermissionDecisions.mockReturnValue({
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
    const api = permissionAPI();

    const result = await api.listRolePermissionDecisions('admin');

    expect(receivedRequest(mocks.listRolePermissionDecisions)).toMatchObject({
      roleName: 'admin',
      includeDirectMessageScope: true
    });
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
    mocks.getUserPermissionMatrix.mockReturnValue({
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
    const api = permissionAPI();

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
    mocks.listUserPermissionDecisions.mockReturnValue({
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
    const api = permissionAPI();

    const result = await api.listUserPermissionDecisions('U1');

    expect(receivedRequest(mocks.listUserPermissionDecisions)).toMatchObject({
      userId: 'U1',
      includeDirectMessageScope: true
    });
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
    mocks.setRolePermission.mockReturnValue({
      decision: {
        permission: 'message.post',
        scope: { kind: PermissionScopeKind.ROOM, id: 'R1' },
        decision: PermissionDecision.DENY
      }
    });
    mocks.setUserPermission.mockReturnValue({
      decision: {
        permission: 'room.create',
        scope: { kind: PermissionScopeKind.GROUP, id: 'G1' },
        decision: PermissionDecision.NONE
      }
    });
    const api = permissionAPI();

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

    expect(receivedRequest(mocks.setRolePermission)).toMatchObject({
      roleName: 'admin',
      permission: 'message.post',
      decision: PermissionDecision.DENY,
      scope: { kind: PermissionScopeKind.ROOM, id: 'R1' }
    });
    expect(receivedRequest(mocks.setUserPermission)).toMatchObject({
      userId: 'U1',
      permission: 'room.create',
      decision: PermissionDecision.NONE,
      scope: { kind: PermissionScopeKind.GROUP, id: 'G1' }
    });
  });

  it('sends scope pages and cancellation for JSON-compatible decision reads', async () => {
    mocks.listRolePermissionDecisions.mockReturnValue({
      roleName: 'moderator',
      decisions: [],
      scopes: [{ kind: PermissionScopeKind.ROOM, id: 'room-1' }],
      page: { totalCount: 1n, hasMore: false }
    });
    const api = permissionAPI();
    const result = await api.listRolePermissionDecisions('moderator', {
      page: { limit: 10, offset: 0 },
      scope: { tier: 'room', roomId: 'room-1' }
    });
    expect(receivedRequest(mocks.listRolePermissionDecisions)).toMatchObject({
      roleName: 'moderator',
      includeDirectMessageScope: true,
      page: { limit: 10, offset: 0 },
      scope: { kind: PermissionScopeKind.ROOM, id: 'room-1' }
    });
    expect(result.scopes).toEqual([{ tier: 'room', roomId: 'room-1' }]);
    expect(result.page).toEqual({ totalCount: 1, hasMore: false });

    await expect(
      api.listRolePermissionDecisions('moderator', { signal: AbortSignal.abort() })
    ).rejects.toMatchObject({ code: Code.Canceled });
  });

  it('maps DM and server scopes and allow decisions in both directions', async () => {
    mocks.setRolePermission.mockImplementation((request) => ({
      decision: {
        permission: request.permission,
        scope: request.scope,
        decision: PermissionDecision.ALLOW
      }
    }));
    const api = permissionAPI();

    for (const scope of [{ tier: 'dm' }, { tier: 'server' }] as const) {
      await expect(
        api.setRolePermission({
          roleName: 'everyone',
          scope,
          permission: 'message.post',
          state: 'allow'
        })
      ).resolves.toEqual({ permission: 'message.post', scope, decision: 'ALLOW' });
    }
    expect(receivedRequest(mocks.setRolePermission, 0)).toMatchObject({
      decision: PermissionDecision.ALLOW,
      scope: { kind: PermissionScopeKind.DM, id: '' }
    });
    expect(receivedRequest(mocks.setRolePermission, 1)).toMatchObject({
      scope: { kind: PermissionScopeKind.SERVER, id: '' }
    });
  });

  it('rejects incomplete permission answers', async () => {
    mocks.setUserPermission.mockReturnValue({});
    mocks.listUserPermissionDecisions.mockReturnValueOnce({ userId: 'U1', decisions: [] });
    mocks.listUserPermissionDecisions.mockReturnValueOnce({
      userId: 'U1',
      decisions: [],
      scopes: [{ kind: PermissionScopeKind.UNSPECIFIED, id: '' }],
      page: { totalCount: 0n, hasMore: false }
    });
    const api = permissionAPI();

    await expect(
      api.setUserPermission({
        userId: 'U1',
        scope: { tier: 'server' },
        permission: 'room.create',
        state: 'deny'
      })
    ).rejects.toThrow('did not include a decision');
    await expect(api.listUserPermissionDecisions('U1')).rejects.toThrow(
      'did not include scope page metadata'
    );
    await expect(api.listUserPermissionDecisions('U1')).rejects.toThrow(
      'unsupported permission scope kind'
    );
  });

  it('returns null for absent matrices and scopes a tier matrix to a group', async () => {
    mocks.getRolePermissionTierMatrix.mockReturnValue({});
    mocks.getRolePermissionMatrix.mockReturnValue({});
    mocks.getUserPermissionMatrix.mockReturnValue({});
    const api = permissionAPI();

    await expect(api.getRolePermissionTierMatrix({ groupId: 'G1' })).resolves.toBeNull();
    expect(receivedRequest(mocks.getRolePermissionTierMatrix)).toMatchObject({
      scope: { kind: PermissionScopeKind.GROUP, id: 'G1' }
    });
    await expect(api.getRolePermissionTierMatrix({})).resolves.toBeNull();
    expect(receivedRequest(mocks.getRolePermissionTierMatrix, 1)).toMatchObject({
      scope: { kind: PermissionScopeKind.SERVER, id: '' }
    });
    await expect(api.getRolePermissionMatrix('everyone')).resolves.toBeNull();
    await expect(api.getUserPermissionMatrix('U1', { scope: { tier: 'dm' } })).resolves.toBeNull();
  });
});
