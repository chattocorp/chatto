<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { instanceIdToSegment } from '$lib/navigation';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { graphql } from '$lib/gql';
  import { useQuery, useMutation } from '$lib/hooks';
  import { Panel } from '$lib/components/admin';
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
  <PaneHeader title="Roles" subtitle="Manage space roles and permissions" showMobileNav>
    {#snippet actions()}
      {#if canManageRoles}
        <Button variant="primary" onclick={goToNewRole}>Create Role</Button>
      {/if}
    {/snippet}
  </PaneHeader>

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    {#if loading}
      <div class="text-muted">Loading roles...</div>
    {:else if error}
      <div class="text-danger">{error}</div>
    {:else}
      <Panel title="Roles applicable in this space" icon="iconify uil--shield-check">
        <p class="mb-4 text-sm text-muted">
          Space roles live in this space; instance roles are defined at the instance level — you
          can override their space-level permissions from here.
          {#if !canManageRoles}
            You need the <code class="rounded bg-surface-200 px-1">role.manage</code> permission to
            change anything.
          {/if}
        </p>

        <table class="w-full border-collapse">
          <thead>
            <tr class="border-b border-border bg-surface-200/50">
              <th class="px-4 py-3 text-left text-sm font-medium">Role</th>
              <th class="px-4 py-3 text-center text-sm font-medium">Scope</th>
              <th class="px-4 py-3 text-center text-sm font-medium">Type</th>
              <th class="px-4 py-3 text-center text-sm font-medium">Grants / Denies</th>
              {#if canManageRoles}
                <th class="px-4 py-3 text-center text-sm font-medium">Actions</th>
              {/if}
            </tr>
          </thead>
          <tbody>
            {#each rows as row (`${row.kind}:${row.name}`)}
              <tr
                class="cursor-pointer border-b border-border bg-surface last:border-b-0 hover:bg-surface-200"
                onclick={() => editRow(row)}
              >
                <td class="px-4 py-3">
                  <div class="font-medium">{row.displayName}</div>
                  <code class="text-xs text-muted">{row.name}</code>
                  {#if row.description}
                    <div class="mt-0.5 text-xs text-muted">{row.description}</div>
                  {/if}
                </td>
                <td class="px-4 py-3 text-center">
                  {#if row.kind === 'instance'}
                    <span class="rounded bg-accent/10 px-2 py-0.5 text-xs font-medium text-accent">
                      Instance
                    </span>
                  {:else}
                    <span class="rounded bg-primary/10 px-2 py-0.5 text-xs font-medium text-primary">
                      Space
                    </span>
                  {/if}
                </td>
                <td class="px-4 py-3 text-center">
                  {#if row.isSystem}
                    <span class="rounded bg-surface-200 px-2 py-0.5 text-xs text-muted">System</span>
                  {:else}
                    <span class="rounded bg-primary/10 px-2 py-0.5 text-xs text-primary">Custom</span>
                  {/if}
                </td>
                <td class="px-4 py-3 text-center text-sm">
                  <span class="text-success">{row.grantCount}</span>
                  <span class="text-muted"> / </span>
                  <span class="text-danger">{row.denyCount}</span>
                </td>
                {#if canManageRoles}
                  <td class="px-4 py-3 text-center">
                    <div class="flex items-center justify-center gap-1">
                      {#if row.kind === 'space' && !row.isSystem}
                        <button
                          type="button"
                          class="cursor-pointer rounded p-1 text-muted hover:bg-surface-200 hover:text-text"
                          title="Move up"
                          disabled={reordering}
                          onclick={(e) => {
                            e.stopPropagation();
                            moveSpaceRole(row.name, -1);
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
                            moveSpaceRole(row.name, 1);
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
                          editRow(row);
                        }}
                      >
                        Edit
                      </Button>
                    </div>
                  </td>
                {/if}
              </tr>
            {/each}
          </tbody>
        </table>
      </Panel>
    {/if}
  </div>
</div>
