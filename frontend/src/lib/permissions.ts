/**
 * Permission metadata for the frontend.
 * This module provides descriptions for all permissions to support tooltips
 * and explanation surfaces. Defined in the frontend to support future i18n.
 */

export type PermissionMetadata = {
  description: string;
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
    description: 'Update server settings (name, description, logo)'
  },

  // Room permissions
  'room.create': {
    description: 'Create new rooms'
  },
  'room.join': {
    description: 'Join existing rooms'
  },
  'room.list': {
    description:
      'See rooms in the directory. A user can be allowed to see a room (e.g. to request access) without being allowed to join it.'
  },
  'room.manage': {
    description: "Edit a room's settings and permissions, and delete rooms"
  },
  'room.ban-member': {
    description: 'Ban members from rooms'
  },

  // Message permissions
  'message.post': {
    description: 'Post root messages in rooms and start DMs'
  },
  'message.post-in-thread': {
    description: 'Post replies inside a thread'
  },
  'message.attach': {
    description: 'Attach files to messages'
  },
  'message.echo': {
    description: 'When posting in a thread, also surface the message in the main channel'
  },
  'message.manage': {
    description: "Edit and delete other users' messages"
  },
  'message.react': {
    description: 'Add and remove reactions on messages'
  },

  // Role management
  'role.manage': {
    description: 'Create, edit, delete, and reorder roles and their permission grants'
  },
  'role.assign': {
    description: 'Assign and revoke roles for users'
  },

  // Admin panel
  'admin.view-users': {
    description: 'View the users page in admin'
  },
  'admin.view-system': {
    description: 'View projection diagnostics in admin'
  },
  'admin.view-audit': {
    description: 'View the audit log in admin'
  },

  // User management
  'user.delete-any': {
    description: "Delete another user's account"
  },
  'user.delete-self': {
    description: 'Delete your own account'
  },
  'user.manage-permissions': {
    description: 'Edit direct per-user permission overrides'
  }
};

/**
 * Get the description for a permission.
 * Returns the permission ID as fallback if not found.
 */
export function getPermissionDescription(id: string): string {
  return PERMISSION_METADATA[id]?.description ?? id;
}
