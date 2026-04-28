<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { graphql } from '$lib/gql';
  import { instanceIdToSegment } from '$lib/navigation';
  import { useQuery, useMutation } from '$lib/hooks';
  import { getAdminPermissions } from '$lib/state/instance/permissions.svelte';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { Panel, DataTable } from '$lib/components/admin';
  import { Hint, Pill } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';

  const AdminRolesQuery = graphql(`
    query AdminRoles {
      admin {
        roles {
          name
          displayName
          description
          permissions
          permissionDenials
          isSystem
          position
        }
      }
    }
  `);

  const ReorderInstanceRolesMutation = graphql(`
    mutation ReorderInstanceRoles($input: ReorderInstanceRolesInput!) {
      reorderInstanceRoles(input: $input) {
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

  type Role = {
    name: string;
    displayName: string;
    description: string;
    permissions: string[];
    permissionDenials: string[];
    isSystem: boolean;
    position: number;
  };

  const getInstanceId = getActiveInstance();
  const instanceSegment = $derived(instanceIdToSegment(getInstanceId()));

  const adminPerms = getAdminPermissions();
  const canManage = $derived(adminPerms.hasPermission('admin.manage-roles'));

  // Local state for optimistic updates after reorder.
  let localRoles = $state<Role[] | null>(null);

  const rolesQuery = useQuery(AdminRolesQuery, () => ({}));
  const reorderMutation = useMutation(ReorderInstanceRolesMutation);

  const roles = $derived<Role[]>(
    [...(localRoles ?? rolesQuery.data?.admin?.roles ?? [])].sort((a, b) => a.position - b.position)
  );
  const loading = $derived(rolesQuery.loading);
  const reordering = $derived(reorderMutation.loading);

  function editRole(role: Role) {
    goto(
      resolve('/chat/[instanceId]/admin/roles/[name]', {
        instanceId: instanceSegment,
        name: role.name
      })
    );
  }

  async function moveRole(name: string, direction: -1 | 1) {
    if (reordering || !canManage) return;
    const customRoles = roles.filter((r) => !r.isSystem);
    const idx = customRoles.findIndex((r) => r.name === name);
    if (idx < 0) return;
    const target = idx + direction;
    if (target < 0 || target >= customRoles.length) return;
    const swapped = [...customRoles];
    [swapped[idx], swapped[target]] = [swapped[target], swapped[idx]];

    const result = await reorderMutation.execute({
      input: { roleNames: swapped.map((r) => r.name) }
    });

    if (result.error) {
      toast.error(`Failed to reorder roles: ${result.error}`);
      localRoles = null;
      rolesQuery.refetch();
    } else if (result.data?.reorderInstanceRoles) {
      localRoles = result.data.reorderInstanceRoles;
      toast.success('Role order updated');
    }
  }
</script>

<PageTitle title="Roles | Admin" />

<PaneHeader
  title="Roles"
  subtitle="Manage instance-level roles and their permissions"
  showMobileNav
/>

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  {#if loading}
    <div class="text-muted">Loading roles...</div>
  {:else}
    <Hint>
      Manage instance-level roles. Use the move up/down buttons to reorder custom roles. System
      roles maintain fixed positions.
      {#if !canManage}
        You need the <code class="rounded bg-surface-200 px-1">admin.manage-roles</code> permission
        to make changes.
      {/if}
    </Hint>

    <Panel title="Instance Roles" icon="iconify uil--shield-check" noPadding>
      {#snippet actions()}
        {#if canManage}
          <Button
            variant="primary"
            size="sm"
            href={resolve('/chat/[instanceId]/admin/roles/new', { instanceId: instanceSegment })}
          >
            Create Role
          </Button>
        {/if}
      {/snippet}

      <DataTable
        items={roles}
        columns={canManage ? 4 : 3}
        getKey={(r) => r.name}
        onRowClick={editRole}
        emptyMessage="No roles found"
      >
        {#snippet header()}
          <th class="px-4 py-3 text-left font-medium">Role</th>
          <th class="px-4 py-3 text-center font-medium">Type</th>
          <th class="px-4 py-3 text-center font-medium">Grants / Denies</th>
          {#if canManage}
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
            <Pill tone={r.isSystem ? 'muted' : 'primary'}>
              {r.isSystem ? 'System' : 'Custom'}
            </Pill>
          </td>
          <td class="px-4 py-3 text-center text-sm">
            <span class="text-success">{r.permissions.length}</span>
            <span class="text-muted"> / </span>
            <span class="text-danger">{r.permissionDenials.length}</span>
          </td>
          {#if canManage}
            <td class="px-4 py-3">
              <div class="flex items-center justify-end gap-1">
                {#if !r.isSystem}
                  <button
                    type="button"
                    class="cursor-pointer rounded p-1 text-muted hover:bg-surface-200 hover:text-text"
                    title="Move up"
                    disabled={reordering}
                    onclick={(e) => {
                      e.stopPropagation();
                      moveRole(r.name, -1);
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
                      moveRole(r.name, 1);
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
                    editRole(r);
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
