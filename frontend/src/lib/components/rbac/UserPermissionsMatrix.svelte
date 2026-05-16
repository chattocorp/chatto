<!--
@component

Per-user permission matrix. Rows are permissions (grouped by category, one
`Panel` per category); columns are the scopes at which the user can have
an explicit override (server tier, every room group, every channel room).

For each cell:

  - The user's **explicit override** (ALLOW / DENY / NONE) drives the cell's
    saturation: solid for ALLOW/DENY, faded for NONE.
  - The **effective** decision (after the full resolver walk, including
    role-derived grants and any user-level override) drives the cell's
    color when there is no override — so the faded baseline reflects
    what the user actually gets from roles.

Clicking a cell cycles `neutral → allow → deny → neutral` at that
(permission, scope) pair via `grantUserPermission` / `denyUserPermission`
/ `clearUserPermissionState`.

Sparse: cells only exist for (permission, scope) intersections where the
permission applies at the scope's tier (room-only perms don't appear in
the server column, server-only perms don't appear in room columns, etc.).
A missing cell renders as an inert "—".
-->
<script lang="ts">
  import { Panel, DataTable } from '$lib/components/admin';
  import { Hint } from '$lib/ui';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { graphql } from '$lib/gql';
  import { toast } from '$lib/ui/toast';
  import { getPermissionDisplayName } from '$lib/permissions';
  import {
    setUserPermission,
    type UserMutationScope,
    type UserPermissionState
  } from './userPermissionMutations';
  import MatrixCell from './MatrixCell.svelte';

  type Decision = 'ALLOW' | 'DENY' | 'NONE';
  type ScopeKind = 'SERVER' | 'GROUP' | 'ROOM';

  type Scope = {
    id: string;
    label: string;
    kind: ScopeKind;
    parentGroupId: string;
  };
  type Cell = {
    permission: string;
    scopeId: string;
    override: Decision;
    effective: Decision;
  };
  type Matrix = {
    userId: string;
    applicablePermissions: string[];
    scopes: Scope[];
    cells: Cell[];
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
      title: 'Space Operations',
      description: 'Control who can browse, create, join, and manage spaces'
    },
    room: {
      title: 'Rooms',
      description: 'Control who can create, join, and manage rooms'
    },
    message: { title: 'Messages', description: 'Control what users can do with messages' },
    member: {
      title: 'Member Management',
      description: 'Control who can invite and remove space members'
    },
    role: {
      title: 'Role Management',
      description: 'Control who can create roles and assign them to users'
    },
    admin: {
      title: 'Server Administration',
      description: 'Access to server-wide admin functions'
    },
    dm: { title: 'Direct Messages', description: 'Control access to direct messaging' },
    user: { title: 'User Management', description: 'Control user account operations' }
  };

  let {
    userId,
    categoryOrder = DEFAULT_CATEGORY_ORDER
  }: {
    userId: string;
    categoryOrder?: string[];
  } = $props();

  const connection = useConnection();

  let data = $state<Matrix | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let updating = $state<string | null>(null); // "{scopeId}::{permission}" while a mutation is in flight

  $effect(() => {
    void load(userId);
  });

  async function load(uid: string) {
    loading = true;
    error = null;

    const resp = await connection().client.query(
      graphql(`
        query UserPermissionsMatrixQuery($userId: ID!) {
          userPermissionMatrix(userId: $userId) {
            userId
            applicablePermissions
            scopes {
              id
              label
              kind
              parentGroupId
            }
            cells {
              permission
              scopeId
              override
              effective
            }
          }
        }
      `),
      { userId: uid },
      { requestPolicy: 'network-only' }
    );

    if (uid !== userId) return;

    loading = false;
    if (resp.error) {
      error = resp.error.message;
      return;
    }
    if (!resp.data?.userPermissionMatrix) {
      error = 'No data returned';
      return;
    }
    const m = resp.data.userPermissionMatrix;
    data = {
      userId: m.userId,
      applicablePermissions: [...m.applicablePermissions],
      scopes: m.scopes.map((s) => ({ ...s })),
      cells: m.cells.map((c) => ({ ...c }))
    };
  }

  // ----- Column layout ----------------------------------------------------

  // Order columns: server first, then each group followed by its rooms.
  // Backend returns server, then all groups, then all rooms — we re-order
  // here so rooms nest visually under their parent group.
  const orderedScopes = $derived.by<Scope[]>(() => {
    if (!data) return [];
    const server = data.scopes.filter((s) => s.kind === 'SERVER');
    const groups = data.scopes.filter((s) => s.kind === 'GROUP');
    const rooms = data.scopes.filter((s) => s.kind === 'ROOM');
    const out: Scope[] = [...server];
    for (const g of groups) {
      out.push(g);
      const groupId = g.id.startsWith('group:') ? g.id.slice('group:'.length) : '';
      for (const r of rooms) {
        if (r.parentGroupId === groupId) out.push(r);
      }
    }
    // Any orphaned rooms (no matching parent group) get appended at the end.
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

  // ----- Cell lookup ------------------------------------------------------

  // Fast index: "{scopeId}|{permission}" → Cell.
  const cellIndex = $derived.by(() => {
    // eslint-disable-next-line svelte/prefer-svelte-reactivity -- Map is ephemeral within derived computation
    const idx = new Map<string, Cell>();
    if (!data) return idx;
    for (const cell of data.cells) {
      idx.set(`${cell.scopeId}|${cell.permission}`, cell);
    }
    return idx;
  });

  function cellFor(scopeId: string, permission: string): Cell | undefined {
    return cellIndex.get(`${scopeId}|${permission}`);
  }

  function decisionToState(d: Decision): 'allow' | 'deny' | 'neutral' {
    if (d === 'ALLOW') return 'allow';
    if (d === 'DENY') return 'deny';
    return 'neutral';
  }

  // ----- Mutations --------------------------------------------------------

  function mutationScopeFor(scope: Scope): UserMutationScope {
    if (scope.kind === 'GROUP') {
      const groupId = scope.id.startsWith('group:') ? scope.id.slice('group:'.length) : '';
      return { tier: 'group', groupId };
    }
    if (scope.kind === 'ROOM') {
      const roomId = scope.id.startsWith('room:') ? scope.id.slice('room:'.length) : '';
      return { tier: 'room', roomId };
    }
    return { tier: 'server' };
  }

  async function cycle(scope: Scope, permission: string, next: UserPermissionState) {
    if (!data) return;
    const cellKey = `${scope.id}::${permission}`;
    updating = cellKey;
    error = null;

    const result = await setUserPermission(
      connection().client,
      data.userId,
      mutationScopeFor(scope),
      permission,
      next
    );
    if (result.error) {
      error = result.error;
      toast.error(result.error);
      updating = null;
      return;
    }

    // Reload the matrix so both the override AND effective decisions stay
    // consistent. The effective state can change in non-adjacent cells (a
    // server-scope grant flows into rooms via inheritance), so a naive
    // local patch wouldn't be enough.
    await load(data.userId);
    updating = null;
  }

  // ----- Visuals for scope-kind ------------------------------------------

  function scopeColumnClass(kind: ScopeKind): string {
    if (kind === 'SERVER') return 'bg-surface-200/40';
    if (kind === 'GROUP') return 'bg-surface-200/20';
    return '';
  }
</script>

{#if error}
  <Hint tone="danger">{error}</Hint>
{/if}

{#if loading}
  <div class="text-muted">Loading permissions…</div>
{:else if !data || orderedScopes.length === 0}
  <Hint tone="info">No scopes available for this user.</Hint>
{:else}
  <div class="flex flex-col gap-6">
    {#each groupedPermissions as group (group.category)}
      {@const meta = CATEGORY_META[group.category]}
      <Panel title={meta?.title ?? group.category} subtitle={meta?.description} noPadding>
        <div class="overflow-x-auto" style="width: max-content; max-width: 100%">
          <DataTable
            items={group.permissions}
            columns={orderedScopes.length + 1}
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
              {#each orderedScopes as scope (scope.id)}
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
              <td class="sticky left-0 z-10 bg-background px-4 py-2">
                <div data-testid="permission-name">{getPermissionDisplayName(permission)}</div>
                <div class="text-xs text-muted/70">{permission}</div>
              </td>
              {#each orderedScopes as scope (scope.id)}
                {@const cell = cellFor(scope.id, permission)}
                {@const cellKey = `${scope.id}::${permission}`}
                {@const isUpdating = updating === cellKey}
                <td
                  class={['px-0 py-2 text-center', scopeColumnClass(scope.kind)]}
                  style="width: 2rem; min-width: 2rem"
                  data-scope={scope.id}
                  data-permission={permission}
                >
                  {#if cell}
                    {@const ov = decisionToState(cell.override)}
                    {@const eff = decisionToState(cell.effective)}
                    {@const ariaLabel =
                      ov !== 'neutral'
                        ? `Override ${ov} for ${permission} at ${scope.label}`
                        : `No override for ${permission} at ${scope.label}, effective ${eff}`}
                    {@const titleParts = [
                      ov !== 'neutral'
                        ? `${ov === 'allow' ? 'Allow' : 'Deny'} (user override at ${scope.label})`
                        : null,
                      ov === 'neutral' && eff !== 'neutral'
                        ? `Effective ${eff === 'allow' ? 'Allow' : 'Deny'} (from roles)`
                        : null,
                      ov === 'neutral' && eff === 'neutral' ? 'No decision' : null
                    ].filter(Boolean)}
                    <MatrixCell
                      override={ov}
                      inherited={eff}
                      updating={isUpdating}
                      {ariaLabel}
                      title={titleParts.join(' · ')}
                      onCycle={(next) => void cycle(scope, permission, next)}
                    />
                  {:else}
                    <span
                      class="inline-flex h-5 w-5 items-center justify-center text-xs text-muted/30"
                      title="Permission does not apply at this scope"
                      aria-label="Not applicable"
                    >
                      —
                    </span>
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
