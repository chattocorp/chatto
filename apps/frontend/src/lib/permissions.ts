/**
 * Permission metadata for the frontend.
 * This module provides descriptions for all permissions to support tooltips
 * and explanation surfaces. Defined in the frontend to support future i18n.
 */

import { m } from '$lib/i18n/messages';

export type PermissionMetadata = {
  label: () => string;
  description: () => string;
  includes?: readonly string[];
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
    label: () => m('rbac.permission_labels.server_manage'),
    description: () => m('rbac.permission_descriptions.server_manage')
  },

  // Room permissions
  'room.create': {
    label: () => m('rbac.permission_labels.room_create'),
    description: () => m('rbac.permission_descriptions.room_create')
  },
  'room.join': {
    label: () => m('rbac.permission_labels.room_join'),
    description: () => m('rbac.permission_descriptions.room_join')
  },
  'room.list': {
    label: () => m('rbac.permission_labels.room_list'),
    description: () => m('rbac.permission_descriptions.room_list')
  },
  'room.manage': {
    label: () => m('rbac.permission_labels.room_manage'),
    description: () => m('rbac.permission_descriptions.room_manage')
  },
  'room.ban-member': {
    label: () => m('rbac.permission_labels.room_ban_member'),
    description: () => m('rbac.permission_descriptions.room_ban_member')
  },

  // Message permissions
  'message.read': {
    label: () => m('rbac.permission_labels.message_read'),
    description: () => m('rbac.permission_descriptions.message_read'),
    includes: ['message.read.interactions']
  },
  'message.read.interactions': {
    label: () => m('rbac.permission_labels.message_read_interactions'),
    description: () => m('rbac.permission_descriptions.message_read_interactions')
  },
  'message.post': {
    label: () => m('rbac.permission_labels.message_post'),
    description: () => m('rbac.permission_descriptions.message_post')
  },
  'message.post-in-thread': {
    label: () => m('rbac.permission_labels.message_post_in_thread'),
    description: () => m('rbac.permission_descriptions.message_post_in_thread')
  },
  'message.attach': {
    label: () => m('rbac.permission_labels.message_attach'),
    description: () => m('rbac.permission_descriptions.message_attach')
  },
  'message.echo': {
    label: () => m('rbac.permission_labels.message_echo'),
    description: () => m('rbac.permission_descriptions.message_echo')
  },
  'message.manage': {
    label: () => m('rbac.permission_labels.message_manage'),
    description: () => m('rbac.permission_descriptions.message_manage')
  },
  'message.react': {
    label: () => m('rbac.permission_labels.message_react'),
    description: () => m('rbac.permission_descriptions.message_react')
  },

  // Role management
  'role.manage': {
    label: () => m('rbac.permission_labels.role_manage'),
    description: () => m('rbac.permission_descriptions.role_manage')
  },
  'role.assign': {
    label: () => m('rbac.permission_labels.role_assign'),
    description: () => m('rbac.permission_descriptions.role_assign')
  },

  // Admin panel
  'admin.view-users': {
    label: () => m('rbac.permission_labels.admin_view_users'),
    description: () => m('rbac.permission_descriptions.admin_view_users')
  },
  'admin.view-audit': {
    label: () => m('rbac.permission_labels.admin_view_audit'),
    description: () => m('rbac.permission_descriptions.admin_view_audit')
  },

  // User management
  'user.delete-any': {
    label: () => m('rbac.permission_labels.user_delete_any'),
    description: () => m('rbac.permission_descriptions.user_delete_any')
  },
  'user.delete-self': {
    label: () => m('rbac.permission_labels.user_delete_self'),
    description: () => m('rbac.permission_descriptions.user_delete_self')
  },
  'user.invite': {
    label: () => m('rbac.permission_labels.user_invite'),
    description: () => m('rbac.permission_descriptions.user_invite')
  },
  'user.manage-accounts': {
    label: () => m('rbac.permission_labels.user_manage_accounts'),
    description: () => m('rbac.permission_descriptions.user_manage_accounts')
  },
  'user.manage-permissions': {
    label: () => m('rbac.permission_labels.user_manage_permissions'),
    description: () => m('rbac.permission_descriptions.user_manage_permissions')
  },

  // Bot accounts
  'bot.create': {
    label: () => m('rbac.permission_labels.bot_create'),
    description: () => m('rbac.permission_descriptions.bot_create')
  },
  'bot.manage': {
    label: () => m('rbac.permission_labels.bot_manage'),
    description: () => m('rbac.permission_descriptions.bot_manage')
  }
};

/** Return the first registered permission that explicitly includes this ID. */
export function getIncludedByPermission(permissions: readonly string[], id: string): string | null {
  return getIncludingPermissions(permissions, id)[0] ?? null;
}

/** Return explicit includers from the nearest relationship to the broadest. */
export function getIncludingPermissions(permissions: readonly string[], id: string): string[] {
  const registered = new Set(permissions);
  if (!registered.has(id)) return [];
  const result: string[] = [];
  const seen = new Set<string>();
  const targets = [id];
  while (targets.length > 0) {
    const target = targets.shift();
    if (target === undefined) break;
    for (const candidate of permissions) {
      if (seen.has(candidate) || !PERMISSION_METADATA[candidate]?.includes?.includes(target))
        continue;
      seen.add(candidate);
      result.push(candidate);
      targets.push(candidate);
    }
  }
  return result;
}

/**
 * Get the short localized label for a permission.
 * Returns the permission ID as fallback if it is unknown to this client.
 */
export function getPermissionLabel(id: string): string {
  return PERMISSION_METADATA[id]?.label() ?? id;
}

/**
 * Get the description for a permission.
 * Returns the permission ID as fallback if not found.
 */
export function getPermissionDescription(id: string): string {
  return PERMISSION_METADATA[id]?.description() ?? id;
}
