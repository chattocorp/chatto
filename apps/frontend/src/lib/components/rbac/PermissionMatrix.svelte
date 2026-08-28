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
  import { onDestroy, type Snippet } from 'svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import { MatrixColumnHeading, MatrixTable } from '$lib/components/matrix';
  import { Hint, HelpTooltip } from '$lib/ui';
  import { ShortcutTextInput } from '$lib/ui/form';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createPermissionAPI } from '$lib/api-client/permissions';
  import { toast } from '$lib/ui/toast';
  import { getIncludedByPermission, getPermissionDescription } from '$lib/permissions';
  import { setRolePermission, type MutationScope } from './permissionMutations';
  import MatrixCell from './MatrixCell.svelte';
  import { m } from '$lib/i18n/messages';
  import { createQuery } from '@tanstack/svelte-query';
  import { adminQueryKeys } from '$lib/query/admin';
  import { queryClient } from '$lib/query/client';
  import { invalidateRolePermissionDependents } from '$lib/query/adminInvalidation';

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
  const CATEGORY_META: Record<string, { title: string; description: string }> = {
    space: {
      title: m('rbac.permissions.categories.space.title'),
      description: m('rbac.permissions.categories.space.description')
    },
    room: {
      title: m('rbac.permissions.categories.room.title'),
      description: m('rbac.permissions.categories.room.description')
    },
    message: {
      title: m('rbac.permissions.categories.message.title'),
      description: m('rbac.permissions.categories.message.description')
    },
    member: {
      title: m('rbac.permissions.categories.member.title'),
      description: m('rbac.permissions.categories.member.description')
    },
    role: {
      title: m('rbac.permissions.categories.role.title'),
      description: m('rbac.permissions.categories.role.description')
    },
    admin: {
      title: m('rbac.permissions.categories.admin.title'),
      description: m('rbac.permissions.categories.admin.description')
    },
    dm: {
      title: m('rbac.permissions.categories.dm.title'),
      description: m('rbac.permissions.categories.dm.description')
    },
    user: {
      title: m('rbac.permissions.categories.user.title'),
      description: m('rbac.permissions.categories.user.description')
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

  const serverScope = useServerScope();

  const matrixQuery = createQuery(
    () => {
      const serverId = serverScope.serverId;
      const activeConnection = serverScope.connection;
      const activeRoomId = roomId ?? null;
      const activeGroupId = groupId ?? null;
      return {
        queryKey: adminQueryKeys.permissionTier(
          serverId,
          activeConnection,
          activeRoomId,
          activeGroupId
        ),
        queryFn: ({ signal }) =>
          activeConnection
            .getAPI(createPermissionAPI)
            .getRolePermissionTierMatrix(
              { roomId: activeRoomId, groupId: activeGroupId },
              { signal }
            )
      };
    },
    () => queryClient
  );

  const data = $derived(matrixQuery.data ?? null);
  const loading = $derived(matrixQuery.isPending);
  const loadError = $derived(matrixQuery.error instanceof Error ? matrixQuery.error.message : null);
  let mutationError = $state<{ context: string; message: string } | null>(null);
  let updating = $state<string[]>([]);
  let disposed = false;
  const activeMutationContext = $derived(
    mutationContext(
      serverScope.serverId,
      serverScope.connection.queryScope,
      spaceId ?? null,
      roomId ?? null,
      groupId ?? null
    )
  );
  const visibleMutationError = $derived(
    mutationError?.context === activeMutationContext ? mutationError.message : null
  );
  onDestroy(() => {
    disposed = true;
  });

  // ----- Layout -----------------------------------------------------------

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
    !spaceId && !roomId && !groupId ? CATEGORY_META.space.title : m('admin.permissions.title')
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

  function exactInheritedState(role: TierRole, permission: string): State {
    if (role.inheritedAllows.includes(permission)) return 'allow';
    if (role.inheritedDenials.includes(permission)) return 'deny';
    return 'neutral';
  }

  function includingPermission(role: TierRole, permission: string): string | null {
    const including = getIncludedByPermission(permission);
    if (!including) return null;
    const includingOverride = overrideState(role, including);
    if (includingOverride === 'allow') return including;
    if (includingOverride === 'deny') return null;
    return exactInheritedState(role, including) === 'allow' ? including : null;
  }

  function inheritedState(role: TierRole, permission: string): State {
    if (includingPermission(role, permission)) return 'allow';
    return exactInheritedState(role, permission);
  }

  function roleIsVirtualOwner(role: TierRole): boolean {
    return role.roleName === 'owner';
  }

  function mutationContext(
    serverId: string,
    queryScope: string,
    activeSpaceId: string | null,
    activeRoomId: string | null,
    activeGroupId: string | null
  ): string {
    return JSON.stringify([serverId, queryScope, activeSpaceId, activeRoomId, activeGroupId]);
  }

  function cellIsUpdating(cellKey: string): boolean {
    return updating.includes(`${activeMutationContext}:${cellKey}`);
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
    const serverId = serverScope.serverId;
    const activeConnection = serverScope.connection;
    const queryKey = adminQueryKeys.permissionTier(
      serverId,
      activeConnection,
      roomId ?? null,
      groupId ?? null
    );
    const mutationScope = scopeFor(role);
    const cellKey = `${role.roleName}::${permission}`;
    const context = mutationContext(
      serverId,
      activeConnection.queryScope,
      spaceId ?? null,
      roomId ?? null,
      groupId ?? null
    );
    const pendingKey = `${context}:${cellKey}`;
    if (updating.includes(pendingKey)) return;
    updating = [...updating, pendingKey];
    mutationError = null;

    const result = await setRolePermission(
      activeConnection.getAPI(createPermissionAPI),
      mutationScope,
      permission,
      next
    );
    if (disposed || !serverScope.isCurrent()) return;
    if (result.error) {
      if (context === activeMutationContext) {
        mutationError = { context, message: result.error };
        toast.error(result.error);
      }
      updating = updating.filter((key) => key !== pendingKey);
      return;
    }

    queryClient.setQueryData<TierRoles>(queryKey, (old) => {
      if (!old) return old;
      return {
        ...old,
        roles: old.roles.map((candidate) => {
          if (candidate.roleName !== role.roleName) return candidate;
          const permissions = candidate.override.permissions.filter((p) => p !== permission);
          const permissionDenials = candidate.override.permissionDenials.filter(
            (p) => p !== permission
          );
          if (next === 'allow') permissions.push(permission);
          if (next === 'deny') permissionDenials.push(permission);
          return { ...candidate, override: { permissions, permissionDenials } };
        })
      };
    });
    void queryClient.invalidateQueries({
      queryKey: adminQueryKeys.rolePermissions(serverId, activeConnection, role.roleName)
    });
    invalidateRolePermissionDependents(serverId, activeConnection, role.roleName);
    updating = updating.filter((key) => key !== pendingKey);
  }
</script>

{#if visibleMutationError || loadError}
  <Hint tone="danger">{visibleMutationError ?? loadError}</Hint>
{/if}

{#if loading}
  <div class="text-muted">{m('rbac.permissions.loading')}</div>
{:else if !data || data.roles.length === 0}
  <Hint tone="info">{m('rbac.permissions.no_roles')}</Hint>
{:else}
  {@const roles = [...data.roles].sort((a, b) => b.position - a.position)}
  <Panel title={panelTitle} {subtitle} {fillHeight} noPadding>
    {#snippet actions()}
      <div class="w-48 sm:w-64">
        <ShortcutTextInput
          id="permission-filter"
          testid="permission-filter"
          label={m('rbac.permissions.filter_label')}
          labelHidden
          shortcutKey="/"
          placeholder={m('rbac.permissions.filter_placeholder')}
          leadingIcon="iconify icon-[uil--search]"
          autocomplete="off"
          bind:value={permissionFilter}
        />
      </div>
    {/snippet}
    <MatrixTable
      rows={filteredPermissions}
      columns={roles}
      getRowKey={(permission) => permission}
      getColumnKey={(role) => role.roleName}
      emptyMessage={m('rbac.permissions.no_filter_matches')}
      stickyHeader={scrollContents}
      {fillHeight}
      stickyHeaderFadeOffset="top-48"
      trailingColumns={newRoleHref ? 1 : 0}
      spacerTestId="permission-matrix-spacer"
      columnAttributes={(role) => ({ 'data-role': role.roleName })}
      cellAttributes={(permission, role) => ({
        'data-role': role.roleName,
        'data-permission': permission
      })}
    >
      {#snippet leadingHeader()}
        {m('rbac.permissions.permission')}
      {/snippet}
      {#snippet columnHeader(role, highlighted)}
        {@const handle =
          onRoleClick && (isRoleClickable ? isRoleClickable(role) : true) ? onRoleClick : undefined}
        {#if handle}
          <button
            type="button"
            class={['cursor-pointer hover:underline', highlighted ? 'text-action' : '']}
            onclick={() => handle(role)}
            title={`${role.displayName} — click to manage`}
          >
            @{role.roleName}
          </button>
        {:else}
          <span class={highlighted ? 'text-action' : ''}>@{role.roleName}</span>
        {/if}
      {/snippet}
      {#snippet trailingHeader()}
        {#if newRoleHref}
          <th
            class="bg-background px-0 py-3 text-center align-bottom font-medium"
            style="width: 2rem; min-width: 2rem; height: 12rem"
          >
            <MatrixColumnHeading>
              <!-- eslint-disable svelte/no-navigation-without-resolve -- newRoleHref is resolved by the owning route -->
              <a
                href={newRoleHref}
                class="cursor-pointer font-medium text-action hover:underline"
                data-testid="new-role-column"
              >
                {m('admin.permissions.new_role_action')}
              </a>
              <!-- eslint-enable svelte/no-navigation-without-resolve -->
            </MatrixColumnHeading>
          </th>
        {/if}
      {/snippet}
      {#snippet rowHeader(permission, highlighted)}
        {@const includedBy = getIncludedByPermission(permission)}
        <div class={['flex items-center gap-2', includedBy ? 'ml-4' : '']}>
          <HelpTooltip label={`About ${permission}`}>
            {getPermissionDescription(permission)}
            {#if includedBy}
              <span class="mt-1 block">
                {m('rbac.permissions.included_by', { permission: includedBy })}
              </span>
            {/if}
          </HelpTooltip>
          <code data-testid="permission-name" class={['text-sm', highlighted ? 'text-action' : '']}
            >{permission}</code
          >
        </div>
      {/snippet}
      {#snippet cell(permission, role)}
        {@const ov = overrideState(role, permission)}
        {@const inh = inheritedState(role, permission)}
        {@const includedBy = includingPermission(role, permission)}
        {@const virtualOwner = roleIsVirtualOwner(role)}
        {@const displayOverride = virtualOwner ? 'allow' : ov}
        {@const displayInherited = virtualOwner ? 'neutral' : inh}
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
              includedBy ? `Effective Allow (included by ${includedBy})` : null,
              ov === 'neutral' && inh === 'neutral' ? 'No decision' : null
            ].filter(Boolean)}
        <MatrixCell
          override={displayOverride}
          inherited={displayInherited}
          updating={cellIsUpdating(`${role.roleName}::${permission}`)}
          disabled={virtualOwner}
          {ariaLabel}
          title={titleParts.join(' · ')}
          onCycle={(next) => void cycle(role, permission, next)}
        />
      {/snippet}
      {#snippet trailingCell()}
        {#if newRoleHref}
          <td class="px-0 py-2" style="width: 2.5rem; min-width: 2.5rem" aria-hidden="true"></td>
        {/if}
      {/snippet}
    </MatrixTable>
  </Panel>
{/if}
