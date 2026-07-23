<!--
@component

Per-tier permission matrix. Rows are permissions, with category headers
between the corresponding groups; columns are roles applicable at the
requested scope. Each cell shows the override at this tier (saturated)
layered over the inherited baseline from above (faded). Clicking a cell cycles
`neutral → allow → deny → neutral`.

Scope is implied by which of `spaceId` / `roomId` are set:

  spaceId | roomId | matrix shows
  --------+--------+---------------------------------------------
  ∅       | ∅      | all instance roles, no inheritance
  set     | ∅      | space + instance roles at space scope, with
                     instance-tier inheritance for instance roles
  set     | set    | same role set at room scope, inheriting the
                     resolved space + instance state per role

The table viewport scrolls when there are too many rows or roles to fit unless
`scrollContents` is disabled for a page-owned scroll container. The role header
and first column (permission name) remain sticky in the contained variant. Column
headers are clickable when `onRoleClick` is provided
(routing to per-role detail pages owned by the parent route). Hovering or
focusing a cell highlights its permission row and role column.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import { Panel, DataTable } from '$lib/components/admin';
  import { Hint, HelpTooltip } from '$lib/ui';
  import { ShortcutTextInput } from '$lib/ui/form';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { createPermissionAPI } from '$lib/api-client/permissions';
  import { toast } from '$lib/ui/toast';
  import { getPermissionDescription } from '$lib/permissions';
  import { setRolePermission, type MutationScope } from './permissionMutations';
  import MatrixCell from './MatrixCell.svelte';
  import * as m from '$lib/i18n/messages';

  type State = 'allow' | 'deny' | 'neutral';
  type MatrixCoordinate = { category: string; column: string; permission: string };

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
  const CATEGORY_META: Record<string, { title: string; description: string }> = {
    space: {
      title: m['rbac.permissions.categories.space.title'](),
      description: m['rbac.permissions.categories.space.description']()
    },
    room: {
      title: m['rbac.permissions.categories.room.title'](),
      description: m['rbac.permissions.categories.room.description']()
    },
    message: {
      title: m['rbac.permissions.categories.message.title'](),
      description: m['rbac.permissions.categories.message.description']()
    },
    member: {
      title: m['rbac.permissions.categories.member.title'](),
      description: m['rbac.permissions.categories.member.description']()
    },
    role: {
      title: m['rbac.permissions.categories.role.title'](),
      description: m['rbac.permissions.categories.role.description']()
    },
    admin: {
      title: m['rbac.permissions.categories.admin.title'](),
      description: m['rbac.permissions.categories.admin.description']()
    },
    dm: {
      title: m['rbac.permissions.categories.dm.title'](),
      description: m['rbac.permissions.categories.dm.description']()
    },
    user: {
      title: m['rbac.permissions.categories.user.title'](),
      description: m['rbac.permissions.categories.user.description']()
    }
  };

  let {
    spaceId = null,
    roomId = null,
    groupId = null,
    onRoleClick,
    isRoleClickable,
    newRoleHref,
    subtitle,
    fillHeight = false,
    scrollContents = true
  }: {
    spaceId?: string | null;
    roomId?: string | null;
    /**
     * Set-scope editing (ADR-031). When provided, the matrix shows the
     * set's grants/denials per role with no inheritance. Mutually
     * exclusive with `roomId`.
     */
    groupId?: string | null;
    /**
     * Called when a column header is clicked. Used by the parent route to
     * navigate to the per-role detail page (metadata, delete, assigned
     * users). When omitted, headers render as inert text.
     */
    onRoleClick?: (role: TierRole) => void;
    /**
     * Per-role gate for header click. Return `false` to render the header
     * as plain text (e.g. when the viewer can't access the destination —
     * a role detail page requires server admin, which a server-scope
     * role.manage holder doesn't necessarily have). Defaults to `true`.
     */
    isRoleClickable?: (role: TierRole) => boolean;
    /** Optional create-role destination rendered as the final matrix column. */
    newRoleHref?: string;
    /** Optional panel subtitle for the requested permission scope. */
    subtitle?: string | Snippet;
    /** Fill the remaining height when this matrix is the page's primary content. */
    fillHeight?: boolean;
    /** Use a contained vertical viewport instead of flowing with the owning page. */
    scrollContents?: boolean;
  } = $props();

  const connection = useConnection();

  function permissionAPI() {
    const conn = connection();
    return createPermissionAPI({
      baseUrl: conn.connectBaseUrl,
      bearerToken: conn.bearerToken
    });
  }

  let data = $state<TierRoles | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let updating = $state<string[]>([]); // "{roleName}::{permission}" entries with mutations in flight
  let hoveredCell = $state<MatrixCoordinate | null>(null);
  let focusedCell = $state<MatrixCoordinate | null>(null);
  const highlightedCell = $derived(hoveredCell ?? focusedCell);

  $effect(() => {
    const s = spaceId ?? null;
    const rm = roomId ?? null;
    const st = groupId ?? null;
    void load(s, rm, st);
  });

  async function load(s: string | null, rm: string | null, st: string | null) {
    loading = true;
    error = null;

    let matrix: TierRoles | null = null;
    try {
      matrix = await permissionAPI().getRolePermissionTierMatrix({
        roomId: rm,
        groupId: st
      });
    } catch (err) {
      if (s !== (spaceId ?? null) || rm !== (roomId ?? null) || st !== (groupId ?? null)) {
        return;
      }
      loading = false;
      error = err instanceof Error ? err.message : String(err);
      return;
    }

    if (s !== (spaceId ?? null) || rm !== (roomId ?? null) || st !== (groupId ?? null)) {
      return;
    }

    loading = false;
    if (!matrix) {
      error = m['rbac.permissions.no_data']();
      return;
    }
    // Clone so we can safely apply optimistic updates.
    data = {
      applicablePermissions: [...matrix.applicablePermissions],
      roles: matrix.roles.map((r: TierRole) => ({
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

  const permissions = $derived.by<string[]>(() =>
    data ? [...data.applicablePermissions].sort((a, b) => a.localeCompare(b)) : []
  );
  let permissionFilter = $state('');
  const filteredPermissions = $derived.by(() => {
    const query = permissionFilter.trim().toLowerCase();
    return query
      ? permissions.filter((permission) => permission.toLowerCase().includes(query))
      : permissions;
  });
  const panelTitle = $derived(
    !spaceId && !roomId && !groupId ? CATEGORY_META.space.title : m['admin.permissions.title']()
  );

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

  function roleIsVirtualOwner(role: TierRole): boolean {
    return role.roleName === 'owner';
  }

  function coordinate(category: string, column: string, permission: string): MatrixCoordinate {
    return { category, column, permission };
  }

  function columnIsHighlighted(column: string): boolean {
    return highlightedCell?.column === column;
  }

  function roleColumnIsHighlighted(column: string): boolean {
    return highlightedCell?.column === column;
  }

  function rowIsHighlighted(category: string, permission: string): boolean {
    return highlightedCell?.category === category && highlightedCell.permission === permission;
  }

  function cellHighlightClass(category: string, column: string, permission: string): string {
    const row = rowIsHighlighted(category, permission);
    const columnHighlighted = columnIsHighlighted(column);
    if (row && columnHighlighted) return 'bg-action/15';
    if (row || columnHighlighted) return 'bg-action/8';
    return '';
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
    if (updating.includes(cellKey)) return;
    updating = [...updating, cellKey];
    error = null;

    const result = await setRolePermission(permissionAPI(), scopeFor(role), permission, next);
    if (result.error) {
      error = result.error;
      toast.error(result.error);
      updating = updating.filter((key) => key !== cellKey);
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
    updating = updating.filter((key) => key !== cellKey);
  }
</script>

{#if error}
  <Hint tone="danger">{error}</Hint>
{/if}

{#if loading}
  <div class="text-muted">{m['rbac.permissions.loading']()}</div>
{:else if !data || data.roles.length === 0}
  <Hint tone="info">{m['rbac.permissions.no_roles']()}</Hint>
{:else}
  {@const roles = [...data.roles].sort((a, b) => b.position - a.position)}
  {@const columnCount = roles.length + 2 + (newRoleHref ? 1 : 0)}
  <Panel title={panelTitle} {subtitle} {fillHeight} noPadding>
    {#snippet actions()}
      <div class="w-48 sm:w-64">
        <ShortcutTextInput
          id="permission-filter"
          testid="permission-filter"
          label={m['rbac.permissions.filter_label']()}
          labelHidden
          shortcutKey="/"
          placeholder={m['rbac.permissions.filter_placeholder']()}
          leadingIcon="iconify uil--search"
          autocomplete="off"
          bind:value={permissionFilter}
        />
      </div>
    {/snippet}
    <DataTable
      items={filteredPermissions}
      columns={columnCount}
      getKey={(permission) => permission}
      emptyMessage={m['rbac.permissions.no_filter_matches']()}
      stickyHeader={scrollContents}
      {fillHeight}
      stickyHeaderFadeOffset="top-48"
      hoverable={false}
    >
      {#snippet header()}
        <th
          class="sticky left-0 z-10 bg-background px-4 py-3 text-left align-bottom font-medium"
          style="width: 14rem"
        >
          {m['rbac.permissions.permission']()}
        </th>
        {#each roles as role (role.roleName)}
          {@const handle =
            onRoleClick && (isRoleClickable ? isRoleClickable(role) : true)
              ? onRoleClick
              : undefined}
          <th
            class={[
              'px-0 py-3 text-center align-bottom font-medium',
              roleColumnIsHighlighted(role.roleName) ? 'bg-action/10 text-action' : 'bg-background'
            ]}
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
        {#if newRoleHref}
          <th
            class="bg-background px-0 py-3 text-center align-bottom font-medium"
            style="width: 2rem; min-width: 2rem; height: 12rem"
          >
            <!-- eslint-disable svelte/no-navigation-without-resolve -- newRoleHref is resolved by the owning route -->
            <a
              href={newRoleHref}
              class="cursor-pointer text-sm font-medium text-action hover:underline"
              style="writing-mode: vertical-rl; transform: rotate(180deg); white-space: nowrap"
              data-testid="new-role-column"
            >
              {m['admin.permissions.new_role_action']()}
            </a>
            <!-- eslint-enable svelte/no-navigation-without-resolve -->
          </th>
        {/if}
        <th class="w-full bg-background p-0" aria-hidden="true"></th>
      {/snippet}
      {#snippet row(permission)}
        {@const category = categoryOf(permission)}
        <td
          class={[
            'sticky left-0 z-10 px-4 py-2 whitespace-nowrap',
            rowIsHighlighted(category, permission) ? 'bg-action/8' : 'bg-background'
          ]}
        >
          <div class="flex items-center gap-2">
            <HelpTooltip label={`About ${permission}`}>
              {getPermissionDescription(permission)}
            </HelpTooltip>
            <code
              data-testid="permission-name"
              class={['text-sm', rowIsHighlighted(category, permission) ? 'text-action' : '']}
              >{permission}</code
            >
          </div>
        </td>
        {#each roles as role (role.roleName)}
          {@const ov = overrideState(role, permission)}
          {@const inh = inheritedState(role, permission)}
          {@const virtualOwner = roleIsVirtualOwner(role)}
          {@const displayOverride = virtualOwner ? 'allow' : ov}
          {@const displayInherited = virtualOwner ? 'neutral' : inh}
          {@const cellKey = `${role.roleName}::${permission}`}
          {@const isUpdating = updating.includes(cellKey)}
          {@const ariaParts = virtualOwner
            ? [`Owner is always granted ${permission}`]
            : [
                ov !== 'neutral'
                  ? `Override ${ov} for ${role.displayName} on ${permission}`
                  : `No override for ${role.displayName} on ${permission}`,
                inh !== 'neutral' && inheritedFromLabel
                  ? `inheriting ${inh} from ${inheritedFromLabel}`
                  : null
              ].filter(Boolean)}
          {@const ariaLabel = ariaParts.join(', ')}
          {@const titleParts = virtualOwner
            ? [
                'Allow (owners are always granted all permissions)',
                'Owner permissions are not editable'
              ]
            : [
                ov !== 'neutral'
                  ? `${ov === 'allow' ? 'Allow' : 'Deny'} (override at this tier)`
                  : null,
                inh !== 'neutral' && inheritedFromLabel
                  ? `Inherits ${inh === 'allow' ? 'Allow' : 'Deny'} from ${inheritedFromLabel}`
                  : null,
                ov === 'neutral' && inh === 'neutral' ? 'No decision' : null
              ].filter(Boolean)}
          <td
            class={[
              'px-0 py-2 text-center',
              cellHighlightClass(category, role.roleName, permission)
            ]}
            style="width: 2.5rem; min-width: 2.5rem"
            data-role={role.roleName}
            data-permission={permission}
            onmouseenter={() => (hoveredCell = coordinate(category, role.roleName, permission))}
            onmouseleave={() => (hoveredCell = null)}
            onfocusin={() => (focusedCell = coordinate(category, role.roleName, permission))}
            onfocusout={() => (focusedCell = null)}
          >
            <MatrixCell
              override={displayOverride}
              inherited={displayInherited}
              updating={isUpdating}
              disabled={virtualOwner}
              {ariaLabel}
              title={titleParts.join(' · ')}
              onCycle={(next) => void cycle(role, permission, next)}
            />
          </td>
        {/each}
        {#if newRoleHref}
          <td class="px-0 py-2" style="width: 2.5rem; min-width: 2.5rem" aria-hidden="true"></td>
        {/if}
        <td class="w-full p-0" aria-hidden="true" data-testid="permission-matrix-spacer"></td>
      {/snippet}
    </DataTable>
  </Panel>
{/if}
