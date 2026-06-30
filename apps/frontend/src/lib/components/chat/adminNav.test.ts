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
  it('shows Members for direct user-permission managers', () => {
    const items = getAdminNavItems({
      serverSegment: 'local',
      chrome: chrome({ canViewAdmin: true, canManageUserPermissions: true }),
      server: server()
    });

    expect(items.some((item) => item.label === 'Members')).toBe(true);
  });

  it('hides Members without a member-management capability', () => {
    const items = getAdminNavItems({
      serverSegment: 'local',
      chrome: chrome({ canViewAdmin: true }),
      server: server()
    });

    expect(items.some((item) => item.label === 'Members')).toBe(false);
  });
});
