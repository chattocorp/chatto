<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { instanceIdToSegment } from '$lib/navigation';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { useConnection } from '$lib/state/instance/connection.svelte';
  import { graphql } from '$lib/gql';
  import { Panel, UserList } from '$lib/components/admin';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button, TextInput, TextArea, FormError } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import {
    DeleteRoleModal,
    RolePermissionEditor,
    type Role
  } from '$lib/components/rbac';

  type User = { id: string; login: string; displayName: string };

  const getInstanceId = getActiveInstance();
  const instanceSegment = $derived(instanceIdToSegment(getInstanceId()));
  const connection = useConnection();
  const spaceId = $derived(page.params.spaceId!);
  const roleName = $derived(page.params.name!);

  let role = $state<Role | null>(null);
  let roleUsers = $state<User[]>([]);
  let canManageRoles = $state(false);
  let canAssignRoles = $state(false);
  let loading = $state(true);
  let saving = $state(false);
  let deleting = $state(false);
  let showDeleteConfirm = $state(false);
  let error = $state<string | null>(null);

  // Form state for editing metadata
  let editDisplayName = $state('');
  let editDescription = $state('');

  async function loadData() {
    loading = true;
    error = null;

    // Metadata + users + viewer permissions. The editor handles its own
    // permission tier loading via the unified rolePermissions query.
    const resp = await connection().client.query(
      graphql(`
        query SpaceRoleDetail($spaceId: ID!, $name: String!) {
          space(id: $spaceId) {
            id
            name
            role(name: $name) {
              name
              displayName
              description
              permissions
              permissionDenials
              isSystem
              position
            }
            roleUsers(roleName: $name) {
              id
              login
              displayName
            }
            viewerCanManageRoles
            viewerCanAssignRoles
          }
        }
      `),
      { spaceId, name: roleName }
    );

    if (resp.error) {
      error = resp.error.message;
      loading = false;
      return;
    }

    if (!resp.data?.space) {
      error = 'Space not found';
      loading = false;
      return;
    }

    role = resp.data.space.role ?? null;
    roleUsers = resp.data.space.roleUsers;
    canManageRoles = resp.data.space.viewerCanManageRoles;
    canAssignRoles = resp.data.space.viewerCanAssignRoles;

    if (role) {
      editDisplayName = role.displayName;
      editDescription = role.description;
    }

    loading = false;
  }

  $effect(() => {
    if (spaceId && roleName) {
      loadData();
    }
  });

  async function saveMetadata() {
    if (!role || role.isSystem) return;

    saving = true;
    error = null;

    const resp = await connection().client.mutation(
      graphql(`
        mutation UpdateSpaceRole($input: UpdateSpaceRoleInput!) {
          updateSpaceRole(input: $input) {
            name
            displayName
            description
          }
        }
      `),
      {
        input: {
          spaceId,
          name: role.name,
          displayName: editDisplayName,
          description: editDescription
        }
      }
    );

    if (resp.error) {
      error = resp.error.message;
    } else {
      // Reload data
      await loadData();
    }

    saving = false;
  }

  async function deleteRole() {
    if (!role || role.isSystem) return;

    deleting = true;
    error = null;

    const resp = await connection().client.mutation(
      graphql(`
        mutation DeleteSpaceRole($input: DeleteSpaceRoleInput!) {
          deleteSpaceRole(input: $input)
        }
      `),
      { input: { spaceId, name: role.name } }
    );

    if (resp.error) {
      error = resp.error.message;
      deleting = false;
      showDeleteConfirm = false;
    } else {
      // Navigate back to roles list
      goto(resolve('/chat/[instanceId]/[spaceId]/admin/roles', { instanceId: instanceSegment, spaceId }));
    }
  }

  const rolesHref = $derived(
    resolve('/chat/[instanceId]/[spaceId]/admin/roles', { instanceId: instanceSegment, spaceId })
  );

  const metadataChanged = $derived(
    role && (editDisplayName !== role.displayName || editDescription !== role.description)
  );
</script>

<PageTitle title={`${role?.displayName ?? 'Edit Role'} | Space Admin`} />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title="Edit Role"
    subtitle={role?.displayName ?? 'Loading...'}
    backHref={rolesHref}
    backLabel="Back to roles"
    showMobileNav
  />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    {#if loading}
      <div class="text-muted">Loading role...</div>
    {:else if !role}
      <div class="text-danger">Role not found</div>
    {:else if !canManageRoles}
      <div class="text-danger">
        You need the <code class="rounded bg-surface-200 px-1">roles.manage</code> permission to edit
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
            <p class="text-sm text-muted">System role metadata cannot be modified.</p>
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
                disabled={!metadataChanged || saving}
                onclick={saveMetadata}
              >
                {saving ? 'Saving...' : 'Save Changes'}
              </Button>
            </div>

            <!-- Delete Role -->
            <div class="mt-4 border-t border-border pt-4">
              <div class="mb-2 text-sm font-medium text-danger">Danger Zone</div>
              <p class="mb-3 text-sm text-muted">
                Deleting this role will remove it from all users who have it assigned.
              </p>
              <Button variant="danger" onclick={() => (showDeleteConfirm = true)}>
                Delete Role
              </Button>
            </div>
          {/if}
        </div>
      </Panel>

      <!-- Permissions -->
      <RolePermissionEditor
        {roleName}
        {spaceId}
        categoryOrder={['member', 'role', 'space', 'room', 'message']}
      />

      <!-- Users with this role -->
      <Panel title="Users with this Role" icon="iconify uil--users-alt">
        {#if role?.name === 'everyone'}
          <p class="text-muted">All space members have the everyone role implicitly.</p>
        {:else}
          <UserList
            users={roleUsers}
            clickable={canAssignRoles}
            emptyMessage="No users have this role"
            onUserClick={(user) =>
              goto(resolve('/chat/[instanceId]/[spaceId]/admin/members/[userId]', { instanceId: instanceSegment, spaceId, userId: user.id }))}
          />
        {/if}
      </Panel>
    {/if}
  </div>
</div>

<!-- Delete Confirmation Dialog -->
{#if showDeleteConfirm && role}
  <DeleteRoleModal
    roleDisplayName={role.displayName}
    {deleting}
    onConfirm={deleteRole}
    onCancel={() => (showDeleteConfirm = false)}
  />
{/if}
