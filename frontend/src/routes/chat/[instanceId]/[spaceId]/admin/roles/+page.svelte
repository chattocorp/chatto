<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { instanceIdToSegment } from '$lib/navigation';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { graphql } from '$lib/gql';
  import { useQuery, useMutation } from '$lib/hooks';
  import { Panel, DataTable } from '$lib/components/admin';
  import { Hint, Pill } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';

  const SpaceRolesQuery = graphql(`
    query SpaceRoles($spaceId: ID!) {
      space(id: $spaceId) {
        id
        name
        roles {
          name
          displayName
          description
          permissions
          permissionDenials
          isSystem
          position
        }
        viewerCanManageRoles
        instanceRoleConfigs {
          role {
            name
            displayName
            description
            position
            isSystem
          }
          permissions
          permissionDenials
        }
      }
    }
  `);

  const ReorderSpaceRolesMutation = graphql(`
    mutation ReorderSpaceRoles($input: ReorderSpaceRolesInput!) {
      reorderSpaceRoles(input: $input) {
        name
        displayName
        description
        permissions
        permissionDenials
        isSystem
        position
      }
    }
  `);

  type RoleRow = {
    name: string;
    displayName: string;
    description: string;
    isSystem: boolean;
    position: number;
    kind: 'space' | 'instance';
    grantCount: number;
    denyCount: number;
  };

  const getInstanceId = getActiveInstance();
  const instanceSegment = $derived(instanceIdToSegment(getInstanceId()));
  const spaceId = $derived(page.params.spaceId!);

  const rolesQuery = useQuery(SpaceRolesQuery, () => ({ spaceId }));
  const reorderMutation = useMutation(ReorderSpaceRolesMutation);

  const canManageRoles = $derived(rolesQuery.data?.space?.viewerCanManageRoles ?? false);
  const loading = $derived(rolesQuery.loading);
  const error = $derived(
    rolesQuery.error ?? (!rolesQuery.loading && !rolesQuery.data?.space ? 'Space not found' : null)
  );
  const reordering = $derived(reorderMutation.loading);

  // Build the unified row list: space roles + instance role configs.
  const rows = $derived.by((): RoleRow[] => {
    const spaceRoles = rolesQuery.data?.space?.roles ?? [];
    const instanceConfigs = rolesQuery.data?.space?.instanceRoleConfigs ?? [];
    const result: RoleRow[] = [];
    for (const r of spaceRoles) {
      result.push({
        name: r.name,
        displayName: r.displayName,
        description: r.description,
        isSystem: r.isSystem,
        position: r.position,
        kind: 'space',
        grantCount: r.permissions.length,
        denyCount: r.permissionDenials.length
      });
    }
    for (const c of instanceConfigs) {
      result.push({
        name: c.role.name,
        displayName: c.role.displayName,
        description: c.role.description,
        isSystem: c.role.isSystem,
        position: c.role.position,
        kind: 'instance',
        grantCount: c.permissions.length,
        denyCount: c.permissionDenials.length
      });
    }
    return result.sort((a, b) => {
      // Group by kind (space first), then by position.
      if (a.kind !== b.kind) return a.kind === 'space' ? -1 : 1;
      return a.position - b.position;
    });
  });

  function editRow(row: RoleRow) {
    if (row.kind === 'space') {
      goto(
        resolve('/chat/[instanceId]/[spaceId]/admin/roles/[name]', {
          instanceId: instanceSegment,
          spaceId,
          name: row.name
        })
      );
    } else {
      goto(
        resolve('/chat/[instanceId]/[spaceId]/admin/roles/instance/[name]', {
          instanceId: instanceSegment,
          spaceId,
          name: row.name
        })
      );
    }
  }

  function goToNewRole() {
    goto(
      resolve('/chat/[instanceId]/[spaceId]/admin/roles/new', {
        instanceId: instanceSegment,
        spaceId
      })
    );
  }

  // Drag-and-drop reorder is space-roles only; instance role configs aren't
  // reorderable here (their position lives at instance scope). We render two
  // grouped sections within the single panel to keep things simple.
  async function moveSpaceRole(name: string, direction: -1 | 1) {
    if (reordering || !canManageRoles) return;
    const spaceRoles = rolesQuery.data?.space?.roles?.filter((r) => !r.isSystem) ?? [];
    const ordered = [...spaceRoles].sort((a, b) => a.position - b.position);
    const idx = ordered.findIndex((r) => r.name === name);
    if (idx < 0) return;
    const target = idx + direction;
    if (target < 0 || target >= ordered.length) return;
    const swapped = [...ordered];
    [swapped[idx], swapped[target]] = [swapped[target], swapped[idx]];
    const result = await reorderMutation.execute({
      input: { spaceId, roleNames: swapped.map((r) => r.name) }
    });
    if (result.error) {
      toast.error(`Failed to reorder roles: ${result.error}`);
    } else {
      toast.success('Role order updated');
      rolesQuery.refetch();
    }
  }
