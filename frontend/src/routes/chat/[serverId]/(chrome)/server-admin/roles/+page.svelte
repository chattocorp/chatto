<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { getServerPermissions } from '$lib/state/server/permissions.svelte';
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
      server {
        viewerCanManageRoles
      }
    }
  `);

  const getInstanceId = getActiveServer();
  const instanceSegment = $derived(serverIdToSegment(getInstanceId()));

  const gateQuery = useQuery(SpaceRolesGateQuery, () => ({}));
  const canManageRoles = $derived(gateQuery.data?.server?.viewerCanManageRoles ?? false);
  const error = $derived(
    gateQuery.error ?? (!gateQuery.loading && !gateQuery.data?.server ? 'Instance not found' : null)
  );

  // Role detail pages require admin.manage-roles. Gate the column-header
  // click so non-admins see plain text.
  const instancePerms = getServerPermissions();
  const canManageRolesFull = $derived(instancePerms.current.canAdminManageRoles);

  function openRoleDetail(role: { roleName: string }) {
    goto(
      resolve('/chat/[serverId]/(chrome)/server-admin/roles/[name]', {
        serverId: instanceSegment,
        name: role.roleName
      })
    );
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
          href={resolve('/chat/[serverId]/(chrome)/server-admin/roles/new', {
            serverId: instanceSegment,
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
        isRoleClickable={() => canManageRolesFull}
      />
    {/if}
  </div>
</div>
