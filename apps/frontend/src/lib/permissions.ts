/**
 * Permission text for the frontend: localized descriptions, help text, and
 * category headings. The permission structure (scopes, privileged mode, and
 * inclusion) comes from the client's permission catalogue.
 * Descriptions are available in chat for bot profiles. Help text and category
 * headings require the management route's `rbac` catalog.
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

/** Localized text for each permission in the client's catalogue. */
const PERMISSION_TEXT: Record<string, PermissionText> = {
  'server.manage': {
    description: () => m('permissions.descriptions.server_manage'),
    help: () => m('rbac.permission_help.server_manage')
  },
  'server.manage-neighbors': {
    description: () => m('permissions.descriptions.server_manage_neighbors'),
    help: () => m('rbac.permission_help.server_manage_neighbors')
  },
  'room.create': {
    description: () => m('permissions.descriptions.room_create'),
    help: () => m('rbac.permission_help.room_create')
  },
  'room.invite': {
    description: () => m('permissions.descriptions.room_invite'),
    help: () => m('rbac.permission_help.room_invite')
  },
  'room.join': {
    description: () => m('permissions.descriptions.room_join'),
    help: () => m('rbac.permission_help.room_join')
  },
  'room.list': {
    description: () => m('permissions.descriptions.room_list'),
    help: () => m('rbac.permission_help.room_list')
  },
  'room.manage': {
    description: () => m('permissions.descriptions.room_manage'),
    help: () => m('rbac.permission_help.room_manage')
  },
  'room.manage-notice': {
    description: () => m('permissions.descriptions.room_manage_notice'),
    help: () => m('rbac.permission_help.room_manage_notice')
  },
  'room.remove-member': {
    description: () => m('permissions.descriptions.room_remove_member'),
    help: () => m('rbac.permission_help.room_remove_member')
  },
  'call.start': {
    description: () => m('permissions.descriptions.call_start'),
    help: () => m('rbac.permission_help.call_start')
  },
  'call.join': {
    description: () => m('permissions.descriptions.call_join'),
    help: () => m('rbac.permission_help.call_join')
  },
  'call.voice': {
    description: () => m('permissions.descriptions.call_voice'),
    help: () => m('rbac.permission_help.call_voice')
  },
  'call.camera': {
    description: () => m('permissions.descriptions.call_camera'),
    help: () => m('rbac.permission_help.call_camera')
  },
  'call.screenshare': {
    description: () => m('permissions.descriptions.call_screenshare'),
    help: () => m('rbac.permission_help.call_screenshare')
  },
  'message.read': {
    description: () => m('permissions.descriptions.message_read'),
    help: () => m('rbac.permission_help.message_read')
  },
  'message.read-interactions': {
    description: () => m('permissions.descriptions.message_read_interactions'),
    help: () => m('rbac.permission_help.message_read_interactions')
  },
  'message.post': {
    description: () => m('permissions.descriptions.message_post'),
    help: () => m('rbac.permission_help.message_post')
  },
  'message.post-in-thread': {
    description: () => m('permissions.descriptions.message_post_in_thread'),
    help: () => m('rbac.permission_help.message_post_in_thread')
  },
  'message.post-in-interactions': {
    description: () => m('permissions.descriptions.message_post_interactions'),
    help: () => m('rbac.permission_help.message_post_interactions')
  },
  'message.attach': {
    description: () => m('permissions.descriptions.message_attach'),
    help: () => m('rbac.permission_help.message_attach')
  },
  'message.echo': {
    description: () => m('permissions.descriptions.message_echo'),
    help: () => m('rbac.permission_help.message_echo')
  },
  'message.pin': {
    description: () => m('permissions.descriptions.message_pin'),
    help: () => m('rbac.permission_help.message_pin')
  },
  'message.manage': {
    description: () => m('permissions.descriptions.message_manage'),
    help: () => m('rbac.permission_help.message_manage')
  },
  'message.react': {
    description: () => m('permissions.descriptions.message_react'),
    help: () => m('rbac.permission_help.message_react')
  },
  'role.manage': {
    description: () => m('permissions.descriptions.role_manage'),
    help: () => m('rbac.permission_help.role_manage')
  },
  'role.assign': {
    description: () => m('permissions.descriptions.role_assign'),
    help: () => m('rbac.permission_help.role_assign')
  },
  'admin.view-users': {
    description: () => m('permissions.descriptions.admin_view_users'),
    help: () => m('rbac.permission_help.admin_view_users')
  },
  'admin.view-audit': {
    description: () => m('permissions.descriptions.admin_view_audit'),
    help: () => m('rbac.permission_help.admin_view_audit')
  },
  'user.delete-any': {
    description: () => m('permissions.descriptions.user_delete_any'),
    help: () => m('rbac.permission_help.user_delete_any')
  },
  'user.delete-self': {
    description: () => m('permissions.descriptions.user_delete_self'),
    help: () => m('rbac.permission_help.user_delete_self')
  },
  'user.invite': {
    description: () => m('permissions.descriptions.user_invite'),
    help: () => m('rbac.permission_help.user_invite')
  },
  'user.manage-invites': {
    description: () => m('permissions.descriptions.user_manage_invites'),
    help: () => m('rbac.permission_help.user_manage_invites')
  },
  'user.manage-accounts': {
    description: () => m('permissions.descriptions.user_manage_accounts'),
    help: () => m('rbac.permission_help.user_manage_accounts')
  },
  'user.manage-permissions': {
    description: () => m('permissions.descriptions.user_manage_permissions'),
    help: () => m('rbac.permission_help.user_manage_permissions')
  },
  'bot.create': {
    description: () => m('permissions.descriptions.bot_create'),
    help: () => m('rbac.permission_help.bot_create')
  },
  'bot.manage': {
    description: () => m('permissions.descriptions.bot_manage'),
    help: () => m('rbac.permission_help.bot_manage')
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
