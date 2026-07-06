import { resolve } from '$app/paths';
import * as m from '$lib/i18n/messages';

export type AdminNavChromePermissions = {
  canViewAdmin: boolean;
  canManage: boolean;
  canManageRooms: boolean;
  canManageRoles: boolean;
  canAssignRoles: boolean;
  canManageUserAccounts: boolean;
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
      label: m['admin.nav.general'](),
      icon: 'iconify uil--setting'
    });
  }

  if (server.canAdminViewUsers) {
    items.push({
      href: resolve('/chat/[serverId]/server-admin/members', { serverId: serverSegment }),
      label: m['admin.nav.members'](),
      icon: 'iconify uil--users-alt'
    });
  }

  if (chrome.canManageRooms) {
    items.push({
      href: resolve('/chat/[serverId]/server-admin/rooms', { serverId: serverSegment }),
      label: m['admin.nav.rooms'](),
      icon: 'iconify uil--apps'
    });
  }

  if (chrome.canViewAdmin) {
    items.push({
      href: resolve('/chat/[serverId]/server-admin/moderation', { serverId: serverSegment }),
      label: m['admin.nav.moderation'](),
      icon: 'iconify uil--ban'
    });
  }

  if (chrome.canManageRoles) {
    items.push({
      href: resolve('/chat/[serverId]/server-admin/permissions', { serverId: serverSegment }),
      label: m['admin.nav.permissions'](),
      icon: 'iconify uil--shield-check'
    });
  }

  if (chrome.canManage) {
    items.push({
      href: resolve('/chat/[serverId]/server-admin/security', { serverId: serverSegment }),
      label: m['admin.nav.security'](),
      icon: 'iconify uil--shield-exclamation'
    });
  }

  if (server.canAdminViewAudit) {
    items.push({
      href: resolve('/chat/[serverId]/server-admin/event-log', { serverId: serverSegment }),
      label: m['admin.nav.event_log'](),
      icon: 'iconify uil--history'
    });
  }

  if (server.canAdminViewSystem) {
    items.push({
      href: resolve('/chat/[serverId]/server-admin/system', { serverId: serverSegment }),
      label: m['admin.nav.system'](),
      icon: 'iconify uil--server'
    });
  }

  return items;
}
