import { describe, expect, it } from 'vitest';
import {
  getAdminNavItems,
  type AdminNavChromePermissions,
  type AdminNavServerPermissions
} from './adminNav';

function chrome(overrides: Partial<AdminNavChromePermissions> = {}): AdminNavChromePermissions {
  return {
    canViewAdmin: false,
    canManage: false,
    canManageRooms: false,
    canManageRoles: false,
    canAssignRoles: false,
    canManageUserAccounts: false,
    canManageUserPermissions: false,
    canManageBots: false,
    supportsBots: false,
    ...overrides
  };
}

function server(overrides: Partial<AdminNavServerPermissions> = {}): AdminNavServerPermissions {
  return {
    canViewAdmin: false,
    canAdminViewUsers: false,
    canAdminViewRoles: false,
    canAdminViewAudit: false,
    canAdminViewSystem: false,
    ...overrides
  };
}

describe('getAdminNavItems', () => {
  it('shows Members for admin user viewers', () => {
    const items = getAdminNavItems({
      serverSegment: 'local',
      chrome: chrome({ canViewAdmin: true }),
      server: server({ canAdminViewUsers: true })
    });

    expect(items.some((item) => item.label === 'Members')).toBe(true);
  });

  it('hides Members for role assignment without admin user view', () => {
    const items = getAdminNavItems({
      serverSegment: 'local',
      chrome: chrome({ canViewAdmin: true, canAssignRoles: true }),
      server: server()
    });

    expect(items.some((item) => item.label === 'Members')).toBe(false);
  });

  it('hides Permissions without role management', () => {
    const items = getAdminNavItems({
      serverSegment: 'local',
      chrome: chrome({ canViewAdmin: true, canAssignRoles: true }),
      server: server({ canAdminViewRoles: true })
    });

    expect(items.some((item) => item.label === 'Permissions')).toBe(false);
  });

  it('shows Permissions for role managers', () => {
    const items = getAdminNavItems({
      serverSegment: 'local',
      chrome: chrome({ canViewAdmin: true, canManageRoles: true }),
      server: server()
    });

    expect(items.some((item) => item.label === 'Permissions')).toBe(true);
  });

  it('shows Bots only when management permission and protocol support are both present', () => {
    const visible = getAdminNavItems({
      serverSegment: 'local',
      chrome: chrome({ canViewAdmin: true, canManageBots: true, supportsBots: true }),
      server: server()
    });
    expect(visible.some((item) => item.label === 'Bots')).toBe(true);

    const unsupported = getAdminNavItems({
      serverSegment: 'local',
      chrome: chrome({ canViewAdmin: true, canManageBots: true, supportsBots: false }),
      server: server()
    });
    expect(unsupported.some((item) => item.label === 'Bots')).toBe(false);
  });

  it('keeps server pages beneath manage/server and rooms as sibling resources', () => {
    const items = getAdminNavItems({
      serverSegment: 'local',
      chrome: chrome({ canViewAdmin: true, canManage: true, canManageRooms: true }),
      server: server()
    });

    expect(items.find((item) => item.label === 'General')?.href).toBe(
      '/chat/local/manage/server/general'
    );
    expect(items.find((item) => item.label === 'Rooms')?.href).toBe('/chat/local/manage/rooms');
  });
});
