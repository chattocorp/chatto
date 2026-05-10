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
  // Space permissions
  'space.create': { displayName: 'Create Spaces', description: 'Create new spaces' },
  'space.manage': {
    displayName: 'Manage Space',
    description: 'Update space settings (name, description, logo)'
  },
  'space.delete': { displayName: 'Delete Space', description: 'Delete the space and all its data' },

  // Room permissions
  'room.list': { displayName: 'List Rooms', description: 'View the list of rooms' },
  'room.create': { displayName: 'Create Rooms', description: 'Create new rooms' },
  'room.join': { displayName: 'Join Rooms', description: 'Join existing rooms' },
  'room.leave': { displayName: 'Leave Rooms', description: 'Leave rooms' },
  'room.manage': { displayName: 'Manage Rooms', description: 'Edit and delete any room' },

  // Message permissions
  'message.post': { displayName: 'Post Messages', description: 'Post new messages in rooms' },
  'message.post-in-thread': {
    displayName: 'Post in Threads',
    description: 'Post messages in threads'
  },
  'message.reply': {
    displayName: 'Reply in Room',
    description: 'Use reply attribution on room-level messages'
  },
  'message.reply-in-thread': {
    displayName: 'Reply in Thread',
    description: 'Use reply attribution on thread messages'
  },
  'message.echo': {
    displayName: 'Echo to Channel',
    description: 'Echo thread replies to the main channel'
  },
  'message.edit-own': { displayName: 'Edit Own Messages', description: 'Edit your own messages' },
  'message.edit-any': {
    displayName: 'Edit Any Message',
    description: "Edit any user's messages"
  },
  'message.delete-own': {
    displayName: 'Delete Own Messages',
    description: 'Delete your own messages'
  },
  'message.delete-any': {
    displayName: 'Delete Any Message',
    description: "Delete any user's messages"
  },
  'message.react': { displayName: 'React to Messages', description: 'Add and remove reactions' },

  // Member management
  'member.invite': {
    displayName: 'Invite Members',
    description: 'Invite new members to the space'
  },
  'member.remove': { displayName: 'Remove Members', description: 'Remove members from the space' },

  // Role management
  'role.manage': { displayName: 'Manage Roles', description: 'Create, edit, and delete roles' },
  'role.assign': {
    displayName: 'Assign Roles',
    description: 'Assign and revoke roles for members'
  },

  // Instance admin
  'admin.access': { displayName: 'Admin Access', description: 'Access the admin panel' },
  'admin.view-users': { displayName: 'View Users', description: 'View the users page in admin' },
  'admin.manage-users': {
    displayName: 'Manage Users',
    description: 'Edit user role assignments'
  },
  'admin.view-roles': {
    displayName: 'View Roles',
    description: 'View the instance roles page in admin'
  },
  'admin.manage-roles': {
    displayName: 'Manage Instance Roles',
    description:
      'Full control over instance roles: create, edit, delete, reorder, and manage permissions'
  },
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
  'user.delete': { displayName: 'Delete Users', description: 'Delete user accounts' },
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
