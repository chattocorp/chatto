import type { EffectivePermission } from '$lib/api-client/effectivePermissions';
import type { MatrixData, MatrixScope } from '$lib/api-client/permissions';
import { m } from '$lib/i18n/messages';
import {
  getIncludingPermissions,
  getPermissionDescription,
  getPermissionCategory
} from '$lib/permissions';

/** Presentation entry shared by effective access and manager-only inactive grants. */
export type BotPermission = Pick<
  EffectivePermission,
  'permission' | 'scope' | 'scopeId' | 'scopeName'
> & { active: boolean };

/** A scope heading and its actions, built only from server-authorised grants. */
export type BotPermissionGroup = {
  id: string;
  label: string;
  /** Static Iconify utility, bundled by the frontend build. */
  icon: string;
  actions: { id: string; text: string }[];
};

/** Group one active state at a time. Scope identity never depends on display names. */
export function groupBotPermissions(
  entries: BotPermission[],
  configuration = false
): BotPermissionGroup[] {
  const groups = new Map<string, { label: string; permissions: Set<string> }>();
  for (const entry of entries) {
    const category = getPermissionCategory(entry.permission);
    const kind =
      entry.scope === 'server'
        ? configuration
          ? 'server'
          : category === 'message' || category === 'call'
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

/** Collapse display entries only when the server proves coverage of hidden children. */
export function compactEffectivePermissions(entries: EffectivePermission[]): BotPermission[] {
  const unique = new Map<string, EffectivePermission>();
  for (const entry of entries)
    unique.set(`${entry.permission}:${entry.scope}:${entry.scopeId}`, entry);
  const all = [...unique.values()];
  const byKey = new Map(
    all.map((entry) => [`${entry.permission}:${entry.scope}:${entry.scopeId}`, entry])
  );
  const permissions = [...new Set(all.map((entry) => entry.permission))];
  const covers = (permission: string, target: EffectivePermission): boolean => {
    if (byKey.get(`${permission}:${target.scope}:${target.scopeId}`)?.coversDescendants)
      return true;
    if (target.scope === 'group' || target.scope === 'room') {
      if (byKey.get(`${permission}:server:`)?.coversDescendants) return true;
    }
    return (
      target.scope === 'room' &&
      !!byKey.get(`${permission}:group:${target.parentGroupId}`)?.coversDescendants
    );
  };
  return all
    .filter((entry) => {
      if (!entry.coversDescendants) return false;
      if (
        (entry.scope === 'group' || entry.scope === 'room') &&
        byKey.get(`${entry.permission}:server:`)?.coversDescendants
      )
        return false;
      if (
        entry.scope === 'room' &&
        byKey.get(`${entry.permission}:group:${entry.parentGroupId}`)?.coversDescendants
      )
        return false;
      return !getIncludingPermissions(permissions, entry.permission).some((permission) =>
        covers(permission, entry)
      );
    })
    .map((entry) => ({ ...entry, active: true }));
}

/** Derive unavailable configured bot grants from the manager's complete matrix.
 * Broader rows describe configuration defaults; room rows describe local limits.
 * This never decides authorization: the admin API owns access and effective values.
 */
export function inactiveBotGrants(matrix: MatrixData): BotPermission[] {
  const cells = new Map(matrix.cells.map((cell) => [`${cell.scopeId}:${cell.permission}`, cell]));
  const scopes = new Map(matrix.scopes.map((scope) => [scope.id, scope]));
  const configured = (scope: MatrixScope, permission: string): boolean => {
    const ids =
      scope.kind === 'ROOM'
        ? [scope.id, `group:${scope.parentGroupId}`, 'server']
        : scope.kind === 'SERVER'
          ? ['server']
          : [scope.id, 'server'];
    for (const id of ids) {
      const decision = cells.get(`${id}:${permission}`)?.override;
      if (decision && decision !== 'NONE') return decision === 'ALLOW';
    }
    return false;
  };
  return matrix.cells
    .filter((cell) => {
      const scope = scopes.get(cell.scopeId);
      if (!scope || cell.effective === 'ALLOW') return false;
      if (!configured(scope, cell.permission)) return false;
      // Included read subsets add no useful second inactive line.
      return !getIncludingPermissions(matrix.applicablePermissions, cell.permission).some(
        (permission) =>
          configured(scope, permission) &&
          cells.get(`${scope.id}:${permission}`)?.effective !== 'ALLOW'
      );
    })
    .map((cell) => {
      const scope = scopes.get(cell.scopeId)!;
      const kind = scope.kind.toLowerCase() as BotPermission['scope'];
      return {
        permission: cell.permission,
        scope: kind,
        scopeId: kind === 'room' || kind === 'group' ? scope.id.slice(kind.length + 1) : '',
        scopeName: scope.label,
        active: false
      };
    });
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
