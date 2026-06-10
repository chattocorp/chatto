import { resolve } from '$app/paths';

export type AdminNavSpacePermissions = {
  hasAnyAdminPermission: boolean;
  canManage: boolean;
  canManageRooms: boolean;
  canManageRoles: boolean;
  canAssignRoles: boolean;
};

export type AdminNavServerPermissions = {
  canViewAdmin: boolean;
  canAdminViewUsers: boolean;
  canAdminViewRoles: boolean;
  canAdminViewAudit: boolean;
  canAdminViewSystem: boolean;
};

export type AdminNavItem = {
  href: string;
  label: string;
  icon: string;
};

export function getAdminNavItems({
  serverSegment,
  space,
  server
}: {
  serverSegment: string;
  space: AdminNavSpacePermissions | null;
  server: AdminNavServerPermissions;
}): AdminNavItem[] {
  if (!space) return [];
  if (!space.hasAnyAdminPermission && !server.canViewAdmin) return [];

  const items: AdminNavItem[] = [];

  if (space.canManage) {
    items.push({
      href: resolve('/chat/[serverId]/server-admin/general', { serverId: serverSegment }),
      label: 'General',
      icon: 'iconify uil--setting'
    });
  }

  if (space.canAssignRoles || server.canAdminViewUsers) {
    items.push({
      href: resolve('/chat/[serverId]/server-admin/members', { serverId: serverSegment }),
      label: 'Members',
      icon: 'iconify uil--users-alt'
    });
  }

  if (space.canManageRooms) {
    items.push({
      href: resolve('/chat/[serverId]/server-admin/rooms', { serverId: serverSegment }),
      label: 'Rooms',
      icon: 'iconify uil--apps'
    });
  }

  if (space.hasAnyAdminPermission) {
    items.push({
      href: resolve('/chat/[serverId]/server-admin/moderation', { serverId: serverSegment }),
      label: 'Moderation',
      icon: 'iconify uil--ban'
    });
  }

  if (space.canManageRoles || server.canAdminViewRoles) {
    items.push({
      href: resolve('/chat/[serverId]/server-admin/permissions', { serverId: serverSegment }),
      label: 'Permissions',
      icon: 'iconify uil--shield-check'
    });
  }

  if (space.canManage) {
    items.push({
      href: resolve('/chat/[serverId]/server-admin/security', { serverId: serverSegment }),
      label: 'Security',
      icon: 'iconify uil--shield-exclamation'
    });
  }

  if (server.canAdminViewAudit) {
    items.push({
      href: resolve('/chat/[serverId]/server-admin/event-log', { serverId: serverSegment }),
      label: 'Event Log',
      icon: 'iconify uil--history'
    });
  }

  if (server.canAdminViewSystem) {
    items.push({
      href: resolve('/chat/[serverId]/server-admin/system', { serverId: serverSegment }),
      label: 'System',
      icon: 'iconify uil--server'
    });
  }

  return items;
}
