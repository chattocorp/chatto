<!--
@component

Presentational matrix used by both the per-user and per-role permissions
pages. Caller owns data loading and mutation dispatch; this component groups
permission rows by category, sorts them by their stable IDs, lays out columns
(server + groups + nested rooms), and forwards cell clicks via `onCycle`.

Cell semantics:
  - `override` ALLOW/DENY → solid (subject has an explicit grant/deny here)
  - `override` NONE        → faded, tinted by `effective` (the resolver's
                             baseline at this scope without an override)

A missing cell renders as an empty placeholder (the permission doesn't
apply at that scope's tier). Hovering or focusing an available cell highlights
its permission row and scope column. The surrounding pane owns vertical
scrolling; the table only scrolls horizontally when its columns overflow.
-->
<script lang="ts">
  import Panel from '$lib/ui/Panel.svelte';
  import { MatrixTable } from '$lib/ui/matrix';
  import { Hint } from '$lib/ui';
  import { ShortcutTextInput } from '$lib/ui/form';
  import {
    getIncludingPermissions,
    getPermissionCategory,
    getPermissionCategoryLabel,
    getPermissionDescription
  } from '$lib/permissions';
  import MatrixCell from './MatrixCell.svelte';
  import { m } from '$lib/i18n/messages';

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
    /** Present when a delegation ceiling can prevent storing an allow. */
    allowPermitted?: boolean;
  };
  export type MatrixData = {
    applicablePermissions: string[];
    scopes: MatrixScope[];
    cells: MatrixCellData[];
  };
  export type CellState = 'allow' | 'deny' | 'neutral';
  export type DecisionMode = 'tri-state' | 'binary';

  let {
    data,
    updatingKey = null,
    onCycle,
    subjectKind = 'subject',
    forceAllow = false,
    readOnly = false,
    decisionMode = 'tri-state'
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
    /** Use a grant-or-absent allowlist UI; inherited grants are read-only. */
    decisionMode?: DecisionMode;
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

  const permissions = $derived([...data.applicablePermissions].sort());
  const inclusionChains = $derived.by(() => {
    // eslint-disable-next-line svelte/prefer-svelte-reactivity -- Map is ephemeral within derived computation
    const chains = new Map<string, string[]>();
    for (const permission of permissions) {
      chains.set(permission, getIncludingPermissions(permissions, permission));
    }
    return chains;
  });
  let permissionFilter = $state('');
  const filteredPermissions = $derived.by(() => {
    const query = permissionFilter.trim().toLowerCase();
    return query
      ? permissions.filter(
          (permission) =>
            permission.toLowerCase().includes(query) ||
            getPermissionDescription(permission).toLowerCase().includes(query)
        )
      : permissions;
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

  function parentDecision(scope: MatrixScope, permission: string): MatrixDecision {
    const serverScope = data.scopes.find((candidate) => candidate.kind === 'SERVER');
    const serverDecision = serverScope
      ? (cellFor(serverScope.id, permission)?.override ?? 'NONE')
      : 'NONE';
    if (scope.kind === 'SERVER') return 'NONE';
    if (scope.kind === 'GROUP') return serverDecision;

    const groupScope = data.scopes.find(
      (candidate) => candidate.kind === 'GROUP' && candidate.id === `group:${scope.parentGroupId}`
    );
    const groupDecision = groupScope
      ? (cellFor(groupScope.id, permission)?.override ?? 'NONE')
      : 'NONE';
    return groupDecision !== 'NONE' ? groupDecision : serverDecision;
  }

  function includingPermission(scope: MatrixScope, permission: string): string | null {
    for (const including of inclusionChains.get(permission) ?? []) {
      if (cellFor(scope.id, including)?.effective === 'ALLOW') return including;
    }
    return null;
  }

  function cycleCell(
    scope: MatrixScope,
    permission: string,
    current: MatrixDecision,
    next: CellState
  ) {
    if (decisionMode !== 'binary') {
      onCycle(scope, permission, next);
      return;
    }
    // Binary bot edits write only an explicit grant or no decision. Older
    // clients could create denies, so clicking one clears it instead of
    // replacing it with a grant.
    onCycle(scope, permission, next === 'allow' && current !== 'DENY' ? 'allow' : 'neutral');
  }

  function scopeColumnClass(kind: MatrixScopeKind): string {
    if (kind === 'SERVER') return 'bg-surface-emphasized/40';
    if (kind === 'GROUP') return 'bg-surface-emphasized/20';
    return '';
  }
</script>

{#if orderedScopes.length === 0}
  <Hint tone="info">No scopes available for this {subjectKind}.</Hint>
{:else}
  <Panel title={m('admin.permissions.title')} noPadding>
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
      columns={matrixScopes}
      getRowKey={(permission) => permission}
      getColumnKey={(scope) => scope.id}
      getGroupKey={(permission) => getPermissionCategory(permission)}
      emptyMessage={m('rbac.permissions.no_filter_matches')}
      compact
      columnHeaderHeight="10rem"
      columnClass={(scope) => scopeColumnClass(scope.kind)}
      columnAttributes={(scope) => ({ 'data-scope': scope.id })}
      cellAttributes={(permission, scope) => ({
        'data-scope': scope.id,
        'data-permission': permission
      })}
      isCellInteractive={(permission, scope) => Boolean(cellFor(scope.id, permission))}
      spacerTestId="permission-matrix-spacer"
    >
      {#snippet leadingHeader()}
        Permission
      {/snippet}
      {#snippet group(permission)}
        <h3 data-testid="permission-section-divider" class="text-sm font-medium text-muted">
          {getPermissionCategoryLabel(getPermissionCategory(permission))}
        </h3>
      {/snippet}
      {#snippet columnHeader(scope, highlighted)}
        <span
          class={[
            scope.kind === 'SERVER' ? 'font-semibold' : '',
            highlighted ? 'text-action' : '',
            !highlighted && scope.kind === 'GROUP' ? 'text-neutral-action' : '',
            !highlighted && scope.kind === 'ROOM' ? 'text-muted' : ''
          ]}
          title={`${scope.label} (${scope.kind.toLowerCase()})`}
        >
          {#if scope.kind === 'ROOM'}#{/if}{scope.label}
        </span>
      {/snippet}
      {#snippet rowHeader(permission, highlighted)}
        <span
          data-testid="permission-name"
          title={getPermissionDescription(permission)}
          class={['text-sm whitespace-nowrap', highlighted ? 'text-action' : '']}
          >{permission}</span
        >
      {/snippet}
      {#snippet cell(permission, scope)}
        {@const permissionId = permission}
        {@const cell = cellFor(scope.id, permission)}
        {#if cell}
          {@const ov = decisionToState(cell.override)}
          {@const eff = decisionToState(cell.effective)}
          {@const parent = parentDecision(scope, permission)}
          {@const configured = cell.override !== 'NONE' ? cell.override : parent}
          {@const includedBy = includingPermission(scope, permission)}
          {@const binaryEnabled = configured === 'ALLOW' || includedBy !== null}
          {@const inheritedBinaryGrant =
            decisionMode === 'binary' && cell.override === 'NONE' && binaryEnabled}
          {@const displayOverride = forceAllow
            ? 'allow'
            : decisionMode === 'binary'
              ? cell.override === 'ALLOW'
                ? 'allow'
                : 'neutral'
              : ov}
          {@const displayEffective = forceAllow
            ? 'neutral'
            : decisionMode === 'binary'
              ? cell.override === 'NONE' && binaryEnabled
                ? 'allow'
                : 'neutral'
              : eff}
          {@const ariaLabel = forceAllow
            ? `${subjectKind} is always granted ${permissionId} at ${scope.label}`
            : decisionMode === 'binary'
              ? m('rbac.permissions.binary.aria', {
                  permission: permissionId,
                  state: binaryEnabled
                    ? m('rbac.permissions.binary.enabled')
                    : m('rbac.permissions.binary.disabled'),
                  subject: subjectKind,
                  scope: scope.label
                })
              : ov !== 'neutral'
                ? `Override ${ov} for ${permissionId} at ${scope.label}`
                : `No override for ${permissionId} at ${scope.label}, effective ${eff}`}
          {@const allowConstraint =
            cell.allowPermitted === false
              ? m('rbac.permissions.binary.owner_ceiling', {
                  permission: permissionId,
                  scope: scope.label
                })
              : null}
          {@const titleParts = forceAllow
            ? [
                'Allow (owners are always granted all permissions)',
                'Owner permissions are not editable'
              ]
            : decisionMode === 'binary'
              ? [
                  binaryEnabled
                    ? [
                        m('rbac.permissions.binary.enabled'),
                        includedBy
                          ? m('rbac.permissions.included_by', {
                              permission: includedBy
                            })
                          : null,
                        cell.override === 'NONE' ? m('rbac.permissions.binary.inherited') : null,
                        cell.allowPermitted === false
                          ? m('rbac.permissions.binary.unavailable')
                          : null
                      ]
                        .filter(Boolean)
                        .join(' · ')
                    : m('rbac.permissions.binary.disabled'),
                  allowConstraint
                ].filter(Boolean)
              : [
                  ov !== 'neutral'
                    ? `${ov === 'allow' ? 'Allow' : 'Deny'} (${subjectKind} override at ${scope.label})`
                    : null,
                  includedBy
                    ? `Effective Allow (included by ${includedBy})`
                    : null,
                  ov === 'neutral' && eff !== 'neutral'
                    ? `Effective ${eff === 'allow' ? 'Allow' : 'Deny'} (inherited)`
                    : null,
                  ov === 'neutral' && eff === 'neutral' ? 'No decision' : null,
                  allowConstraint
                ].filter(Boolean)}
          <MatrixCell
            override={displayOverride}
            inherited={displayEffective}
            updating={updatingKey === `${scope.id}::${permission}`}
            disabled={readOnly}
            locked={inheritedBinaryGrant}
            allowBlocked={cell.allowPermitted === false &&
              (decisionMode !== 'binary' || parent !== 'ALLOW')}
            ceilingBlocked={cell.allowPermitted === false &&
              (decisionMode === 'binary' ? binaryEnabled : ov === 'allow')}
            {decisionMode}
            {ariaLabel}
            title={titleParts.join(' · ')}
            onCycle={(next) => cycleCell(scope, permission, cell.override, next)}
          />
        {:else}
          <span class="inline-block h-10 w-10" aria-hidden="true"></span>
        {/if}
      {/snippet}
    </MatrixTable>
  </Panel>
{/if}
