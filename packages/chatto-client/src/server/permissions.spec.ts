import { describe, expect, it } from 'vitest';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import type { ViewerCapabilities, ViewerState } from '../api/viewer.js';
import { NO_SERVER_PERMISSIONS, serverPermissionsFromViewer } from './permissions.js';

const noCapabilities: ViewerCapabilities = {
  canViewAdmin: false,
  canStartDMs: false,
  canAdminViewUsers: false,
  canAdminManageAccounts: false,
  canAssignRoles: false,
  canAdminViewRoles: false,
  canAdminManageRoles: false,
  canAdminViewSystem: false,
  canAdminViewAudit: false,
  canManageUserPermissions: false,
  canManageInvites: false
};

function viewer(
  capabilities: Partial<ViewerCapabilities>,
  viewerPermissions: Record<string, boolean>
): ViewerState {
  return {
    ...noCapabilities,
    ...capabilities,
    user: {
      id: 'U1',
      login: 'viewer',
      displayName: 'Viewer',
      presenceStatus: PresenceStatus.ONLINE,
      hasVerifiedEmail: true,
      hasPassword: true,
      viewerCanDeleteAccount: true
    },
    viewerPermissions,
    viewerHasUnreadRooms: false,
    privilegedMode: { available: false, active: false, expiresAt: null }
  };
}

describe('serverPermissionsFromViewer', () => {
  it('grants nothing before the viewer projection loads', () => {
    expect(NO_SERVER_PERMISSIONS.loaded).toBe(false);
    expect(Object.isFrozen(NO_SERVER_PERMISSIONS)).toBe(true);
    for (const [key, value] of Object.entries(NO_SERVER_PERMISSIONS)) {
      expect(value, key).toBe(false);
    }
  });

  it('passes every viewer capability through unchanged', () => {
    for (const capability of Object.keys(noCapabilities) as (keyof ViewerCapabilities)[]) {
      const permissions = serverPermissionsFromViewer(viewer({ [capability]: true }, {}));
      expect(permissions).toEqual({
        ...NO_SERVER_PERMISSIONS,
        loaded: true,
        [capability]: true
      });
    }
  });

  it.each([
    ['server.manage', 'canManageServer'],
    ['server.manage-neighbors', 'canManageNeighbors'],
    ['room.manage', 'canManageRooms'],
    ['room.remove-member', 'canModerateRooms'],
    ['bot.create', 'canCreateBots'],
    ['bot.manage', 'canManageBots']
  ] as const)('maps effective %s to %s', (permission, flag) => {
    expect(serverPermissionsFromViewer(viewer({}, { [permission]: true }))).toEqual({
      ...NO_SERVER_PERMISSIONS,
      loaded: true,
      [flag]: true
    });
    expect(serverPermissionsFromViewer(viewer({}, { [permission]: false }))[flag]).toBe(false);
  });

  it('ignores unrelated or missing effective permissions', () => {
    expect(
      serverPermissionsFromViewer(viewer({}, { 'message.post': true, 'room.create': true }))
    ).toEqual({ ...NO_SERVER_PERMISSIONS, loaded: true });
  });
});
