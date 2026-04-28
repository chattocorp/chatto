<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { graphql } from '$lib/gql';
  import { instanceIdToSegment } from '$lib/navigation';
  import { useQuery, useMutation } from '$lib/hooks';
  import { getAdminPermissions } from '$lib/state/instance/permissions.svelte';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { Panel, UserList } from '$lib/components/admin';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button, TextInput, TextArea, FormError } from '$lib/ui/form';
  import { DeleteRoleModal, type Role } from '$lib/components/rbac';

  let { data } = $props();

  type User = { id: string; login: string; displayName: string };

  const getInstanceId = getActiveInstance();

  // Get permissions context from layout
  const adminPerms = getAdminPermissions();
  const canManage = $derived(adminPerms.hasPermission('admin.manage-roles'));
  const canManageUsers = $derived(adminPerms.hasPermission('admin.manage-users'));

  const roleName = $derived(data.roleName ?? '');

  let role = $state<Role | null>(null);
  let roleUsers = $state<User[]>([]);
  let showDeleteConfirm = $state(false);
  let error = $state<string | null>(null);

  // Form state for editing metadata
  let editDisplayName = $state('');
  let editDescription = $state('');

  // Load role metadata + users (the editor handles permissions independently).
  const roleQuery = useQuery(
    graphql(`
      query AdminRole($name: String!) {
        admin {
          role(name: $name) {
            name
            displayName
            description
            permissions
            permissionDenials
            isSystem
            position
          }
          instanceRoleUsers(roleName: $name) {
            id
            login
            displayName
          }
        }
      }
    `),
    () => ({ name: roleName }),
    {
      skip: () => !roleName,
      onCompleted: (data) => {
        role = data.admin?.role ?? null;
        roleUsers = data.admin?.instanceRoleUsers ?? [];
        if (role) {
          editDisplayName = role.displayName;
          editDescription = role.description;
        }
      }
    }
  );

  // Update role metadata mutation
  const updateRoleMutation = useMutation(
    graphql(`
      mutation UpdateRole($input: UpdateRoleInput!) {
        updateRole(input: $input) {
          name
          displayName
          description
        }
      }
    `)
  );

  async function saveMetadata() {
    if (!role || role.isSystem) return;

    error = null;

    const result = await updateRoleMutation.execute({
      input: {
        name: role.name,
        displayName: editDisplayName,
        description: editDescription
      }
    });

    if (result.error) {
      error = result.error;
    } else {
      await roleQuery.refetch();
    }
  }

  // Delete role mutation
  const deleteRoleMutation = useMutation(
    graphql(`
      mutation DeleteRole($input: DeleteRoleInput!) {
        deleteRole(input: $input)
      }
    `)
  );

  async function deleteRole() {
    if (!role || role.isSystem) return;

    error = null;

    const result = await deleteRoleMutation.execute({ input: { name: role.name } });

    if (result.error) {
      error = result.error;
      showDeleteConfirm = false;
    } else {
      goto(resolve('/chat/[instanceId]/admin/roles', { instanceId: instanceIdToSegment(getInstanceId()) }));
    }
  }

  const metadataChanged = $derived(
    role && (editDisplayName !== role.displayName || editDescription !== role.description)
  );
</script>

<PageTitle title={`${role?.displayName ?? 'Edit Role'} | Admin`} />

<PaneHeader
  title="Edit Role"
  subtitle={role?.displayName ?? 'Loading...'}
  backHref={resolve('/chat/[instanceId]/admin/roles', { instanceId: instanceIdToSegment(getInstanceId()) })}
  backLabel="Back to roles"
  showMobileNav
/>

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  {#if roleQuery.loading}
    <div class="text-muted">Loading role...</div>
  {:else if !role}
    <div class="text-danger">Role not found</div>
  {:else if !canManage}
    <div class="text-danger">
      You need the <code class="rounded bg-surface-200 px-1">admin.manage-roles</code> permission to edit
      roles.
    </div>
  {:else}
    {#if error}
      <FormError {error} />
    {/if}

    <!-- Role Metadata -->
    <Panel title="Role Details" icon="iconify uil--info-circle">
      <div class="flex flex-col gap-4">
        <div>
          <div class="mb-1 text-sm font-medium">Name</div>
          <code class="rounded bg-surface-200 px-2 py-1">{role.name}</code>
          <p class="mt-1 text-xs text-muted">Role names cannot be changed after creation.</p>
        </div>

        {#if role.isSystem}
          <div>
            <div class="mb-1 text-sm font-medium">Display Name</div>
            <div class="text-foreground">{role.displayName}</div>
          </div>
          <div>
            <div class="mb-1 text-sm font-medium">Description</div>
            <div class="text-muted">{role.description}</div>
          </div>
          <p class="text-sm text-muted">
            System role metadata cannot be modified. You can only change permissions (except for
            admin).
          </p>
        {:else}
          <TextInput
            id="displayName"
            testid="role-form-display-name"
            label="Display Name"
            bind:value={editDisplayName}
          />
          <TextArea
            id="description"
            testid="role-form-description"
            label="Description"
            bind:value={editDescription}
          />
          <div class="flex gap-2">
            <Button
              variant="primary"
              disabled={!metadataChanged || updateRoleMutation.loading}
              onclick={saveMetadata}
            >
              {updateRoleMutation.loading ? 'Saving...' : 'Save Changes'}
            </Button>
          </div>

          <!-- Delete Role -->
          <div class="mt-4 border-t border-border pt-4">
            <div class="mb-2 text-sm font-medium text-danger">Danger Zone</div>
            <p class="mb-3 text-sm text-muted">
              Deleting this role will remove it from all users who have it assigned.
            </p>
            <Button variant="danger" onclick={() => (showDeleteConfirm = true)}>Delete Role</Button>
          </div>
        {/if}
      </div>
    </Panel>

    <!-- Users with this role -->
    <Panel title="Users with this Role" icon="iconify uil--users-alt">
      {#if role?.name === 'everyone'}
        <p class="text-muted">
          All authenticated users are implicit members of this role. The everyone role cannot be
          explicitly assigned.
        </p>
      {:else}
        <UserList
          users={roleUsers}
          clickable={canManageUsers}
          emptyMessage="No users have this role"
        />
      {/if}
    </Panel>
  {/if}
</div>

<!-- Delete Confirmation Dialog -->
{#if showDeleteConfirm && role}
  <DeleteRoleModal
    roleDisplayName={role.displayName}
    deleting={deleteRoleMutation.loading}
    onConfirm={deleteRole}
    onCancel={() => (showDeleteConfirm = false)}
  />
{/if}
