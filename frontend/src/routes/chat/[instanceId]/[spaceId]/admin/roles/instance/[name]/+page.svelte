<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { instanceIdToSegment } from '$lib/navigation';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { graphql } from '$lib/gql';
  import { useQuery } from '$lib/hooks';
  import { Panel } from '$lib/components/admin';
  import { Hint } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { RolePermissionEditor } from '$lib/components/rbac';

  const getInstanceId = getActiveInstance();
  const spaceId = $derived(page.params.spaceId!);
  const instanceRoleName = $derived(page.params.name!);

  let displayName = $state<string | null>(null);
  let description = $state<string>('');

  // We only need viewerCanManageRoles from the space for the gate; the editor
  // handles its own data loading via the unified rolePermissions query.
  const spaceQuery = useQuery(
    graphql(`
      query InstanceRoleConfigSpaceContext($spaceId: ID!) {
        space(id: $spaceId) {
          id
          viewerCanManageRoles
        }
      }
    `),
    () => ({ spaceId })
  );

  const canManageRoles = $derived(spaceQuery.data?.space?.viewerCanManageRoles ?? false);

  const rolesHref = $derived(
    resolve('/chat/[instanceId]/[spaceId]/admin/roles', {
      instanceId: instanceIdToSegment(getInstanceId()),
      spaceId
    })
  );
</script>

<PageTitle title={`instance:${displayName ?? instanceRoleName} | Space Admin`} />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title="Instance Role Permissions"
    subtitle={displayName ? `instance:${displayName}` : 'Loading...'}
    backHref={rolesHref}
    backLabel="Back to roles"
    showMobileNav
  />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    {#if !canManageRoles && !spaceQuery.loading}
      <Hint variant="danger">
        You need the <code class="rounded bg-surface-200 px-1">role.manage</code> permission to
        configure instance role permissions.
      </Hint>
    {:else}
      <Hint variant="warning">
        <strong>Instance role.</strong> The role itself (name, description, instance-level
        permissions) is managed by instance administrators. Here you can configure how this role
        behaves at <em>this</em> space — overrides take precedence over the instance defaults.
      </Hint>

      <Panel title="Role Details" icon="iconify uil--info-circle">
        <div class="flex flex-col gap-4">
          <div>
            <div class="mb-1 text-sm font-medium">Instance Role Name</div>
            <code class="rounded bg-surface-200 px-2 py-1">instance:{instanceRoleName}</code>
          </div>
          <div>
            <div class="mb-1 text-sm font-medium">Display Name</div>
            <div class="text-foreground">{displayName ?? '...'}</div>
          </div>
          <div>
            <div class="mb-1 text-sm font-medium">Description</div>
            <div class="text-muted">{description || '(No description)'}</div>
          </div>
        </div>
      </Panel>

      <Hint>
        Override or supplement the role's instance-scope permissions for this space. Changes save
        immediately.
      </Hint>

      <RolePermissionEditor
        roleName={instanceRoleName}
        {spaceId}
        categoryOrder={['member', 'role', 'space', 'room', 'message']}
        onLoaded={(role) => {
          displayName = role.displayName;
          description = role.description;
        }}
      />
    {/if}
  </div>
</div>
