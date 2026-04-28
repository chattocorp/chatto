<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { instanceIdToSegment } from '$lib/navigation';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { useConnection } from '$lib/state/instance/connection.svelte';
  import { graphql } from '$lib/gql';
  import { Panel } from '$lib/components/admin';
  import { Hint } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { PermissionGrid, type PermissionState } from '$lib/components/rbac';

  type Tier = { permissions: string[]; permissionDenials: string[] } | null;
  type RoleAcrossTiers = {
    roleName: string;
    displayName: string;
    description: string;
    isInstanceRole: boolean;
    isSystem: boolean;
    position: number;
    applicablePermissions: string[];
    instance: Tier;
    space: Tier;
  };

  const getInstanceId = getActiveInstance();
  const connection = useConnection();
  const spaceId = $derived(page.params.spaceId!);
  const instanceRoleName = $derived(page.params.name!);

  let role = $state<RoleAcrossTiers | null>(null);
  let canManageRoles = $state(false);
  let loading = $state(true);
  let updating = $state<string | null>(null);
  let error = $state<string | null>(null);

  async function loadData() {
    loading = true;
    error = null;

    const resp = await connection().client.query(
      graphql(`
        query InstanceRoleSpaceDetail($roleName: String!, $spaceId: ID!) {
          rolePermissions(roleName: $roleName, spaceId: $spaceId) {
            roleName
            displayName
            description
            isInstanceRole
            isSystem
            position
            applicablePermissions
            instance {
              permissions
              permissionDenials
            }
            space {
              permissions
              permissionDenials
            }
          }
          space(id: $spaceId) {
            id
            viewerCanManageRoles
          }
        }
      `),
      { roleName: instanceRoleName, spaceId }
    );

    if (resp.error) {
      error = resp.error.message;
      loading = false;
      return;
    }

    if (!resp.data?.rolePermissions) {
      error = 'Instance role not found';
      loading = false;
      return;
    }

    role = resp.data.rolePermissions as RoleAcrossTiers;
    canManageRoles = resp.data.space?.viewerCanManageRoles ?? false;
    loading = false;
  }

  $effect(() => {
    if (spaceId && instanceRoleName) {
      loadData();
    }
  });

  async function setPermissionState(permission: string, newState: PermissionState) {
    if (!role || !role.space) return;

    updating = permission;
    error = null;

    let mutation;
    switch (newState) {
      case 'allow':
        mutation = graphql(`
          mutation GrantInstanceRoleSpacePermission($input: GrantInstanceRoleSpacePermissionInput!) {
            grantInstanceRoleSpacePermission(input: $input)
          }
        `);
        break;
      case 'deny':
        mutation = graphql(`
          mutation DenyInstanceRoleSpacePermission($input: DenyInstanceRoleSpacePermissionInput!) {
            denyInstanceRoleSpacePermission(input: $input)
          }
        `);
        break;
      case 'neutral':
        mutation = graphql(`
          mutation ClearInstanceRoleSpacePermission($input: ClearInstanceRoleSpacePermissionInput!) {
            clearInstanceRoleSpacePermission(input: $input)
          }
        `);
        break;
    }

    const resp = await connection().client.mutation(mutation, {
      input: { spaceId, instanceRole: instanceRoleName, permission }
    });

    if (resp.error) {
      error = resp.error.message;
    } else if (role.space) {
      role.space.permissions = role.space.permissions.filter((p) => p !== permission);
      role.space.permissionDenials = role.space.permissionDenials.filter((p) => p !== permission);
      if (newState === 'allow') {
        role.space.permissions = [...role.space.permissions, permission];
        toast.success(`Granted ${permission}`);
      } else if (newState === 'deny') {
        role.space.permissionDenials = [...role.space.permissionDenials, permission];
        toast.success(`Denied ${permission}`);
      } else {
        toast.success(`Cleared ${permission}`);
      }
    }

    updating = null;
  }

  function goBack() {
    goto(resolve('/chat/[instanceId]/[spaceId]/admin/roles', { instanceId: instanceIdToSegment(getInstanceId()), spaceId }));
  }
</script>

<PageTitle title={`instance:${role?.displayName ?? instanceRoleName} | Space Admin`} />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title="Instance Role Permissions"
    subtitle={role ? `instance:${role.displayName}` : 'Loading...'}
    showMobileNav
  >
    {#snippet actions()}
      <Button variant="secondary" onclick={goBack}>Back to Roles</Button>
    {/snippet}
  </PaneHeader>

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    {#if error}
      <Hint variant="danger">{error}</Hint>
    {/if}

    {#if loading}
      <div class="text-muted">Loading instance role...</div>
    {:else if !role}
      <Hint variant="danger">Instance role not found</Hint>
    {:else if !canManageRoles}
      <Hint variant="danger">
        You need the <code class="rounded bg-surface-200 px-1">admin.manage-roles</code> permission
        to configure instance role permissions.
      </Hint>
    {:else}
      <Hint variant="warning">
        <strong>Instance role.</strong> The role itself (name, description, instance-level
        permissions) is managed by instance administrators. Here you can configure how this role
        behaves at <em>this</em> space — overrides will take precedence over the instance defaults.
      </Hint>

      <!-- Role Metadata (read-only) -->
      <Panel title="Role Details" icon="iconify uil--info-circle">
        <div class="flex flex-col gap-4">
          <div>
            <div class="mb-1 text-sm font-medium">Instance Role Name</div>
            <code class="rounded bg-surface-200 px-2 py-1">instance:{role.roleName}</code>
          </div>
          <div>
            <div class="mb-1 text-sm font-medium">Display Name</div>
            <div class="text-foreground">{role.displayName}</div>
          </div>
          <div>
            <div class="mb-1 text-sm font-medium">Description</div>
            <div class="text-muted">{role.description || '(No description)'}</div>
          </div>
        </div>
      </Panel>

      <!-- Space-level overrides for this instance role -->
      <Panel title="Space Permissions" icon="iconify uil--shield-check">
        <p class="mb-4 text-sm text-muted">
          Override or supplement the role's instance-scope permissions for this space. Changes save
          immediately.
        </p>

        <PermissionGrid
          permissions={role.applicablePermissions}
          grantedPermissions={role.space?.permissions ?? []}
          deniedPermissions={role.space?.permissionDenials ?? []}
          inheritedPermissions={role.instance?.permissions ?? []}
          inheritedDenials={role.instance?.permissionDenials ?? []}
          inheritedFromLabel="instance"
          updatingPermission={updating}
          categoryOrder={['member', 'role', 'space', 'room', 'message']}
          onSetState={setPermissionState}
        />
      </Panel>
    {/if}
  </div>
</div>
