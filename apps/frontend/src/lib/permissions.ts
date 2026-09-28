/**
 * Permission metadata for the frontend.
 * This module provides localized descriptions, help text, scopes, and
 * privileged-mode requirements for the permission explanation surfaces.
 */

import { m } from '$lib/i18n/messages';

/** A level where a permission can be configured. Mirrors `PermissionScope` in the backend. */
export type PermissionScope = 'server' | 'group' | 'room' | 'dm';

export type PermissionMetadata = {
  category: PermissionCategory;
  /** One-line summary for filters and compact labels. */
  description: () => string;
  /** Detailed explanation for the permission help dialog. */
  help: () => string;
  /** Levels where the permission can be configured, broadest first. */
  scopes: readonly PermissionScope[];
  /** A human session must enable privileged mode to use this permission. */
  privileged?: boolean;
  /** Permissions that an allow for this permission also grants. */
  includes?: readonly string[];
};

export type PermissionCategory =
  'admin' | 'bot' | 'call' | 'message' | 'role' | 'room' | 'server' | 'user' | 'other';

const PERMISSION_CATEGORY_LABELS: Record<PermissionCategory, () => string> = {
  call: () => m('rbac.permission_categories.call'),
  admin: () => m('rbac.permission_categories.admin'),
  bot: () => m('rbac.permission_categories.bot'),
  message: () => m('rbac.permission_categories.message'),
  role: () => m('rbac.permission_categories.role'),
  room: () => m('rbac.permission_categories.room'),
  server: () => m('rbac.permission_categories.server'),
  user: () => m('rbac.permission_categories.user'),
  other: () => m('rbac.permission_categories.other')
};

/**
 * Map of permission IDs to their metadata.
 * Keep in sync with cli/internal/core/permission.go
 *
 * Permission IDs are stable opaque keys. Inclusion relationships are explicit
 * metadata and do not follow punctuation in an ID.
 */