</script>

<PageTitle title="Roles | Space Admin" />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader title="Roles" subtitle="Manage space roles and permissions" showMobileNav />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    {#if loading}
      <div class="text-muted">Loading roles...</div>
    {:else if error}
      <Hint variant="danger">{error}</Hint>
    {:else}
      <Hint>
        Space roles live in this space; instance roles are defined at the instance level — you can
        override their space-level permissions from here.
        {#if !canManageRoles}
          You need the <code class="rounded bg-surface-200 px-1">role.manage</code> permission to
          change anything.
        {/if}
      </Hint>

      <Panel title="Roles applicable in this space" icon="iconify uil--shield-check" noPadding>
        {#snippet actions()}
          {#if canManageRoles}
            <Button variant="primary" size="sm" onclick={goToNewRole}>Create Role</Button>
          {/if}
        {/snippet}
        <DataTable
          items={rows}
          columns={canManageRoles ? 5 : 4}
          getKey={(row) => `${row.kind}:${row.name}`}
          onRowClick={editRow}
          emptyMessage="No roles found"
        >
          {#snippet header()}
            <th class="px-4 py-3 font-medium">Role</th>
            <th class="px-4 py-3 text-center font-medium">Scope</th>
            <th class="px-4 py-3 text-center font-medium">Type</th>
            <th class="px-4 py-3 text-center font-medium">Grants / Denies</th>
            {#if canManageRoles}
              <th class="px-4 py-3 text-right font-medium">Actions</th>
            {/if}
          {/snippet}
          {#snippet row(r)}
            <td class="px-4 py-3">
              <div class="font-medium">{r.displayName}</div>
              <code class="text-xs text-muted">{r.name}</code>
              {#if r.description}
                <div class="mt-0.5 text-xs text-muted">{r.description}</div>
              {/if}
            </td>
            <td class="px-4 py-3 text-center">
              <Pill tone={r.kind === 'instance' ? 'accent' : 'primary'}>
                {r.kind === 'instance' ? 'Instance' : 'Space'}
              </Pill>
            </td>
            <td class="px-4 py-3 text-center">
              <Pill tone={r.isSystem ? 'muted' : 'primary'}>
                {r.isSystem ? 'System' : 'Custom'}
              </Pill>
            </td>
            <td class="px-4 py-3 text-center text-sm">
              <span class="text-success">{r.grantCount}</span>
              <span class="text-muted"> / </span>
              <span class="text-danger">{r.denyCount}</span>
            </td>
            {#if canManageRoles}
              <td class="px-4 py-3">
                <div class="flex items-center justify-end gap-1">
                  {#if r.kind === 'space' && !r.isSystem}
                    <button
                      type="button"
                      class="cursor-pointer rounded p-1 text-muted hover:bg-surface-200 hover:text-text"
                      title="Move up"
                      disabled={reordering}
                      onclick={(e) => {
                        e.stopPropagation();
                        moveSpaceRole(r.name, -1);
                      }}
                    >
                      <span class="iconify text-base uil--angle-up"></span>
                    </button>
                    <button
                      type="button"
                      class="cursor-pointer rounded p-1 text-muted hover:bg-surface-200 hover:text-text"
                      title="Move down"
                      disabled={reordering}
                      onclick={(e) => {
                        e.stopPropagation();
                        moveSpaceRole(r.name, 1);
                      }}
                    >
                      <span class="iconify text-base uil--angle-down"></span>
                    </button>
                  {/if}
                  <Button
                    variant="ghost"
                    size="sm"
                    onclick={(e: MouseEvent) => {
                      e.stopPropagation();
                      editRow(r);
                    }}
                  >
                    Edit
                  </Button>
                </div>
              </td>
            {/if}
          {/snippet}
        </DataTable>
      </Panel>
    {/if}
  </div>
</div>
