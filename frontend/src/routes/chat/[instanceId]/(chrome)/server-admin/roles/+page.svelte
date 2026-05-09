<script lang="ts">
  import { goto } from '$app/navigation';
  import { getActiveInstanceSpaceId } from '$lib/state/activeInstance.svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { instanceIdToSegment } from '$lib/navigation';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { getInstancePermissions } from '$lib/state/instance/permissions.svelte';
  import { graphql } from '$lib/gql';
  import { useQuery } from '$lib/hooks';
  import { Hint } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button } from '$lib/ui/form';
  import PermissionMatrix from '$lib/components/rbac/PermissionMatrix.svelte';

  // Lightweight query just to gate UI on viewerCanManageRoles. The heavy
  // lifting is done by PermissionMatrix's own tierRoles query.
  const SpaceRolesGateQuery = graphql(`
    query SpaceRolesGate {
      instance {
        viewerCanManageRoles
      }
    }
  `);

  const getInstanceId = getActiveInstance();
  const instanceSegment = $derived(instanceIdToSegment(getInstanceId()));
  const spaceId = $derived(getActiveInstanceSpaceId()());

  const gateQuery = useQuery(SpaceRolesGateQuery, () => ({}));
  const canManageRoles = $derived(gateQuery.data?.instance?.viewerCanManageRoles ?? false);
  const error = $derived(
    gateQuery.error ?? (!gateQuery.loading && !gateQuery.data?.instance ? 'Instance not found' : null)
  );

  // Instance role detail pages require instance admin (admin.manage-roles);
  // a space admin without that permission would land on a permission-denied
  // shell. Gate the column-header click so non-admins see plain text.
  const instancePerms = getInstancePermissions();
  const canManageInstanceRoles = $derived(instancePerms.current.canAdminManageRoles);

  // Instance roles live at instance scope — clicking their column header
  // jumps to the instance role detail (where metadata + assigned users live);
  // space roles open the space role detail.
  function openRoleDetail(role: { roleName: string; isInstanceRole: boolean }) {
    if (role.isInstanceRole) {
      goto(
        resolve('/chat/[instanceId]/admin/roles/[name]', {
          instanceId: instanceSegment,
          name: role.roleName
        })
      );
    } else {
      goto(
        resolve('/chat/[instanceId]/(chrome)/server-admin/roles/[name]', {
          instanceId: instanceSegment,
          name: role.roleName
        })
      );
    }
  }
</script>

<PageTitle title="Roles | Space Admin" />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader title="Roles" subtitle="Manage space roles and permissions" showMobileNav>
    {#snippet actions()}
      {#if canManageRoles}
        <Button
          variant="primary"
          size="sm"
          href={resolve('/chat/[instanceId]/(chrome)/server-admin/roles/new', {
            instanceId: instanceSegment,
          })}
        >
          Create Role
        </Button>
      {/if}
    {/snippet}
  </PaneHeader>

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    {#if error}
      <Hint tone="danger">{error}</Hint>
    {:else}
      <PermissionMatrix
        onRoleClick={openRoleDetail}
        isRoleClickable={(role) => (role.isInstanceRole ? canManageInstanceRoles : true)}
      />
    {/if}
  </div>
</div>
