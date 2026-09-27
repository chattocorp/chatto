import { resolve } from '$app/paths';
import { m } from '$lib/i18n/messages';
import type { ServerPermissions } from '$lib/state/server/permissions';

/** The server permissions that decide which management pages the navigation lists. */
export type AdminNavPermissions = Pick<
  ServerPermissions,
  | 'loaded'
  | 'canManageServer'
  | 'canManageNeighbors'
  | 'canAdminViewUsers'
  | 'canManageInvites'
  | 'canManageRooms'
  | 'canModerateRooms'
  | 'canAdminManageRoles'
  | 'canAdminViewAudit'
  | 'canAdminViewSystem'
>;

export type AdminNavItem = {
  href: string;
  label: string;
  icon: string;
};

export function getAdminNavItems({
  serverSegment,
  permissions
}: {
  serverSegment: string;
  permissions: AdminNavPermissions;
}): AdminNavItem[] {
  if (!permissions.loaded) return [];

  const items: AdminNavItem[] = [];

  if (permissions.canManageServer) {
    items.push({
      href: resolve('/chat/[serverId]/manage/server/general', { serverId: serverSegment }),
      label: m('admin.nav.general'),
      icon: 'iconify icon-[uil--setting]'
    });
  }

  if (permissions.canManageNeighbors) {
    items.push({
      href: resolve('/chat/[serverId]/manage/server/neighbors', { serverId: serverSegment }),
      label: m('admin.nav.neighbors'),
      icon: 'iconify icon-[uil--servers]'
    });
  }

  if (permissions.canAdminViewUsers) {
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

  if (permissions.canManageInvites) {
    items.push({
      href: resolve('/chat/[serverId]/manage/server/invite-links', { serverId: serverSegment }),
      label: m('admin.nav.invitations'),
      icon: 'iconify icon-[uil--envelope-share]'
    });
  }

  if (permissions.canManageRooms) {
    items.push({
      href: resolve('/chat/[serverId]/manage/rooms', { serverId: serverSegment }),
      label: m('admin.nav.rooms'),
      icon: 'iconify icon-[uil--apps]'
    });
  }

  if (permissions.canModerateRooms) {
    items.push({
      href: resolve('/chat/[serverId]/manage/server/moderation', { serverId: serverSegment }),
      label: m('admin.nav.moderation'),
      icon: 'iconify icon-[uil--ban]'
    });
  }

  if (permissions.canAdminManageRoles) {
    items.push({
      href: resolve('/chat/[serverId]/manage/server/permissions', { serverId: serverSegment }),
      label: m('admin.nav.permissions'),
      icon: 'iconify icon-[uil--shield-check]'
    });
  }

  if (permissions.canManageServer) {
    items.push({
      href: resolve('/chat/[serverId]/manage/server/security', { serverId: serverSegment }),
      label: m('admin.nav.security'),
      icon: 'iconify icon-[uil--shield-exclamation]'
    });
  }

  if (permissions.canAdminViewAudit) {
    items.push({
      href: resolve('/chat/[serverId]/manage/server/event-log', { serverId: serverSegment }),
      label: m('admin.nav.event_log'),
      icon: 'iconify icon-[uil--history]'
    });
  }

  if (permissions.canAdminViewSystem) {
    items.push({
      href: resolve('/chat/[serverId]/manage/server/system', { serverId: serverSegment }),
      label: m('admin.nav.system'),
      icon: 'iconify icon-[uil--server]'
    });
  }

  return items;
}