export const PERMISSION_METADATA: Record<string, PermissionMetadata> = {
  // Server permissions
  'server.manage': {
    category: 'server',
    description: () => m('rbac.permission_descriptions.server_manage'),
    help: () => m('rbac.permission_help.server_manage'),
    scopes: ['server'],
    privileged: true,
    includes: ['server.manage-neighbors']
  },
  'server.manage-neighbors': {
    category: 'server',
    description: () => m('rbac.permission_descriptions.server_manage_neighbors'),
    help: () => m('rbac.permission_help.server_manage_neighbors'),
    scopes: ['server'],
    privileged: true
  },

  // Room permissions
  'room.create': {
    category: 'room',
    description: () => m('rbac.permission_descriptions.room_create'),
    help: () => m('rbac.permission_help.room_create'),
    scopes: ['server', 'group'],
    privileged: true
  },
  'room.join': {
    category: 'room',
    description: () => m('rbac.permission_descriptions.room_join'),
    help: () => m('rbac.permission_help.room_join'),
    scopes: ['server', 'group', 'room']
  },
  'room.list': {
    category: 'room',
    description: () => m('rbac.permission_descriptions.room_list'),
    help: () => m('rbac.permission_help.room_list'),
    scopes: ['server', 'group', 'room']
  },
  'room.manage': {
    category: 'room',
    description: () => m('rbac.permission_descriptions.room_manage'),
    help: () => m('rbac.permission_help.room_manage'),
    scopes: ['server', 'group', 'room'],
    privileged: true
  },
  'room.remove-member': {
    category: 'room',
    description: () => m('rbac.permission_descriptions.room_remove_member'),
    help: () => m('rbac.permission_help.room_remove_member'),
    scopes: ['server', 'group', 'room'],
    privileged: true
  },

  // Call permissions
  'call.start': {
    category: 'call',
    description: () => m('rbac.permission_descriptions.call_start'),
    help: () => m('rbac.permission_help.call_start'),
    scopes: ['server', 'group', 'room', 'dm']
  },
  'call.join': {
    category: 'call',
    description: () => m('rbac.permission_descriptions.call_join'),
    help: () => m('rbac.permission_help.call_join'),
    scopes: ['server', 'group', 'room', 'dm']
  },
  'call.voice': {
    category: 'call',
    description: () => m('rbac.permission_descriptions.call_voice'),
    help: () => m('rbac.permission_help.call_voice'),
    scopes: ['server', 'group', 'room', 'dm']
  },
  'call.camera': {
    category: 'call',
    description: () => m('rbac.permission_descriptions.call_camera'),
    help: () => m('rbac.permission_help.call_camera'),
    scopes: ['server', 'group', 'room', 'dm']
  },
  'call.screenshare': {
    category: 'call',
    description: () => m('rbac.permission_descriptions.call_screenshare'),
    help: () => m('rbac.permission_help.call_screenshare'),
    scopes: ['server', 'group', 'room', 'dm']
  },

  // Message permissions
  'message.read': {
    category: 'message',
    description: () => m('rbac.permission_descriptions.message_read'),
    help: () => m('rbac.permission_help.message_read'),
    scopes: ['server', 'group', 'room', 'dm'],
    includes: ['message.read-interactions']
  },
  'message.read-interactions': {
    category: 'message',
    description: () => m('rbac.permission_descriptions.message_read_interactions'),
    help: () => m('rbac.permission_help.message_read_interactions'),
    scopes: ['server', 'group', 'room', 'dm']
  },
  'message.post': {
    category: 'message',
    description: () => m('rbac.permission_descriptions.message_post'),
    help: () => m('rbac.permission_help.message_post'),
    scopes: ['server', 'group', 'room', 'dm'],
    includes: ['message.post-in-thread', 'message.post-in-interactions']
  },
  'message.post-in-thread': {
    category: 'message',
    description: () => m('rbac.permission_descriptions.message_post_in_thread'),
    help: () => m('rbac.permission_help.message_post_in_thread'),
    scopes: ['server', 'group', 'room', 'dm']
  },
  'message.post-in-interactions': {
    category: 'message',
    description: () => m('rbac.permission_descriptions.message_post_interactions'),
    help: () => m('rbac.permission_help.message_post_interactions'),
    scopes: ['server', 'group', 'room', 'dm']
  },
  'message.attach': {
    category: 'message',
    description: () => m('rbac.permission_descriptions.message_attach'),
    help: () => m('rbac.permission_help.message_attach'),
    scopes: ['server', 'group', 'room', 'dm']
  },
  'message.echo': {
    category: 'message',
    description: () => m('rbac.permission_descriptions.message_echo'),
    help: () => m('rbac.permission_help.message_echo'),
    scopes: ['server', 'group', 'room', 'dm']
  },
  'message.manage': {
    category: 'message',
    description: () => m('rbac.permission_descriptions.message_manage'),
    help: () => m('rbac.permission_help.message_manage'),
    scopes: ['server', 'group', 'room', 'dm'],
    privileged: true
  },
  'message.react': {
    category: 'message',
    description: () => m('rbac.permission_descriptions.message_react'),
    help: () => m('rbac.permission_help.message_react'),
    scopes: ['server', 'group', 'room', 'dm']
  },

  // Role management
  'role.manage': {
    category: 'role',
    description: () => m('rbac.permission_descriptions.role_manage'),
    help: () => m('rbac.permission_help.role_manage'),
    scopes: ['server'],
    privileged: true
  },
  'role.assign': {
    category: 'role',
    description: () => m('rbac.permission_descriptions.role_assign'),
    help: () => m('rbac.permission_help.role_assign'),
    scopes: ['server'],
    privileged: true
  },

  // Admin panel
  'admin.view-users': {
    category: 'admin',
    description: () => m('rbac.permission_descriptions.admin_view_users'),
    help: () => m('rbac.permission_help.admin_view_users'),
    scopes: ['server'],
    privileged: true
  },
  'admin.view-audit': {
    category: 'admin',
    description: () => m('rbac.permission_descriptions.admin_view_audit'),
    help: () => m('rbac.permission_help.admin_view_audit'),
    scopes: ['server'],
    privileged: true
  },

  // User management
  'user.delete-any': {
    category: 'user',
    description: () => m('rbac.permission_descriptions.user_delete_any'),
    help: () => m('rbac.permission_help.user_delete_any'),
    scopes: ['server'],
    privileged: true
  },
  'user.delete-self': {
    category: 'user',
    description: () => m('rbac.permission_descriptions.user_delete_self'),
    help: () => m('rbac.permission_help.user_delete_self'),
    scopes: ['server']
  },
  'user.invite': {
    category: 'user',
    description: () => m('rbac.permission_descriptions.user_invite'),
    help: () => m('rbac.permission_help.user_invite'),
    scopes: ['server'],
    privileged: true
  },
  'user.manage-accounts': {
    category: 'user',
    description: () => m('rbac.permission_descriptions.user_manage_accounts'),
    help: () => m('rbac.permission_help.user_manage_accounts'),
    scopes: ['server'],
    privileged: true
  },
  'user.manage-permissions': {
    category: 'user',
    description: () => m('rbac.permission_descriptions.user_manage_permissions'),
    help: () => m('rbac.permission_help.user_manage_permissions'),
    scopes: ['server'],
    privileged: true
  },

  // Bot accounts
  'bot.create': {
    category: 'bot',
    description: () => m('rbac.permission_descriptions.bot_create'),
    help: () => m('rbac.permission_help.bot_create'),
    scopes: ['server']
  },
  'bot.manage': {
    category: 'bot',
    description: () => m('rbac.permission_descriptions.bot_manage'),
    help: () => m('rbac.permission_help.bot_manage'),
    scopes: ['server'],
    privileged: true
  }
};

/** Return the first registered permission that explicitly includes this ID. */
export function getIncludedByPermission(permissions: readonly string[], id: string): string | null {
  return getIncludingPermissions(permissions, id)[0] ?? null;
}

/** Return permissions that directly and explicitly include this ID. */
export function getIncludingPermissions(permissions: readonly string[], id: string): string[] {
  const registered = new Set(permissions);
  if (!registered.has(id)) return [];
  return permissions.filter((candidate) => PERMISSION_METADATA[candidate]?.includes?.includes(id));
}

/**
 * Return the presentation category for a permission.
 * Known permissions use explicit metadata. A recognized prefix is only a
 * display fallback for IDs from newer servers; it never defines authority.
 */
export function getPermissionCategory(id: string): PermissionCategory {
  const known = PERMISSION_METADATA[id]?.category;
  if (known) return known;
  const prefix = id.split('.', 1)[0] as PermissionCategory;
  return prefix in PERMISSION_CATEGORY_LABELS && prefix !== 'other' ? prefix : 'other';
}

/** Return the localized heading for a permission presentation category. */
export function getPermissionCategoryLabel(category: PermissionCategory): string {
  return PERMISSION_CATEGORY_LABELS[category]();
}

/**
 * Get the description for a permission.
 * Returns the permission ID as fallback if not found.
 */
export function getPermissionDescription(id: string): string {
  return PERMISSION_METADATA[id]?.description() ?? id;
}
