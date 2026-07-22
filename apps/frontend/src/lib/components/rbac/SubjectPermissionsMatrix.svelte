<!--
@component

Presentational matrix used by both the per-user and per-role permissions
pages. Caller owns data loading and mutation dispatch; this component
lays out alphabetically sorted permission rows and columns (server + groups +
nested rooms), and forwards cell clicks via `onCycle`.

Cell semantics:
  - `override` ALLOW/DENY → solid (subject has an explicit grant/deny here)
  - `override` NONE        → faded, tinted by `effective` (the resolver's
                             baseline at this scope without an override)

A missing cell renders as an empty placeholder (the permission doesn't
apply at that scope's tier). Hovering or focusing an available cell highlights
its permission row and scope column. The table header remains visible while
its dense matrix rows scroll.
-->
<script lang="ts">
  import { Panel, DataTable } from '$lib/components/admin';
  import { Hint, HelpTooltip } from '$lib/ui';
  import { ShortcutTextInput } from '$lib/ui/form';
  import { getPermissionDescription } from '$lib/permissions';
  import MatrixCell from './MatrixCell.svelte';
  import * as m from '$lib/i18n/messages';

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
  type MatrixCoordinate = { category: string; column: string; permission: string };

  let {
    data,
    updatingKey = null,
    onCycle,
    subjectKind = 'subject',
    forceAllow = false,
    readOnly = false
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
  } = $props();

  let hoveredCell = $state<MatrixCoordinate | null>(null);
  let focusedCell = $state<MatrixCoordinate | null>(null);
  const highlightedCell = $derived(hoveredCell ?? focusedCell);

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

  const permissions = $derived([...data.applicablePermissions].sort((a, b) => a.localeCompare(b)));
  let permissionFilter = $state('');
  const filteredPermissions = $derived.by(() => {
    const query = permissionFilter.trim().toLowerCase();
    return query ? permissions.filter((permission) => permission.toLowerCase().includes(query)) : permissions;
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

  const matrixScopes = $derived(
    orderedScopes.filter((scope) => permissions.some((permission) => cellFor(scope.id, permission)))
  );

  function decisionToState(d: MatrixDecision): CellState {
    if (d === 'ALLOW') return 'allow';
    if (d === 'DENY') return 'deny';
    return 'neutral';
  }

  function scopeColumnClass(kind: MatrixScopeKind): string {
    if (kind === 'SERVER') return 'bg-surface-emphasized/40';
    if (kind === 'GROUP') return 'bg-surface-emphasized/20';
    return '';
  }

  function coordinate(category: string, column: string, permission: string): MatrixCoordinate {
    return { category, column, permission };
  }

  function columnIsHighlighted(column: string): boolean {
    return highlightedCell?.column === column;
  }

  function rowIsHighlighted(category: string, permission: string): boolean {
    return highlightedCell?.category === category && highlightedCell.permission === permission;
  }

  function cellBackgroundClass(category: string, scope: MatrixScope, permission: string): string {
    const row = rowIsHighlighted(category, permission);
    const columnHighlighted = columnIsHighlighted(scope.id);
    if (row && columnHighlighted) return 'bg-action/15';
    if (row || columnHighlighted) return 'bg-action/8';
    return scopeColumnClass(scope.kind);
  }
</script>

{#if orderedScopes.length === 0}
  <Hint tone="info">No scopes available for this {subjectKind}.</Hint>
{:else}
  <Panel title={m['admin.permissions.title']()} noPadding>
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
      columns={matrixScopes.length + 2}
      getKey={(permission) => permission}
      emptyMessage={m['rbac.permissions.no_filter_matches']()}
      stickyHeader
      stickyHeaderFadeOffset="top-48"
      hoverable={false}
    >
          {#snippet header()}
            <th
              class="sticky left-0 z-10 bg-background px-4 py-3 text-left align-bottom font-medium"
              style="width: 14rem"
            >
              Permission
            </th>
            {#each matrixScopes as scope (scope.id)}
              <th
                class={[
                  'px-0 py-3 text-center align-bottom font-medium',
                  columnIsHighlighted(scope.id)
                    ? 'bg-action/10 text-action'
                    : scopeColumnClass(scope.kind)
                ]}
                style="width: 2rem; min-width: 2rem; height: 12rem"
                title={`${scope.label} (${scope.kind.toLowerCase()})`}
                data-scope={scope.id}
              >
                <span
                  class={[
                    'text-sm',
                    scope.kind === 'SERVER' ? 'font-semibold' : '',
                    scope.kind === 'GROUP' ? 'text-neutral-action' : '',
                    scope.kind === 'ROOM' ? 'text-muted' : ''
                  ]}
                  style="writing-mode: vertical-rl; transform: rotate(180deg); white-space: nowrap"
                >
                  {#if scope.kind === 'ROOM'}#{/if}{scope.label}
                </span>
              </th>
            {/each}
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
              <code
                data-testid="permission-name"
                class={[
                  'text-sm',
                  rowIsHighlighted(category, permission) ? 'text-action' : ''
                ]}>{permission}</code
              >
              <HelpTooltip label={`About ${permission}`}>
                {getPermissionDescription(permission)}
              </HelpTooltip>
            </td>
            {#each matrixScopes as scope (scope.id)}
              {@const cell = cellFor(scope.id, permission)}
              {@const cellKey = `${scope.id}::${permission}`}
              {@const isUpdating = updatingKey === cellKey}
              <td
                class={[
                  'px-0 py-2 text-center',
                  cellBackgroundClass(category, scope, permission)
                ]}
                style="width: 2.5rem; min-width: 2.5rem"
                data-scope={scope.id}
                data-permission={permission}
                onmouseenter={cell
                  ? () => (hoveredCell = coordinate(category, scope.id, permission))
                  : undefined}
                onmouseleave={cell ? () => (hoveredCell = null) : undefined}
                onfocusin={cell
                  ? () => (focusedCell = coordinate(category, scope.id, permission))
                  : undefined}
                onfocusout={cell ? () => (focusedCell = null) : undefined}
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
                  <span class="inline-block h-10 w-10" aria-hidden="true"></span>
                {/if}
              </td>
            {/each}
            <td class="w-full p-0" aria-hidden="true" data-testid="permission-matrix-spacer"></td>
          {/snippet}
    </DataTable>
  </Panel>
{/if}
