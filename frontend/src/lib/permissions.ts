/**
 * Permission metadata for the frontend.
 * This module provides human-readable descriptions and display names for all permissions.
 * These are defined in the frontend to support future i18n.
 */

export type PermissionMetadata = {
  displayName: string;
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
    displayName: 'Manage Server',
    description: 'Update server settings (name, description, logo)'
  },

  // Room permissions
  'room.create': {
    displayName: 'Create Rooms',
    description: 'Create new rooms in this group (or anywhere if granted at server scope)'
  },
  'room.join': {
    displayName: 'Join Rooms',
    description: 'Join existing rooms. Also gates room visibility — a user sees a room iff they are already a member OR can join it.'
  },
  'room.list': {
    displayName: 'Discover Rooms',
    description: "See rooms in the directory and group 'Join all' affordances"
  },
  'room.manage': {
    displayName: 'Manage Rooms',
    description: 'Edit, configure permissions on, and delete rooms'
  },

  // Message permissions
  'message.post': { displayName: 'Post Messages', description: 'Post new messages in rooms' },
  'message.post-in-thread': {
    displayName: 'Post in Threads',
    description: 'Post messages in threads'
  },
  'message.echo': {
    displayName: 'Echo to Channel',
    description: 'Echo thread replies to the main channel'
  },
  'message.manage': {
    displayName: 'Manage Messages',
    description: "Edit and delete other users' messages (subject to outranking the author)"
  },
  'message.react': { displayName: 'React to Messages', description: 'Add and remove reactions' },

  // Role management
  'role.manage': {
    displayName: 'Manage Roles',
    description: 'Create, edit, delete, and reorder roles and their permissions'
  },
  'role.assign': {
    displayName: 'Assign Roles',
    description: 'Assign and revoke roles for users'
  },

  // Admin panel
  'admin.access': { displayName: 'Admin Access', description: 'Access the admin panel' },
  'admin.view-users': { displayName: 'View Users', description: 'View the users page in admin' },
  'admin.view-system': {
    displayName: 'View System',
    description: 'View system and data pages in admin'
  },
  'admin.view-audit': {
    displayName: 'View Audit Log',
    description: 'View the audit log in admin'
  },

  // DM
  'dm.view': { displayName: 'View DMs', description: 'Access DMs and read direct messages' },
  'dm.write': {
    displayName: 'Send DMs',
    description: 'Start DM conversations and send messages'
  },

  // User management
  'user.delete-any': {
    displayName: 'Delete Any User',
    description: "Delete any user's account (subject to the rank check)"
  },
  'user.delete-self': {
    displayName: 'Delete Own Account',
    description: 'Delete your own account'
  }
};

/**
 * Get the description for a permission.
 * Returns the permission ID as fallback if not found.
 */
export function getPermissionDescription(id: string): string {
  return PERMISSION_METADATA[id]?.description ?? id;
}

/**
 * Get the display name for a permission.
 * Returns the permission ID as fallback if not found.
 */
export function getPermissionDisplayName(id: string): string {
  return PERMISSION_METADATA[id]?.displayName ?? id;
}
