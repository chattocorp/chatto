<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { instanceIdToSegment } from '$lib/navigation';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { Hint } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { RolePermissionEditor } from '$lib/components/rbac';

  const getInstanceId = getActiveInstance();
  const instanceSegment = $derived(instanceIdToSegment(getInstanceId()));
  const spaceId = $derived(page.params.spaceId!);
  const roomId = $derived(page.params.roomId!);
  const roleName = $derived(page.params.roleName!);

  let displayName = $state<string | null>(null);

  const backHref = $derived(
    resolve('/chat/[instanceId]/[spaceId]/[roomId]/settings/permissions', {
      instanceId: instanceSegment,
      spaceId,
      roomId
    })
  );
</script>

<PageTitle title={`${displayName ?? roleName} | Room Permissions`} />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title={displayName ?? roleName}
    subtitle="Room-level overrides for this role"
    backHref={backHref}
    backLabel="Back to roles"
    showMobileNav
  />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    <Hint>
      Set <strong>Allow</strong> or <strong>Deny</strong> to override this role's space-level
      configuration in this room. Leave both off to inherit from the role's space-level setting.
    </Hint>

    <RolePermissionEditor
      {roleName}
      {spaceId}
      {roomId}
      onLoaded={(role) => (displayName = role.displayName)}
    />
  </div>
</div>
