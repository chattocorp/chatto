import { describe, expect, it } from 'vitest';
import { NO_SERVER_PERMISSIONS, type ServerPermissions } from '@chatto/client/server/permissions';
import { getAdminNavItems } from './adminNav';

function permissions(overrides: Partial<ServerPermissions> = {}): ServerPermissions {
  return { ...NO_SERVER_PERMISSIONS, loaded: true, ...overrides };
}

describe('getAdminNavItems', () => {
  it('lists nothing until the viewer permissions load', () => {
    expect(
      getAdminNavItems({ serverSegment: 'local', permissions: NO_SERVER_PERMISSIONS })
    ).toEqual([]);
  });

  it('shows Bots as a server-management surface for every signed-in human', () => {
    const items = getAdminNavItems({
      serverSegment: 'local',
      permissions: permissions()
    });

    expect(items.find((item) => item.label === 'Bots')?.href).toBe(
      '/chat/local/manage/server/bots'
    );
  });

  it('hides Moderation for admin entitlement and unrelated effective permissions', () => {
    const items = getAdminNavItems({
      serverSegment: 'local',
      permissions: permissions({
        canViewAdmin: true,
        canManageServer: true,
        canAdminViewUsers: true
      })
    });

    expect(items.some((item) => item.label === 'Moderation')).toBe(false);
  });

  it('shows Moderation for effective server-wide ban permission', () => {
    const items = getAdminNavItems({
      serverSegment: 'local',
      permissions: permissions({ canModerateRooms: true })
    });

    expect(items.find((item) => item.label === 'Moderation')?.href).toBe(
      '/chat/local/manage/server/moderation'
    );
  });

  it('shows Members for admin user viewers', () => {
    const items = getAdminNavItems({
      serverSegment: 'local',
      permissions: permissions({ canViewAdmin: true, canAdminViewUsers: true })
    });

    expect(items.some((item) => item.label === 'Members')).toBe(true);
  });

  it('hides Members for role assignment without admin user view', () => {
    const items = getAdminNavItems({
      serverSegment: 'local',
      permissions: permissions({ canViewAdmin: true, canAssignRoles: true })
    });

    expect(items.some((item) => item.label === 'Members')).toBe(false);
  });

  it('hides Permissions without role management', () => {
    const items = getAdminNavItems({
      serverSegment: 'local',
      permissions: permissions({
        canViewAdmin: true,
        canAssignRoles: true,
        canAdminViewRoles: true
      })
    });

    expect(items.some((item) => item.label === 'Permissions')).toBe(false);
  });

  it('shows Permissions for role managers', () => {
    const items = getAdminNavItems({
      serverSegment: 'local',
      permissions: permissions({ canViewAdmin: true, canAdminManageRoles: true })
    });

    expect(items.some((item) => item.label === 'Permissions')).toBe(true);
  });

  it('shows Invite links only for invitation managers', () => {
    const hidden = getAdminNavItems({
      serverSegment: 'local',
      permissions: permissions({ canViewAdmin: true })
    });
    const visible = getAdminNavItems({
      serverSegment: 'local',
      permissions: permissions({ canViewAdmin: true, canManageInvites: true })
    });

    expect(hidden.some((item) => item.label === 'Invite links')).toBe(false);
    expect(visible.find((item) => item.label === 'Invite links')?.href).toBe(
      '/chat/local/manage/server/invite-links'
    );
  });

  it('shows Neighbors only for Neighbor managers', () => {
    const hidden = getAdminNavItems({
      serverSegment: 'local',
      permissions: permissions()
    });
    const visible = getAdminNavItems({
      serverSegment: 'local',
      permissions: permissions({ canManageNeighbors: true })
    });

    expect(hidden.some((item) => item.label === 'Neighbors')).toBe(false);
    expect(visible.find((item) => item.label === 'Neighbors')?.href).toBe(
      '/chat/local/manage/server/neighbors'
    );
  });

  it('keeps server pages beneath manage/server and rooms as sibling resources', () => {
    const items = getAdminNavItems({
      serverSegment: 'local',
      permissions: permissions({ canViewAdmin: true, canManageServer: true, canManageRooms: true })
    });

    expect(items.find((item) => item.label === 'General')?.href).toBe(
      '/chat/local/manage/server/general'
    );
    expect(items.find((item) => item.label === 'Rooms')?.href).toBe('/chat/local/manage/rooms');
  });
});
