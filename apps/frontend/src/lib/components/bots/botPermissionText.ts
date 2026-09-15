import type { BotPermission } from '$lib/api-client/bots';
import { getReactiveLocale } from '$lib/i18n/state.svelte';
import { m } from '$lib/i18n/messages';
import { getPermissionDescription, getPermissionCategory } from '$lib/permissions';

/** Describe a server-authorised grant. Text never determines permission access. */
export function botPermissionText(entry: BotPermission): string {
  const action = botPermissionAction(entry.permission);
  const category = getPermissionCategory(entry.permission);
  const scope =
    entry.scope === 'room'
      ? m('chat.profile.permissions.room', { name: entry.scopeName })
      : entry.scope === 'group'
        ? m('chat.profile.permissions.group', { name: entry.scopeName })
        : entry.scope === 'dm'
          ? m('chat.profile.permissions.dm')
          : category === 'message' || category === 'call'
            ? m('chat.profile.permissions.joined_rooms')
            : category === 'room'
              ? m('chat.profile.permissions.all_rooms')
              : m('chat.profile.permissions.server');
  return m(
    entry.active ? 'chat.profile.permissions.allowed' : 'chat.profile.permissions.inactive',
    {
      action:
        action !== entry.permission && getReactiveLocale().startsWith('en-')
          ? action[0].toLowerCase() + action.slice(1)
          : action,
      scope
    }
  );
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
