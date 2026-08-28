/**
 * Permission metadata for the frontend.
 * This module provides descriptions for all permissions to support tooltips
 * and explanation surfaces. Defined in the frontend to support future i18n.
 */

import { m } from '$lib/i18n/messages';

export type PermissionMetadata = {
  category: PermissionCategory;
  description: () => string;
  includes?: readonly string[];
};

export type PermissionCategory =
  'admin' | 'bot' | 'message' | 'role' | 'room' | 'server' | 'user' | 'other';

const PERMISSION_CATEGORY_LABELS: Record<PermissionCategory, () => string> = {
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
    description: () => m('rbac.permission_descriptions.server_manage')
  },

  // Room permissions
  'room.create': {
    category: 'room',
    description: () => m('rbac.permission_descriptions.room_create')
  },
  'room.join': {
    category: 'room',
    description: () => m('rbac.permission_descriptions.room_join')
  },
  'room.list': {
    category: 'room',
    description: () => m('rbac.permission_descriptions.room_list')
  },
  'room.manage': {
    category: 'room',
    description: () => m('rbac.permission_descriptions.room_manage')
  },
  'room.ban-member': {
    category: 'room',
    description: () => m('rbac.permission_descriptions.room_ban_member')
  },

  // Message permissions
  'message.read': {
    category: 'message',
    description: () => m('rbac.permission_descriptions.message_read'),
    includes: ['message.read-interactions']
  },
  'message.read-interactions': {
    category: 'message',
    description: () => m('rbac.permission_descriptions.message_read_interactions')
  },
  'message.post': {
    category: 'message',
    description: () => m('rbac.permission_descriptions.message_post')
  },
  'message.post-in-thread': {
    category: 'message',
    description: () => m('rbac.permission_descriptions.message_post_in_thread')
  },
  'message.attach': {
    category: 'message',
    description: () => m('rbac.permission_descriptions.message_attach')
  },
  'message.echo': {
    category: 'message',
    description: () => m('rbac.permission_descriptions.message_echo')
  },
  'message.manage': {
    category: 'message',
    description: () => m('rbac.permission_descriptions.message_manage')
  },
  'message.react': {
    category: 'message',
    description: () => m('rbac.permission_descriptions.message_react')
  },

  // Role management
  'role.manage': {
    category: 'role',
    description: () => m('rbac.permission_descriptions.role_manage')
  },
  'role.assign': {
    category: 'role',
    description: () => m('rbac.permission_descriptions.role_assign')
  },

  // Admin panel
  'admin.view-users': {
    category: 'admin',
    description: () => m('rbac.permission_descriptions.admin_view_users')
  },
  'admin.view-audit': {
    category: 'admin',
    description: () => m('rbac.permission_descriptions.admin_view_audit')
  },

  // User management
  'user.delete-any': {
    category: 'user',
    description: () => m('rbac.permission_descriptions.user_delete_any')
  },
  'user.delete-self': {
    category: 'user',
    description: () => m('rbac.permission_descriptions.user_delete_self')
  },
  'user.invite': {
    category: 'user',
    description: () => m('rbac.permission_descriptions.user_invite')
  },
  'user.manage-accounts': {
    category: 'user',
    description: () => m('rbac.permission_descriptions.user_manage_accounts')
  },
  'user.manage-permissions': {
    category: 'user',
    description: () => m('rbac.permission_descriptions.user_manage_permissions')
  },

  // Bot accounts
  'bot.create': {
    category: 'bot',
    description: () => m('rbac.permission_descriptions.bot_create')
  },
  'bot.manage': {
    category: 'bot',
    description: () => m('rbac.permission_descriptions.bot_manage')
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
  return permissions.filter((candidate) =>
    PERMISSION_METADATA[candidate]?.includes?.includes(id)
  );
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
