<!--
@component

Per-tier permission matrix. Rows are permissions (grouped by category, one
`Panel` per category); columns are roles applicable at the requested
scope. Each cell shows the override at this tier (saturated) layered over
the inherited baseline from above (faded). Clicking a cell cycles
`neutral → allow → deny → neutral`.

Scope is implied by which of `spaceId` / `roomId` are set:

  spaceId | roomId | matrix shows
  --------+--------+---------------------------------------------
  ∅       | ∅      | all instance roles, no inheritance
  set     | ∅      | space + instance roles at space scope, with
                     instance-tier inheritance for instance roles
  set     | set    | same role set at room scope, inheriting the
                     resolved space + instance state per role

The container scrolls horizontally when there are too many roles to fit;
the first column (permission name) is sticky so role columns can scroll
under it. Column headers are clickable when `onRoleClick` is provided
(routing to per-role detail pages owned by the parent route).
-->
<script lang="ts">
  import { Panel, DataTable } from '$lib/components/admin';
  import { Hint } from '$lib/ui';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { graphql } from '$lib/gql';
  import { toast } from '$lib/ui/toast';
  import { getPermissionDisplayName } from '$lib/permissions';
  import { setRolePermission, type MutationScope } from './permissionMutations';
  import MatrixCell from './MatrixCell.svelte';

  type State = 'allow' | 'deny' | 'neutral';

  type TierPerms = { permissions: string[]; permissionDenials: string[] };
  type TierRole = {
    roleName: string;
    displayName: string;
    description: string;
    isSystem: boolean;
    position: number;
    override: TierPerms;
    inheritedAllows: string[];
    inheritedDenials: string[];
  };
  type TierRoles = {
    applicablePermissions: string[];
    roles: TierRole[];
  };

  const DEFAULT_CATEGORY_ORDER = [
    'space',
    'room',
    'message',
    'member',
    'role',
    'admin',
    'dm',
    'user'
  ];

  const CATEGORY_META: Record<string, { title: string; description: string }> = {
    space: {
      title: 'Space Permissions',
      description: 'Control who can browse, create, join, and manage spaces'
    },
    room: {
      title: 'Room Permissions',
      description: 'Control who can create, join, and manage rooms'
    },
    message: {
      title: 'Message Permissions',
      description: 'Control what users can do with messages'
    },
    member: {
      title: 'Member Permissions',
      description: 'Control who can invite and remove space members'
    },
    role: {
      title: 'Role Permissions',
      description: 'Control who can create roles and assign them to users'
    },
    admin: {
      title: 'Admin Permissions',
      description: 'Access to server-wide admin functions'
    },
    dm: { title: 'DM Permissions', description: 'Control access to direct messaging' },
    user: { title: 'User Permissions', description: 'Control user account operations' }
  };

  let {
    spaceId = null,
    roomId = null,
    groupId = null,
    categoryOrder = DEFAULT_CATEGORY_ORDER,
    onRoleClick,
    isRoleClickable
  }: {
    spaceId?: string | null;
    roomId?: string | null;
    /**
     * Set-scope editing (ADR-031). When provided, the matrix shows the
     * set's grants/denials per role with no inheritance. Mutually
     * exclusive with `roomId`.
     */
    groupId?: string | null;
    categoryOrder?: string[];
    /**
     * Called when a column header is clicked. Used by the parent route to
     * navigate to the per-role detail page (metadata, delete, assigned
     * users). When omitted, headers render as inert text.
     */
    onRoleClick?: (role: TierRole) => void;
    /**
     * Per-role gate for header click. Return `false` to render the header
     * as plain text (e.g. when the viewer can't access the destination —
     * a role detail page requires server admin, which a space role.manage
     * holder doesn't necessarily have). Defaults to `true`.
     */
    isRoleClickable?: (role: TierRole) => boolean;
  } = $props();

  const connection = useConnection();

  let data = $state<TierRoles | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let updating = $state<string | null>(null); // "{roleName}::{permission}" while a mutation is in flight

  $effect(() => {
    const s = spaceId ?? null;
    const rm = roomId ?? null;
    const st = groupId ?? null;
    void load(s, rm, st);
  });

  async function load(s: string | null, rm: string | null, st: string | null) {
    loading = true;
    error = null;

    const resp = await connection().client.query(
      graphql(`
        query MatrixTierRoles($roomId: ID, $groupId: ID) {
          tierRoles(roomId: $roomId, groupId: $groupId) {
            applicablePermissions
            roles {
              roleName
              displayName
              description
              isSystem
              position
              override {
                permissions
                permissionDenials
              }
              inheritedAllows
              inheritedDenials
            }
          }
        }
      `),
      { roomId: rm ?? undefined, groupId: st ?? undefined }
    );

    if (
      s !== (spaceId ?? null) ||
      rm !== (roomId ?? null) ||
      st !== (groupId ?? null)
    ) {
      return;
    }

    loading = false;
    if (resp.error) {
      error = resp.error.message;
      return;
    }
    if (!resp.data?.tierRoles) {
      error = 'No data returned';
      return;
    }
    // Clone so we can safely apply optimistic updates.
    data = {
      applicablePermissions: [...resp.data.tierRoles.applicablePermissions],
      roles: resp.data.tierRoles.roles.map((r: TierRole) => ({
        ...r,
        override: {
          permissions: [...r.override.permissions],
          permissionDenials: [...r.override.permissionDenials]
        },
        inheritedAllows: [...r.inheritedAllows],
        inheritedDenials: [...r.inheritedDenials]
      }))
    };
  }

  // ----- Layout -----------------------------------------------------------

  function categoryOf(permission: string): string {
    const dot = permission.indexOf('.');
    return dot > 0 ? permission.slice(0, dot) : permission;
  }

  const groupedPermissions = $derived.by(() => {
    if (!data) return [];
    // eslint-disable-next-line svelte/prefer-svelte-reactivity -- Map is ephemeral within derived computation
    const groups = new Map<string, string[]>();
    for (const p of data.applicablePermissions) {
      const cat = categoryOf(p);
      if (!groups.has(cat)) groups.set(cat, []);
      groups.get(cat)!.push(p);
    }
    for (const arr of groups.values()) arr.sort((a, b) => a.localeCompare(b));
    const out: Array<{ category: string; permissions: string[] }> = [];
    for (const cat of categoryOrder) {
      const arr = groups.get(cat);
      if (arr && arr.length) out.push({ category: cat, permissions: arr });
    }
    for (const [cat, arr] of groups) {
      if (!categoryOrder.includes(cat) && arr.length) out.push({ category: cat, permissions: arr });
    }
    return out;
  });

  const inheritedFromLabel = $derived.by(() => {
    if (roomId) return 'space';
    if (spaceId) return 'instance';
    return null;
  });

  // ----- State accessors --------------------------------------------------

  function overrideState(role: TierRole, permission: string): State {
    if (role.override.permissions.includes(permission)) return 'allow';
    if (role.override.permissionDenials.includes(permission)) return 'deny';
    return 'neutral';
  }

  function inheritedState(role: TierRole, permission: string): State {
    if (role.inheritedAllows.includes(permission)) return 'allow';
    if (role.inheritedDenials.includes(permission)) return 'deny';
    return 'neutral';
  }

  // ----- Mutations --------------------------------------------------------

  function scopeFor(role: TierRole): MutationScope {
    if (groupId) {
      return { tier: 'group', roleName: role.roleName, groupId };
    }
    if (roomId) {
      return { tier: 'room', roleName: role.roleName, roomId };
    }
    return { tier: 'server', roleName: role.roleName };
  }

  async function cycle(role: TierRole, permission: string, next: State) {
    if (!data) return;
    const cellKey = `${role.roleName}::${permission}`;
    updating = cellKey;
    error = null;

    const result = await setRolePermission(
      connection().client,
      scopeFor(role),
      permission,
      next
    );
    if (result.error) {
      error = result.error;
      toast.error(result.error);
      updating = null;
      return;
    }

    // Optimistic update on the cell's role.
    role.override.permissions = role.override.permissions.filter((p) => p !== permission);
    role.override.permissionDenials = role.override.permissionDenials.filter(
      (p) => p !== permission
    );
    if (next === 'allow') {
      role.override.permissions = [...role.override.permissions, permission];
    } else if (next === 'deny') {
      role.override.permissionDenials = [...role.override.permissionDenials, permission];
    }
    updating = null;
  }
