import { resolve } from '$app/paths';
import { m } from '$lib/i18n/messages';
import type { ServerPermissions } from '@chatto/client/server/permissions';

/** The server permissions that decide which management pages the navigation lists. */
export type AdminNavPermissions = Pick<
  ServerPermissions,
  | 'loaded'
  | 'canManageServer'
  | 'canManageNeighbors'
  | 'canAdminViewUsers'
  | 'canManageInvites'
  | 'canInviteUsers'
  | 'canManageRooms'
  | 'canModerateRooms'
  | 'canAdminManageRoles'
  | 'canAssignRoles'
  | 'canAdminViewAudit'
  | 'canAdminViewSystem'
>;

export type AdminNavItem = {
  href: string;
  label: string;
  icon: string;
  /** Short explanation displayed on the Settings Overview card. */
  description: string;
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
      label: m('chat.management_nav.general'),
      description: m('settings.overview.descriptions.general'),
      icon: 'iconify icon-[uil--setting]'
    });
  }

  if (permissions.canManageServer) {
    items.push({
      href: resolve('/chat/[serverId]/manage/server/registration', { serverId: serverSegment }),
      label: m('chat.management_nav.registration'),
      description: m('settings.overview.descriptions.invitations'),
      icon: 'iconify icon-[uil--envelope-share]'
    });
  }

  if (permissions.canInviteUsers || permissions.canManageInvites) {
    items.push({
      href: resolve('/chat/[serverId]/manage/server/invites', { serverId: serverSegment }),
      label: m('chat.management_nav.invites'),
      description: m('settings.overview.descriptions.invitations'),
      icon: 'iconify icon-[uil--envelope-share]'
    });
  }

  if (permissions.canManageNeighbors) {
    items.push({
      href: resolve('/chat/[serverId]/manage/server/neighbors', { serverId: serverSegment }),
      label: m('chat.management_nav.neighbors'),
      description: m('settings.overview.descriptions.neighbors'),
      icon: 'iconify icon-[uil--servers]'
    });
  }

  if (permissions.canAdminViewUsers) {
    items.push({
      href: resolve('/chat/[serverId]/manage/server/members', { serverId: serverSegment }),
      label: m('chat.management_nav.members'),
      description: m('settings.overview.descriptions.members'),
      icon: 'iconify icon-[uil--users-alt]'
    });
  }

  // Bot ownership is itself sufficient to manage an existing bot, even after
  // bot.create is revoked. Keep this entry available to every signed-in human;
  // BotService remains authoritative for which bots and actions they may use.
  items.push({
    href: resolve('/chat/[serverId]/manage/server/bots', { serverId: serverSegment }),
    label: m('settings.bots.title'),
    description: m('settings.overview.descriptions.bots'),
    icon: 'iconify icon-[uil--robot]'
  });

  if (permissions.canManageRooms) {
    items.push({
      href: resolve('/chat/[serverId]/manage/rooms', { serverId: serverSegment }),
      label: m('chat.management_nav.rooms'),
      description: m('settings.overview.descriptions.rooms'),
      icon: 'iconify icon-[uil--apps]'
    });
  }

  if (permissions.canModerateRooms) {
    items.push({
      href: resolve('/chat/[serverId]/manage/server/moderation', { serverId: serverSegment }),
      label: m('chat.management_nav.moderation'),
      description: m('settings.overview.descriptions.moderation'),
      icon: 'iconify icon-[uil--ban]'
    });
  }

  if (permissions.canAdminManageRoles || permissions.canAssignRoles) {
    items.push({
      href: resolve('/chat/[serverId]/manage/server/permissions', { serverId: serverSegment }),
      label: m('chat.management_nav.permissions'),
      description: m('settings.overview.descriptions.permissions'),
      icon: 'iconify icon-[uil--shield-check]'
    });
  }

  if (permissions.canManageServer) {
    items.push({
      href: resolve('/chat/[serverId]/manage/server/security', { serverId: serverSegment }),
      label: m('chat.management_nav.security'),
      description: m('settings.overview.descriptions.security'),
      icon: 'iconify icon-[uil--shield-exclamation]'
    });
  }

  if (permissions.canAdminViewAudit) {
    items.push({
      href: resolve('/chat/[serverId]/manage/server/event-log', { serverId: serverSegment }),
      label: m('chat.management_nav.event_log'),
      description: m('settings.overview.descriptions.event_log'),
      icon: 'iconify icon-[uil--history]'
    });
  }

  if (permissions.canAdminViewSystem) {
    items.push({
      href: resolve('/chat/[serverId]/manage/server/system', { serverId: serverSegment }),
      label: m('chat.management_nav.system'),
      description: m('settings.overview.descriptions.system'),
      icon: 'iconify icon-[uil--server]'
    });
  }

  return items;
}
