import { resolve } from '$app/paths';
import { m } from '$lib/i18n/messages';

export type AdminNavChromePermissions = {
  canViewAdmin: boolean;
  canManage: boolean;
  canManageNeighbors: boolean;
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
  canManageInvites: boolean;
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

  const items: AdminNavItem[] = [];

  if (chrome.canManage) {
    items.push({
      href: resolve('/chat/[serverId]/manage/server/general', { serverId: serverSegment }),
      label: m('admin.nav.general'),
      icon: 'iconify icon-[uil--setting]'
    });
  }

  if (chrome.canManageNeighbors) {
    items.push({
      href: resolve('/chat/[serverId]/manage/server/neighbors', { serverId: serverSegment }),
      label: m('admin.nav.neighbors'),
      icon: 'iconify icon-[uil--servers]'
    });
  }

  if (server.canAdminViewUsers) {
    items.push({
      href: resolve('/chat/[serverId]/manage/server/members', { serverId: serverSegment }),
      label: m('admin.nav.members'),
      icon: 'iconify icon-[uil--users-alt]'
    });
  }

  // Bot ownership is itself sufficient to manage an existing bot, even after
  // bot.create is revoked. Keep this entry available to every signed-in human;
  // BotService remains authoritative for which bots and actions they may use.
  items.push({
    href: resolve('/chat/[serverId]/manage/server/bots', { serverId: serverSegment }),
    label: m('settings.bots.title'),
    icon: 'iconify icon-[uil--robot]'
  });

  if (server.canManageInvites) {
    items.push({
      href: resolve('/chat/[serverId]/manage/server/invite-links', { serverId: serverSegment }),
      label: m('admin.nav.invitations'),
      icon: 'iconify icon-[uil--envelope-share]'
    });
  }

  if (chrome.canManageRooms) {
    items.push({
      href: resolve('/chat/[serverId]/manage/rooms', { serverId: serverSegment }),
      label: m('admin.nav.rooms'),
      icon: 'iconify icon-[uil--apps]'
    });
  }

  if (chrome.canViewAdmin) {
    items.push({
      href: resolve('/chat/[serverId]/manage/server/moderation', { serverId: serverSegment }),
      label: m('admin.nav.moderation'),
      icon: 'iconify icon-[uil--ban]'
    });
  }

  if (chrome.canManageRoles) {
    items.push({
      href: resolve('/chat/[serverId]/manage/server/permissions', { serverId: serverSegment }),
      label: m('admin.nav.permissions'),
      icon: 'iconify icon-[uil--shield-check]'
    });
  }

  if (chrome.canManage) {
    items.push({
      href: resolve('/chat/[serverId]/manage/server/security', { serverId: serverSegment }),
      label: m('admin.nav.security'),
      icon: 'iconify icon-[uil--shield-exclamation]'
    });
  }

  if (server.canAdminViewAudit) {
    items.push({
      href: resolve('/chat/[serverId]/manage/server/event-log', { serverId: serverSegment }),
      label: m('admin.nav.event_log'),
      icon: 'iconify icon-[uil--history]'
    });
  }

  if (server.canAdminViewSystem) {
    items.push({
      href: resolve('/chat/[serverId]/manage/server/system', { serverId: serverSegment }),
      label: m('admin.nav.system'),
      icon: 'iconify icon-[uil--server]'
    });
  }

  return items;
}
