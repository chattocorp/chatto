import { resolve } from '$app/paths';
import { m } from '$lib/i18n/messages';
import type { ServerFeature } from '@chatto/client/server/compatibility';
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
  /** Opens Shield Mode for a page available only in the preview. */
  requestAccess?: () => void;
  /** The selected Shield Mode roles grant access to this destination. */
  shieldActive?: boolean;
};

/** Every management destination, so each surface that links to one shows it the same way. */
const ADMIN_DESTINATIONS = {
  general: (serverId) => ({
    href: resolve('/chat/[serverId]/manage/server/general', { serverId }),
    label: m('chat.management_nav.general'),
    description: m('settings.overview.descriptions.general'),
    icon: 'iconify icon-[uil--setting]'
  }),
  registration: (serverId) => ({
    href: resolve('/chat/[serverId]/manage/server/registration', { serverId }),
    label: m('chat.management_nav.registration'),
    description: m('settings.overview.descriptions.registration'),
    icon: 'iconify icon-[uil--envelope-share]'
  }),
  invites: (serverId) => ({
    href: resolve('/chat/[serverId]/manage/server/invites', { serverId }),
    label: m('chat.management_nav.invites'),
    description: m('settings.overview.descriptions.invitations'),
    icon: 'iconify icon-[uil--envelope-share]'
  }),
  neighbors: (serverId) => ({
    href: resolve('/chat/[serverId]/manage/server/neighbors', { serverId }),
    label: m('chat.management_nav.neighbors'),
    description: m('settings.overview.descriptions.neighbors'),
    icon: 'iconify icon-[uil--servers]'
  }),
  members: (serverId) => ({
    href: resolve('/chat/[serverId]/manage/server/members', { serverId }),
    label: m('chat.management_nav.members'),
    description: m('settings.overview.descriptions.members'),
    icon: 'iconify icon-[uil--users-alt]'
  }),
  bots: (serverId) => ({
    href: resolve('/chat/[serverId]/manage/server/bots', { serverId }),
    label: m('settings.bots.title'),
    description: m('settings.overview.descriptions.bots'),
    icon: 'iconify icon-[uil--robot]'
  }),
  rooms: (serverId) => ({
    href: resolve('/chat/[serverId]/manage/rooms', { serverId }),
    label: m('chat.management_nav.rooms'),
    description: m('settings.overview.descriptions.rooms'),
    icon: 'iconify icon-[uil--apps]'
  }),
  moderation: (serverId) => ({
    href: resolve('/chat/[serverId]/manage/server/moderation', { serverId }),
    label: m('chat.management_nav.moderation'),
    description: m('settings.overview.descriptions.moderation'),
    icon: 'iconify icon-[uil--ban]'
  }),
  roles: (serverId) => ({
    href: resolve('/chat/[serverId]/manage/server/roles', { serverId }),
    label: m('chat.management_nav.roles'),
    description: m('settings.overview.descriptions.roles'),
    icon: 'iconify icon-[uil--shield]'
  }),
  permissions: (serverId) => ({
    href: resolve('/chat/[serverId]/manage/server/permissions', { serverId }),
    label: m('chat.management_nav.permissions'),
    description: m('settings.overview.descriptions.permissions'),
    icon: 'iconify icon-[uil--shield-check]'
  }),
  security: (serverId) => ({
    href: resolve('/chat/[serverId]/manage/server/security', { serverId }),
    label: m('chat.management_nav.security'),
    description: m('settings.overview.descriptions.security'),
    icon: 'iconify icon-[uil--shield-exclamation]'
  }),
  event_log: (serverId) => ({
    href: resolve('/chat/[serverId]/manage/server/event-log', { serverId }),
    label: m('chat.management_nav.event_log'),
    description: m('settings.overview.descriptions.event_log'),
    icon: 'iconify icon-[uil--history]'
  }),
  jobs: (serverId) => ({
    href: resolve('/chat/[serverId]/manage/server/jobs', { serverId }),
    label: m('chat.management_nav.jobs'),
    description: m('settings.overview.descriptions.jobs'),
    icon: 'iconify icon-[uil--process]'
  }),
  system: (serverId) => ({
    href: resolve('/chat/[serverId]/manage/server/system', { serverId }),
    label: m('chat.management_nav.system'),
    description: m('settings.overview.descriptions.system'),
    icon: 'iconify icon-[uil--server]'
  })
} satisfies Record<string, (serverSegment: string) => AdminNavItem>;

/** A management destination that the navigation, overview, or another surface links to. */
export type AdminDestination = keyof typeof ADMIN_DESTINATIONS;

/** Build the link for one management destination. Callers decide whether the viewer may use it. */
export function adminNavItem(destination: AdminDestination, serverSegment: string): AdminNavItem {
  return ADMIN_DESTINATIONS[destination](serverSegment);
}

export function getAdminNavItems({
  serverSegment,
  permissions,
  preview,
  limitedFeatures = []
}: {
  serverSegment: string;
  permissions: AdminNavPermissions;
  /** The server previews ordinary permissions while active and all protected roles while inactive. */
  preview?: {
    active: boolean;
    permissions: AdminNavPermissions;
    requestAccess: (href: string) => void;
  };
  /** Features that the server version does not support. Their pages are not listed. */
  limitedFeatures?: readonly ServerFeature[];
}): AdminNavItem[] {
  if (!permissions.loaded) return [];

  const items: AdminNavItem[] = [];

  function add(destination: AdminDestination, allowed: (p: AdminNavPermissions) => boolean) {
    if (allowed(permissions)) {
      const item = adminNavItem(destination, serverSegment);
      if (preview?.active && !allowed(preview.permissions)) item.shieldActive = true;
      items.push(item);
    } else if (preview && !preview.active && allowed(preview.permissions)) {
      const item = adminNavItem(destination, serverSegment);
      items.push({ ...item, requestAccess: () => preview.requestAccess(item.href) });
    }
  }

  add('general', (p) => p.canManageServer);
  add('registration', (p) => p.canManageServer);
  add('invites', (p) => p.canInviteUsers || p.canManageInvites);
  add('neighbors', (p) => p.canManageNeighbors);
  add('members', (p) => p.canAdminViewUsers);
  // A signed-in human can manage an existing bot they own after bot.create is revoked.
  items.push(adminNavItem('bots', serverSegment));
  add('rooms', (p) => p.canManageRooms);
  add('moderation', (p) => p.canModerateRooms);
  add('roles', (p) => p.canAdminManageRoles || p.canAssignRoles);
  add('permissions', (p) => p.canAdminManageRoles);
  add('security', (p) => p.canManageServer);
  add('event_log', (p) => p.canAdminViewAudit);
  if (!limitedFeatures.includes('backgroundJobs')) add('jobs', (p) => p.canManageServer);
  add('system', (p) => p.canAdminViewSystem);
  return items;
}
