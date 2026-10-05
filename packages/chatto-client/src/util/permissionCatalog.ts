/**
 * Catalogue of the server's permissions: where each permission can be
 * configured, whether it needs privileged mode, and which permissions it
 * includes. Hosts use it to explain and group permissions. It never decides
 * authority: the server checks every request.
 *
 * Keep in sync with `cli/internal/core/permission.go`. Permission IDs are
 * stable opaque keys. Inclusion is explicit and does not follow punctuation in
 * an ID.
 */

/** A level where a permission can be configured. Mirrors `PermissionScope` in the backend. */
export type PermissionScope = 'server' | 'group' | 'room' | 'dm';

/** Categories that hosts use to group permissions. */
export const PERMISSION_CATEGORIES = [
  'admin',
  'bot',
  'call',
  'message',
  'role',
  'room',
  'server',
  'user',
  'other'
] as const;

/** A category that hosts use to group permissions. */
export type PermissionCategory = (typeof PERMISSION_CATEGORIES)[number];

/** The structure of one permission. */
export type PermissionDefinition = {
  /** Category for grouping. */
  category: PermissionCategory;
  /** Levels where the permission can be configured, broadest first. */
  scopes: readonly PermissionScope[];
  /** A human session must enable privileged mode to use this permission. */
  privileged?: boolean;
  /** Permissions that an allow for this permission also grants. */
  includes?: readonly string[];
};

/** Definitions of the permissions that this client knows, by permission ID. */
export const PERMISSION_DEFINITIONS: Readonly<Record<string, PermissionDefinition>> = {
  // Server permissions
  'server.manage': {
    category: 'server',
    scopes: ['server'],
    privileged: true,
    includes: ['server.manage-neighbors']
  },
  'server.manage-neighbors': {
    category: 'server',
    scopes: ['server'],
    privileged: true
  },

  // Room permissions
  'room.create': {
    category: 'room',
    scopes: ['server', 'group'],
    privileged: true
  },
  'room.join': {
    category: 'room',
    scopes: ['server', 'group', 'room']
  },
  'room.list': {
    category: 'room',
    scopes: ['server', 'group', 'room']
  },
  'room.manage': {
    category: 'room',
    scopes: ['server', 'group', 'room'],
    privileged: true
  },
  'room.remove-member': {
    category: 'room',
    scopes: ['server', 'group', 'room'],
    privileged: true
  },

  // Call permissions
  'call.start': {
    category: 'call',
    scopes: ['server', 'group', 'room', 'dm']
  },
  'call.join': {
    category: 'call',
    scopes: ['server', 'group', 'room', 'dm']
  },
  'call.voice': {
    category: 'call',
    scopes: ['server', 'group', 'room', 'dm']
  },
  'call.camera': {
    category: 'call',
    scopes: ['server', 'group', 'room', 'dm']
  },
  'call.screenshare': {
    category: 'call',
    scopes: ['server', 'group', 'room', 'dm']
  },

  // Message permissions
  'message.read': {
    category: 'message',
    scopes: ['server', 'group', 'room', 'dm'],
    includes: ['message.read-interactions']
  },
  'message.read-interactions': {
    category: 'message',
    scopes: ['server', 'group', 'room', 'dm']
  },
  'message.post': {
    category: 'message',
    scopes: ['server', 'group', 'room', 'dm'],
    includes: ['message.post-in-thread', 'message.post-in-interactions']
  },
  'message.post-in-thread': {
    category: 'message',
    scopes: ['server', 'group', 'room', 'dm']
  },
  'message.post-in-interactions': {
    category: 'message',
    scopes: ['server', 'group', 'room', 'dm']
  },
  'message.attach': {
    category: 'message',
    scopes: ['server', 'group', 'room', 'dm']
  },
  'message.echo': {
    category: 'message',
    scopes: ['server', 'group', 'room', 'dm']
  },
  'message.manage': {
    category: 'message',
    scopes: ['server', 'group', 'room', 'dm'],
    privileged: true
  },
  'message.react': {
    category: 'message',
    scopes: ['server', 'group', 'room', 'dm']
  },

  // Role management
  'role.manage': {
    category: 'role',
    scopes: ['server'],
    privileged: true
  },
  'role.assign': {
    category: 'role',
    scopes: ['server'],
    privileged: true
  },

  // Admin panel
  'admin.view-users': {
    category: 'admin',
    scopes: ['server'],
    privileged: true
  },
  'admin.view-audit': {
    category: 'admin',
    scopes: ['server'],
    privileged: true
  },

  // User management
  'user.delete-any': {
    category: 'user',
    scopes: ['server'],
    privileged: true
  },
  'user.delete-self': {
    category: 'user',
    scopes: ['server']
  },
  'user.invite': {
    category: 'user',
    scopes: ['server'],
    privileged: true
  },
  'user.manage-accounts': {
    category: 'user',
    scopes: ['server'],
    privileged: true
  },
  'user.manage-permissions': {
    category: 'user',
    scopes: ['server'],
    privileged: true
  },

  // Bot accounts
  'bot.create': {
    category: 'bot',
    scopes: ['server']
  },
  'bot.manage': {
    category: 'bot',
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
  return permissions.filter((candidate) =>
    PERMISSION_DEFINITIONS[candidate]?.includes?.includes(id)
  );
}

/**
 * Return the category of a permission.
 * Known permissions use their definition. For an ID from a newer server, a
 * known prefix selects the category, for display only; it never defines
 * authority.
 */
export function getPermissionCategory(id: string): PermissionCategory {
  const known = PERMISSION_DEFINITIONS[id]?.category;
  if (known) return known;
  const prefix = id.split('.', 1)[0];
  return (PERMISSION_CATEGORIES as readonly string[]).includes(prefix) && prefix !== 'other'
    ? (prefix as PermissionCategory)
    : 'other';
}
