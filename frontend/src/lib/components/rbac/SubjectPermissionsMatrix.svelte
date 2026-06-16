<!--
@component

Presentational matrix used by both the per-user and per-role permissions
pages. Caller owns data loading and mutation dispatch; this component
just lays out the rows (permissions grouped by category) and columns
(server + groups + nested rooms), and forwards cell clicks via `onCycle`.

Cell semantics:
  - `override` ALLOW/DENY → solid (subject has an explicit grant/deny here)
  - `override` NONE        → faded, tinted by `effective` (the resolver's
                             baseline at this scope without an override)

A missing cell renders as an empty placeholder (the permission doesn't
apply at that scope's tier).
-->
<script lang="ts">
  import { Panel, DataTable } from '$lib/components/admin';
  import { Hint, HelpTooltip } from '$lib/ui';
  import { getPermissionDescription } from '$lib/permissions';
  import MatrixCell from './MatrixCell.svelte';

  export type MatrixDecision = 'ALLOW' | 'DENY' | 'NONE';
  export type MatrixScopeKind = 'SERVER' | 'GROUP' | 'ROOM';

  export type MatrixScope = {
    id: string;
    label: string;
    kind: MatrixScopeKind;
    parentGroupId: string;
  };
  export type MatrixCellData = {
    permission: string;
    scopeId: string;
    override: MatrixDecision;
    effective: MatrixDecision;
  };
  export type MatrixData = {
    applicablePermissions: string[];
    scopes: MatrixScope[];
    cells: MatrixCellData[];
  };
  export type CellState = 'allow' | 'deny' | 'neutral';

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
      description: 'Legacy space-level capabilities kept for compatibility during the server model transition'
    },
    room: {
      title: 'Room Permissions',
      description: 'Server defaults for room discovery and joining, plus room creation. Add room/group denies for local restrictions'
    },
    message: {
      title: 'Message Permissions',
      description: 'Server defaults for posting, threads, reactions, echoing, and message moderation. Room/group denies create local exceptions'
    },
    member: {
      title: 'Member Permissions',
      description: 'Control who can invite and remove space members'
    },
    role: {
      title: 'Role Permissions',
      description: 'Control who can edit roles, assign roles, and manage permission presets'
    },
    admin: {
      title: 'Admin Permissions',
      description: 'Control access to server-wide administrative views and diagnostics'
    },
    dm: { title: 'DM Permissions', description: 'Control access to direct messaging entry points' },
    user: {
      title: 'User Permissions',
      description: 'Control user account and per-user permission operations. Any non-owner deny cancels grants'
    }
  };

  let {
    data,
    updatingKey = null,
    onCycle,
    subjectKind = 'subject',
    forceAllow = false,
    readOnly = false,
    categoryOrder = DEFAULT_CATEGORY_ORDER
  }: {
    data: MatrixData;
    /** `${scopeId}::${permission}` of the cell whose mutation is in flight. */
    updatingKey?: string | null;
    onCycle: (scope: MatrixScope, permission: string, next: CellState) => void;
    /** Used in aria/title text — "user", "role", etc. */
    subjectKind?: string;
    /** Display every existing cell as allowed regardless of stored decisions. */
    forceAllow?: boolean;
    /** Disable cell mutation controls. */
    readOnly?: boolean;
    categoryOrder?: string[];
  } = $props();

  // ----- Column layout ----------------------------------------------------

  // Order columns: server first, then each group followed by its rooms.
  // Backend returns server, then all groups, then all rooms — we re-order
  // here so rooms nest visually under their parent group.
  const orderedScopes = $derived.by<MatrixScope[]>(() => {
    const server = data.scopes.filter((s) => s.kind === 'SERVER');
    const groups = data.scopes.filter((s) => s.kind === 'GROUP');
    const rooms = data.scopes.filter((s) => s.kind === 'ROOM');
    const out: MatrixScope[] = [...server];
    for (const g of groups) {
      out.push(g);
      const groupId = g.id.startsWith('group:') ? g.id.slice('group:'.length) : '';
      for (const r of rooms) {
        if (r.parentGroupId === groupId) out.push(r);
      }
    }
    const seen = new Set(out.map((s) => s.id));
    for (const r of rooms) {
      if (!seen.has(r.id)) out.push(r);
    }
    return out;
  });

  // ----- Row layout -------------------------------------------------------

  function categoryOf(permission: string): string {
    const dot = permission.indexOf('.');
    return dot > 0 ? permission.slice(0, dot) : permission;
  }

  const groupedPermissions = $derived.by(() => {
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

  // ----- Cell lookup ------------------------------------------------------

  const cellIndex = $derived.by(() => {
    // eslint-disable-next-line svelte/prefer-svelte-reactivity -- Map is ephemeral within derived computation
    const idx = new Map<string, MatrixCellData>();
    for (const cell of data.cells) {
      idx.set(`${cell.scopeId}|${cell.permission}`, cell);
    }
    return idx;
  });

  function cellFor(scopeId: string, permission: string): MatrixCellData | undefined {
    return cellIndex.get(`${scopeId}|${permission}`);
  }

  function decisionToState(d: MatrixDecision): CellState {
    if (d === 'ALLOW') return 'allow';
    if (d === 'DENY') return 'deny';
    return 'neutral';
  }

  function scopeColumnClass(kind: MatrixScopeKind): string {
    if (kind === 'SERVER') return 'bg-surface-200/40';
    if (kind === 'GROUP') return 'bg-surface-200/20';
    return '';
  }
</script>

{#if orderedScopes.length === 0}
  <Hint tone="info">No scopes available for this {subjectKind}.</Hint>
{:else}
  <div class="flex flex-col gap-6">
    {#each groupedPermissions as group (group.category)}
      {@const meta = CATEGORY_META[group.category]}
      {@const categoryScopes = orderedScopes.filter((scope) =>
        group.permissions.some((p) => cellFor(scope.id, p) !== undefined)
      )}
      <Panel title={meta?.title ?? group.category} subtitle={meta?.description} noPadding>
        <div class="overflow-x-auto" style="width: max-content; max-width: 100%">
          <DataTable
            items={group.permissions}
            columns={categoryScopes.length + 1}
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
              {#each categoryScopes as scope (scope.id)}
                <th
                  class={[
                    'px-0 py-3 text-center align-bottom font-medium',
                    scopeColumnClass(scope.kind)
                  ]}
                  style="width: 2rem; min-width: 2rem; height: 12rem"
                  title={`${scope.label} (${scope.kind.toLowerCase()})`}
                  data-scope={scope.id}
                >
                  <span
                    class={[
                      'text-sm',
                      scope.kind === 'SERVER' ? 'font-semibold' : '',
                      scope.kind === 'GROUP' ? 'text-primary' : '',
                      scope.kind === 'ROOM' ? 'text-muted' : ''
                    ]}
                    style="writing-mode: vertical-rl; transform: rotate(180deg); white-space: nowrap"
                  >
                    {#if scope.kind === 'ROOM'}#{/if}{scope.label}
                  </span>
                </th>
              {/each}
            {/snippet}
            {#snippet row(permission)}
              <td class="sticky left-0 z-10 bg-background px-4 py-2 whitespace-nowrap">
                <code data-testid="permission-name" class="text-sm">{permission}</code>
                <HelpTooltip label={`About ${permission}`}>
                  {getPermissionDescription(permission)}
                </HelpTooltip>
              </td>
              {#each categoryScopes as scope (scope.id)}
                {@const cell = cellFor(scope.id, permission)}
                {@const cellKey = `${scope.id}::${permission}`}
                {@const isUpdating = updatingKey === cellKey}
                <td
                  class={['px-0 py-2 text-center', scopeColumnClass(scope.kind)]}
                  style="width: 2rem; min-width: 2rem"
                  data-scope={scope.id}
                  data-permission={permission}
                >
                  {#if cell}
                    {@const ov = decisionToState(cell.override)}
                    {@const eff = decisionToState(cell.effective)}
                    {@const displayOverride = forceAllow ? 'allow' : ov}
                    {@const displayEffective = forceAllow ? 'neutral' : eff}
                    {@const ariaLabel = forceAllow
                      ? `${subjectKind} is always granted ${permission} at ${scope.label}`
                      : ov !== 'neutral'
                        ? `Override ${ov} for ${permission} at ${scope.label}`
                        : `No override for ${permission} at ${scope.label}, effective ${eff}`}
                    {@const titleParts = forceAllow
                      ? [
                          'Allow (owners are always granted all permissions)',
                          'Owner permissions are not editable'
                        ]
                      : [
                          ov !== 'neutral'
                            ? `${ov === 'allow' ? 'Allow' : 'Deny'} (${subjectKind} override at ${scope.label})`
                            : null,
                          ov === 'neutral' && eff !== 'neutral'
                            ? `Effective ${eff === 'allow' ? 'Allow' : 'Deny'} (inherited)`
                            : null,
                          ov === 'neutral' && eff === 'neutral' ? 'No decision' : null
                        ].filter(Boolean)}
                    <MatrixCell
                      override={displayOverride}
                      inherited={displayEffective}
                      updating={isUpdating}
                      disabled={readOnly}
                      {ariaLabel}
                      title={titleParts.join(' · ')}
                      onCycle={(next) => onCycle(scope, permission, next)}
                    />
                  {:else}
                    <span class="inline-block h-5 w-5" aria-hidden="true"></span>
                  {/if}
                </td>
              {/each}
            {/snippet}
          </DataTable>
        </div>
      </Panel>
    {/each}
  </div>
{/if}
