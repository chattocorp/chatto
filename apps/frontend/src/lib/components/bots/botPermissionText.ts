import type { BotPermission } from '$lib/api-client/permissions';
import { m } from '$lib/i18n/messages';
import { getPermissionDescription, getPermissionCategory } from '$lib/permissions';

/** A scope heading and its actions, built only from server-authorised grants. */
export type BotPermissionGroup = {
  id: string;
  label: string;
  /** Static Iconify utility, bundled by the frontend build. */
  icon: string;
  actions: { id: string; text: string }[];
};

/** Group one active state at a time. Scope identity never depends on display names. */
export function groupBotPermissions(entries: BotPermission[]): BotPermissionGroup[] {
  const groups = new Map<string, { label: string; permissions: Set<string> }>();
  for (const entry of entries) {
    const category = getPermissionCategory(entry.permission);
    const kind =
      entry.scope === 'server'
        ? category === 'message' || category === 'call'
          ? 'joined_rooms'
          : category === 'room'
            ? 'all_rooms'
            : 'server'
        : entry.scope;
    const id = `${kind}:${entry.scopeId}`;
    let group = groups.get(id);
    if (!group) {
      const label =
        entry.scope === 'room'
          ? m('chat.profile.permissions.room', { name: entry.scopeName })
          : entry.scope === 'group'
            ? m('chat.profile.permissions.group', { name: entry.scopeName })
            : kind === 'dm'
              ? m('chat.profile.permissions.dm')
              : kind === 'joined_rooms'
                ? m('chat.profile.permissions.joined_rooms')
                : kind === 'all_rooms'
                  ? m('chat.profile.permissions.all_rooms')
                  : m('chat.profile.permissions.server');
      group = { label, permissions: new Set() };
      groups.set(id, group);
    }
    group.permissions.add(entry.permission);
  }
  const scopeOrder = ['server', 'all_rooms', 'joined_rooms', 'group', 'room', 'dm'];
  return [...groups]
    .sort(([a], [b]) => scopeOrder.indexOf(a.split(':')[0]) - scopeOrder.indexOf(b.split(':')[0]))
    .map(([id, group]) => {
      const combineBrowseJoin =
        group.permissions.has('room.list') && group.permissions.has('room.join');
      return {
        id,
        label: group.label,
        icon: id.startsWith('dm:')
          ? 'icon-[uil--comments]'
          : id.startsWith('group:')
            ? 'icon-[uil--layer-group]'
            : id.startsWith('room:')
              ? 'icon-[uil--comment-alt-lines]'
              : id.startsWith('joined_rooms:')
                ? 'icon-[uil--users-alt]'
                : id.startsWith('all_rooms:')
                  ? 'icon-[uil--apps]'
                  : 'icon-[uil--server]',
        actions: [...group.permissions]
          .filter((permission) => !combineBrowseJoin || permission !== 'room.list')
          .map((permission) => ({
            id: permission,
            text:
              combineBrowseJoin && permission === 'room.join'
                ? m('chat.profile.permissions.browse_join')
                : botPermissionAction(permission)
          }))
      };
    });
}

/** Offset pages can overlap after a permission change. Keep the newest entry. */
export function mergeBotPermissionPages(
  pages: { permissions: BotPermission[] }[]
): BotPermission[] {
  const unique = new Map<string, BotPermission>();
  for (const page of pages) {
    for (const entry of page.permissions) {
      unique.set(`${entry.permission}:${entry.scope}:${entry.scopeId}`, entry);
    }
  }
  return [...unique.values()];
}

// Keep bot actions concise and avoid human-only behaviour or protocol keys in
// descriptions shared with the permission editor.
function botPermissionAction(permission: string): string {
  switch (permission) {
    case 'message.read':
      return m('chat.profile.permissions.read');
    case 'message.read-interactions':
      return m('chat.profile.permissions.interactions');
    case 'message.post-in-thread':
      return m('chat.profile.permissions.reply');
    case 'message.manage':
      return m('chat.profile.permissions.manage_messages');
    case 'message.post':
      return m('chat.profile.permissions.post');
    case 'room.list':
      return m('chat.profile.permissions.list_rooms');
    case 'message.echo':
      return m('chat.profile.permissions.echo');
    case 'call.start':
      return m('chat.profile.permissions.start_calls');
    default:
      return getPermissionDescription(permission);
  }
}