</script>

{#if error}
  <Hint tone="danger">{error}</Hint>
{/if}

{#if loading}
  <div class="text-muted">Loading permissions…</div>
{:else if !data || data.roles.length === 0}
  <Hint tone="info">No roles applicable at this scope.</Hint>
{:else}
  {@const roles = [...data.roles].sort((a, b) => b.position - a.position)}
  <div class="flex flex-col gap-6">
    {#each groupedPermissions as group (group.category)}
      {@const meta = CATEGORY_META[group.category]}
      <Panel title={meta?.title ?? group.category} subtitle={meta?.description} noPadding>
        <div class="overflow-x-auto" style="width: max-content; max-width: 100%">
          <DataTable
            items={group.permissions}
            columns={roles.length + 1}
            getKey={(p) => p}
            emptyMessage="No permissions in this category"
            hoverable={false}
          >
            {#snippet header()}
              <th
                class="sticky left-0 z-10 bg-background px-4 py-3 text-left align-bottom font-medium"
                style="width: 14rem"
              >
                Permission
              </th>
              {#each roles as role (role.roleName)}
                {@const handle =
                  onRoleClick && (isRoleClickable ? isRoleClickable(role) : true)
                    ? onRoleClick
                    : undefined}
                <th
                  class="px-0 py-3 text-center align-bottom font-medium"
                  style="width: 2rem; min-width: 2rem; height: 12rem"
                  title={`${role.displayName} — click to manage`}
                  data-role={role.roleName}
                >
                  {#if handle}
                    <button
                      type="button"
                      class="cursor-pointer text-sm hover:underline"
                      onclick={() => handle(role)}
                      style="writing-mode: vertical-rl; transform: rotate(180deg); white-space: nowrap"
                    >
                      @{role.roleName}
                    </button>
                  {:else}
                    <span
                      class="text-sm"
                      style="writing-mode: vertical-rl; transform: rotate(180deg); white-space: nowrap"
                    >
                      @{role.roleName}
                    </span>
                  {/if}
                </th>
              {/each}
            {/snippet}
            {#snippet row(permission)}
              <td class="sticky left-0 z-10 bg-background px-4 py-2">
                <div data-testid="permission-name">{getPermissionDisplayName(permission)}</div>
                <div class="text-xs text-muted/70">{permission}</div>
              </td>
              {#each roles as role (role.roleName)}
                {@const ov = overrideState(role, permission)}
                {@const inh = inheritedState(role, permission)}
                {@const cellKey = `${role.roleName}::${permission}`}
                {@const isUpdating = updating === cellKey}
                {@const ariaParts = [
                  ov !== 'neutral'
                    ? `Override ${ov} for ${role.displayName} on ${permission}`
                    : `No override for ${role.displayName} on ${permission}`,
                  inh !== 'neutral' && inheritedFromLabel
                    ? `inheriting ${inh} from ${inheritedFromLabel}`
                    : null
                ].filter(Boolean)}
                {@const ariaLabel = ariaParts.join(', ')}
                {@const titleParts = [
                  ov !== 'neutral'
                    ? `${ov === 'allow' ? 'Allow' : 'Deny'} (override at this tier)`
                    : null,
                  inh !== 'neutral' && inheritedFromLabel
                    ? `Inherits ${inh === 'allow' ? 'Allow' : 'Deny'} from ${inheritedFromLabel}`
                    : null,
                  ov === 'neutral' && inh === 'neutral' ? 'No decision' : null
                ].filter(Boolean)}
                <td
                  class="px-0 py-2 text-center"
                  style="width: 2rem; min-width: 2rem"
                  data-role={role.roleName}
                  data-permission={permission}
                >
                  <MatrixCell
                    override={ov}
                    inherited={inh}
                    updating={isUpdating}
                    {ariaLabel}
                    title={titleParts.join(' · ')}
                    onCycle={(next) => void cycle(role, permission, next)}
                  />
                </td>
              {/each}
            {/snippet}
          </DataTable>
        </div>
      </Panel>
    {/each}
  </div>
{/if}
