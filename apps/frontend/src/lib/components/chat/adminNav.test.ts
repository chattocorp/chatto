import { describe, expect, it } from 'vitest';
import { getAdminNavItems, type AdminNavChromePermissions, type AdminNavServerPermissions } from './adminNav';

function chrome(overrides: Partial<AdminNavChromePermissions> = {}): AdminNavChromePermissions {
  return {
    canViewAdmin: false,
    canManage: false,
    canManageRooms: false,
    canManageRoles: false,
    canAssignRoles: false,
    canManageUserAccounts: false,
    canManageUserPermissions: false,
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
});
