import { resolve } from '$app/paths';

export type AdminNavChromePermissions = {
  canViewAdmin: boolean;
  canManage: boolean;
  canManageRooms: boolean;
  canManageRoles: boolean;
  canAssignRoles: boolean;
  canManageUserPermissions: boolean;
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
  chrome,
  server
}: {
  serverSegment: string;
  chrome: AdminNavChromePermissions | null;
  server: AdminNavServerPermissions;
}): AdminNavItem[] {
  if (!chrome) return [];
  if (!chrome.canViewAdmin && !server.canViewAdmin) return [];

  const items: AdminNavItem[] = [];

  if (chrome.canManage) {
    items.push({
      href: resolve('/chat/[serverId]/server-admin/general', { serverId: serverSegment }),
      label: 'General',
      icon: 'iconify uil--setting'
    });
  }

  if (chrome.canAssignRoles || chrome.canManageUserPermissions || server.canAdminViewUsers) {
    items.push({
      href: resolve('/chat/[serverId]/server-admin/members', { serverId: serverSegment }),
      label: 'Members',
      icon: 'iconify uil--users-alt'
    });
  }

  if (chrome.canManageRooms) {
    items.push({
      href: resolve('/chat/[serverId]/server-admin/rooms', { serverId: serverSegment }),
      label: 'Rooms',
      icon: 'iconify uil--apps'
    });
  }

  if (chrome.canViewAdmin) {
    items.push({
      href: resolve('/chat/[serverId]/server-admin/moderation', { serverId: serverSegment }),
      label: 'Moderation',
      icon: 'iconify uil--ban'
    });
  }

  if (chrome.canManageRoles || server.canAdminViewRoles) {
    items.push({
      href: resolve('/chat/[serverId]/server-admin/permissions', { serverId: serverSegment }),
      label: 'Permissions',
      icon: 'iconify uil--shield-check'
    });
  }

  if (chrome.canManage) {
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
