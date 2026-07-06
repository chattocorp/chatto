/**
 * Permission metadata for the frontend.
 * This module provides descriptions for all permissions to support tooltips
 * and explanation surfaces. Defined in the frontend to support future i18n.
 */

import * as m from '$lib/i18n/messages';

export type PermissionMetadata = {
  description: () => string;
};

/**
 * Map of permission IDs to their metadata.
 * Keep in sync with cli/internal/core/permission.go
 *
 * Permission IDs follow the "{objectType}.{verb}" convention, matching the KV key format.
 */
export const PERMISSION_METADATA: Record<string, PermissionMetadata> = {
  // Server permissions
  'server.manage': {
    description: m['rbac.permission_descriptions.server_manage']
  },

  // Room permissions
  'room.create': {
    description: m['rbac.permission_descriptions.room_create']
  },
  'room.join': {
    description: m['rbac.permission_descriptions.room_join']
  },
  'room.list': {
    description: m['rbac.permission_descriptions.room_list']
  },
  'room.manage': {
    description: m['rbac.permission_descriptions.room_manage']
  },
  'room.ban-member': {
    description: m['rbac.permission_descriptions.room_ban_member']
  },

  // Message permissions
  'message.post': {
    description: m['rbac.permission_descriptions.message_post']
  },
  'message.post-in-thread': {
    description: m['rbac.permission_descriptions.message_post_in_thread']
  },
  'message.attach': {
    description: m['rbac.permission_descriptions.message_attach']
  },
  'message.echo': {
    description: m['rbac.permission_descriptions.message_echo']
  },
  'message.manage': {
    description: m['rbac.permission_descriptions.message_manage']
  },
  'message.react': {
    description: m['rbac.permission_descriptions.message_react']
  },

  // Role management
  'role.manage': {
    description: m['rbac.permission_descriptions.role_manage']
  },
  'role.assign': {
    description: m['rbac.permission_descriptions.role_assign']
  },

  // Admin panel
  'admin.view-users': {
    description: m['rbac.permission_descriptions.admin_view_users']
  },
  'admin.view-audit': {
    description: m['rbac.permission_descriptions.admin_view_audit']
  },

  // User management
  'user.delete-any': {
    description: m['rbac.permission_descriptions.user_delete_any']
  },
  'user.delete-self': {
    description: m['rbac.permission_descriptions.user_delete_self']
  },
  'user.manage-accounts': {
    description: m['rbac.permission_descriptions.user_manage_accounts']
  },
  'user.manage-permissions': {
    description: m['rbac.permission_descriptions.user_manage_permissions']
  }
};

/**
 * Get the description for a permission.
 * Returns the permission ID as fallback if not found.
 */
export function getPermissionDescription(id: string): string {
  return PERMISSION_METADATA[id]?.description() ?? id;
}
