/**
 * Permission text for the frontend: localized descriptions, help text, and
 * category headings. The permission structure (scopes, privileged mode, and
 * inclusion) comes from the client's permission catalogue.
 */

import {
  PERMISSION_DEFINITIONS,
  type PermissionCategory,
  type PermissionDefinition
} from '@chatto/client/util/permissionCatalog';
import { m } from '$lib/i18n/messages';

export {
  getIncludedByPermission,
  getIncludingPermissions,
  getPermissionCategory,
  type PermissionCategory,
  type PermissionScope
} from '@chatto/client/util/permissionCatalog';

type PermissionText = {
  /** One-line summary for filters and compact labels. */
  description: () => string;
  /** Detailed explanation for the permission help dialog. */
  help: () => string;
};

/** A catalogue definition with its localized text. */
export type PermissionMetadata = PermissionDefinition & PermissionText;

const PERMISSION_CATEGORY_LABELS: Record<PermissionCategory, () => string> = {
  call: () => m('permissions.categories.call'),
  admin: () => m('permissions.categories.admin'),
  bot: () => m('permissions.categories.bot'),
  message: () => m('permissions.categories.message'),
  role: () => m('permissions.categories.role'),
  room: () => m('permissions.categories.room'),
  server: () => m('permissions.categories.server'),
  user: () => m('permissions.categories.user'),
  other: () => m('permissions.categories.other')
};

/** Localized text for each permission in the client's catalogue. */
const PERMISSION_TEXT: Record<string, PermissionText> = {
  'server.manage': {
    description: () => m('permissions.descriptions.server_manage'),
    help: () => m('permissions.help.server_manage')
  },
  'server.manage-neighbors': {
    description: () => m('permissions.descriptions.server_manage_neighbors'),
    help: () => m('permissions.help.server_manage_neighbors')
  },
  'room.create': {
    description: () => m('permissions.descriptions.room_create'),
    help: () => m('permissions.help.room_create')
  },
  'room.join': {
    description: () => m('permissions.descriptions.room_join'),
    help: () => m('permissions.help.room_join')
  },
  'room.list': {
    description: () => m('permissions.descriptions.room_list'),
    help: () => m('permissions.help.room_list')
  },
  'room.manage': {
    description: () => m('permissions.descriptions.room_manage'),
    help: () => m('permissions.help.room_manage')
  },
  'room.remove-member': {
    description: () => m('permissions.descriptions.room_remove_member'),
    help: () => m('permissions.help.room_remove_member')
  },
  'call.start': {
    description: () => m('permissions.descriptions.call_start'),
    help: () => m('permissions.help.call_start')
  },
  'call.join': {
    description: () => m('permissions.descriptions.call_join'),
    help: () => m('permissions.help.call_join')
  },
  'call.voice': {
    description: () => m('permissions.descriptions.call_voice'),
    help: () => m('permissions.help.call_voice')
  },
  'call.camera': {
    description: () => m('permissions.descriptions.call_camera'),
    help: () => m('permissions.help.call_camera')
  },
  'call.screenshare': {
    description: () => m('permissions.descriptions.call_screenshare'),
    help: () => m('permissions.help.call_screenshare')
  },
  'message.read': {
    description: () => m('permissions.descriptions.message_read'),
    help: () => m('permissions.help.message_read')
  },
  'message.read-interactions': {
    description: () => m('permissions.descriptions.message_read_interactions'),
    help: () => m('permissions.help.message_read_interactions')
  },
  'message.post': {
    description: () => m('permissions.descriptions.message_post'),
    help: () => m('permissions.help.message_post')
  },
  'message.post-in-thread': {
    description: () => m('permissions.descriptions.message_post_in_thread'),
    help: () => m('permissions.help.message_post_in_thread')
  },
  'message.post-in-interactions': {
    description: () => m('permissions.descriptions.message_post_interactions'),
    help: () => m('permissions.help.message_post_interactions')
  },
  'message.attach': {
    description: () => m('permissions.descriptions.message_attach'),
    help: () => m('permissions.help.message_attach')
  },
  'message.echo': {
    description: () => m('permissions.descriptions.message_echo'),
    help: () => m('permissions.help.message_echo')
  },
  'message.pin': {
    description: () => m('permissions.descriptions.message_pin'),
    help: () => m('permissions.help.message_pin')
  },
  'message.manage': {
    description: () => m('permissions.descriptions.message_manage'),
    help: () => m('permissions.help.message_manage')
  },
  'message.react': {
    description: () => m('permissions.descriptions.message_react'),
    help: () => m('permissions.help.message_react')
  },
  'role.manage': {
    description: () => m('permissions.descriptions.role_manage'),
    help: () => m('permissions.help.role_manage')
  },
  'role.assign': {
    description: () => m('permissions.descriptions.role_assign'),
    help: () => m('permissions.help.role_assign')
  },
  'admin.view-users': {
    description: () => m('permissions.descriptions.admin_view_users'),
    help: () => m('permissions.help.admin_view_users')
  },
  'admin.view-audit': {
    description: () => m('permissions.descriptions.admin_view_audit'),
    help: () => m('permissions.help.admin_view_audit')
  },
  'user.delete-any': {
    description: () => m('permissions.descriptions.user_delete_any'),
    help: () => m('permissions.help.user_delete_any')
  },
  'user.delete-self': {
    description: () => m('permissions.descriptions.user_delete_self'),
    help: () => m('permissions.help.user_delete_self')
  },
  'user.invite': {
    description: () => m('permissions.descriptions.user_invite'),
    help: () => m('permissions.help.user_invite')
  },
  'user.manage-accounts': {
    description: () => m('permissions.descriptions.user_manage_accounts'),
    help: () => m('permissions.help.user_manage_accounts')
  },
  'user.manage-permissions': {
    description: () => m('permissions.descriptions.user_manage_permissions'),
    help: () => m('permissions.help.user_manage_permissions')
  },
  'bot.create': {
    description: () => m('permissions.descriptions.bot_create'),
    help: () => m('permissions.help.bot_create')
  },
  'bot.manage': {
    description: () => m('permissions.descriptions.bot_manage'),
    help: () => m('permissions.help.bot_manage')
  }
};

/** Map of known permission IDs to their definition and localized text. */
export const PERMISSION_METADATA: Record<string, PermissionMetadata> = Object.fromEntries(
  Object.entries(PERMISSION_DEFINITIONS).flatMap(([id, definition]) => {
    const text = PERMISSION_TEXT[id];
    return text ? [[id, { ...definition, ...text }]] : [];
  })
);

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
