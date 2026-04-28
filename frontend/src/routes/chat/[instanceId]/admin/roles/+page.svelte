<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { instanceIdToSegment } from '$lib/navigation';
  import { getAdminPermissions } from '$lib/state/instance/permissions.svelte';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button } from '$lib/ui/form';
  import PermissionMatrix from '$lib/components/rbac/PermissionMatrix.svelte';

  const getInstanceId = getActiveInstance();
  const instanceSegment = $derived(instanceIdToSegment(getInstanceId()));

  const adminPerms = getAdminPermissions();
  const canManage = $derived(adminPerms.hasPermission('admin.manage-roles'));

  function openRoleDetail(role: { roleName: string }) {
    goto(
      resolve('/chat/[instanceId]/admin/roles/[name]', {
        instanceId: instanceSegment,
        name: role.roleName
      })
    );
  }
</script>

<PageTitle title="Roles | Admin" />

<PaneHeader title="Roles" subtitle="Manage instance-level roles and their permissions" showMobileNav>
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
</PaneHeader>

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  <PermissionMatrix onRoleClick={openRoleDetail} />
</div>
