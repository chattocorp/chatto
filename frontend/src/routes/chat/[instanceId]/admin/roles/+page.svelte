<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { graphql } from '$lib/gql';
  import { instanceIdToSegment } from '$lib/navigation';
  import { useQuery, useMutation } from '$lib/hooks';
  import { getAdminPermissions } from '$lib/state/instance/permissions.svelte';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { Panel } from '$lib/components/admin';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { RoleTable, type Role } from '$lib/components/rbac';

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

  const getInstanceId = getActiveInstance();

  // Get permissions context from layout
  const adminPerms = getAdminPermissions();
  const canManage = $derived(adminPerms.hasPermission('admin.manage-roles'));

  // Local state for optimistic updates after reorder
  let localRoles = $state<Role[] | null>(null);

  const rolesQuery = useQuery(AdminRolesQuery, () => ({}));
  const reorderMutation = useMutation(ReorderInstanceRolesMutation);

  let roles = $derived(localRoles ?? rolesQuery.data?.admin?.roles ?? []);
  let loading = $derived(rolesQuery.loading);
  let reordering = $derived(reorderMutation.loading);

  function handleEditRole(role: Role) {
    goto(resolve('/chat/[instanceId]/admin/roles/[name]', { instanceId: instanceIdToSegment(getInstanceId()), name: role.name }));
  }

  async function handleReorder(roleNames: string[]) {
    if (reordering) return;

    const result = await reorderMutation.execute({ input: { roleNames } });

    if (result.error) {
      toast.error(`Failed to reorder roles: ${result.error}`);
      // Reset local state to trigger refetch from query
      localRoles = null;
      rolesQuery.refetch();
    } else if (result.data?.reorderInstanceRoles) {
      // Update local state with reordered roles
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
    <Panel title="Instance Roles" icon="iconify uil--shield-check">
      {#snippet actions()}
        {#if canManage}
          <Button
            variant="primary"
            size="sm"
            href={resolve('/chat/[instanceId]/admin/roles/new', { instanceId: instanceIdToSegment(getInstanceId()) })}
          >
            Create Role
          </Button>
        {/if}
      {/snippet}

      <p class="mb-4 text-sm text-muted">
        {#if canManage}
          Manage instance roles and their permissions. Drag custom roles to reorder them. System
          roles (admin, member) cannot be deleted or reordered.
        {:else}
          View instance roles and their permissions. You need the
          <code class="rounded bg-surface-200 px-1">admin.manage-roles</code> permission to make changes.
        {/if}
      </p>

      <RoleTable
        {roles}
        {canManage}
        onEdit={canManage ? handleEditRole : undefined}
        onReorder={canManage ? handleReorder : undefined}
      />
    </Panel>
  {/if}
</div>
