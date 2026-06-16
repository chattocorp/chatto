<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { graphql } from '$lib/gql';
  import { Panel, UserList } from '$lib/components/admin';
  import { Hint } from '$lib/ui';
  import { toast } from '$lib/ui/toast';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button, Checkbox, TextInput, TextArea, FormError } from '$lib/ui/form';
  import { DeleteRoleModal, RolePermissionsMatrix, type Role } from '$lib/components/rbac';

  type User = { id: string; login: string; displayName: string };

  const serverSegment = $derived(serverIdToSegment(getActiveServer()));
  const connection = useConnection();
  const roleName = $derived(page.params.name!);

  let role = $state<Role | null>(null);
  let roleUsers = $state<User[]>([]);
  let canManageRoles = $state(false);
  let canAssignRoles = $state(false);
  let loading = $state(true);
  let saving = $state(false);
  let savingPingable = $state(false);
  let deleting = $state(false);
  let showDeleteConfirm = $state(false);
  let error = $state<string | null>(null);

  // Form state for editing metadata
  let editDisplayName = $state('');
  let editDescription = $state('');
  let editPingable = $state(false);

  const UpdateRoleDetailPageMutation = graphql(`
    mutation UpdateRoleDetailPage($input: UpdateRoleInput!) {
      updateRole(input: $input) {
        name
        displayName
        description
        pingable
      }
    }
  `);

  async function loadData() {
    loading = true;
    error = null;

    // Metadata + users + viewer permissions. The editor handles its own
    // permission loading through the RBAC matrix query.
    const resp = await connection().client.query(
      graphql(`
        query SpaceRoleDetail($name: String!) {
          server {
            role(name: $name) {
              name
              displayName
              description
              permissions
              permissionDenials
              isSystem
              position
              pingable
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
      { name: roleName }
    );

    if (resp.error) {
      error = resp.error.message;
      loading = false;
      return;
    }

    if (!resp.data?.server) {
      error = 'Server not found';
      loading = false;
      return;
    }

    role = resp.data.server.role ?? null;
    roleUsers = resp.data.server.roleUsers;
    canManageRoles = resp.data.server.viewerCanManageRoles;
    canAssignRoles = resp.data.server.viewerCanAssignRoles;

    if (role) {
      editDisplayName = role.displayName;
      editDescription = role.description;
      editPingable = role.pingable;
    }

    loading = false;
  }

  $effect(() => {
    if (roleName) {
      loadData();
    }
  });

  async function saveMetadata() {
    if (!role || savingPingable) return;

    saving = true;
    error = null;

    const resp = await connection().client.mutation(UpdateRoleDetailPageMutation, {
      input: {
        name: role.name,
        displayName: editDisplayName,
        description: editDescription
      }
    });

    if (resp.error) {
      error = resp.error.message;
    } else {
      // Reload data
      await loadData();
    }

    saving = false;
  }

  async function savePingable(event: Event) {
    if (!role || !canEditPingable || saving) return;

    const target = event.currentTarget as HTMLInputElement;
    const nextPingable = target.checked;
    const previousPingable = role.pingable;

    if (nextPingable === previousPingable) return;

    savingPingable = true;
    error = null;

    const resp = await connection().client.mutation(UpdateRoleDetailPageMutation, {
      input: {
        name: role.name,
        displayName: role.displayName,
        description: role.description,
        pingable: nextPingable
      }
    });

    if (resp.error || !resp.data?.updateRole) {
      editPingable = previousPingable;
      error = resp.error?.message ?? 'Failed to update role ping setting';
    } else {
      role = {
        ...role,
        pingable: resp.data.updateRole.pingable
      };
      editPingable = resp.data.updateRole.pingable;
      toast.success(resp.data.updateRole.pingable ? 'Role pings enabled' : 'Role pings disabled');
    }

    savingPingable = false;
  }

  async function deleteRole() {
    if (!role || role.isSystem) return;

    deleting = true;
    error = null;

    const resp = await connection().client.mutation(
      graphql(`
        mutation DeleteRoleDetailPage($input: DeleteRoleInput!) {
          deleteRole(input: $input)
        }
      `),
      { input: { name: role.name } }
    );

    if (resp.error) {
      error = resp.error.message;
      deleting = false;
      showDeleteConfirm = false;
    } else {
      // Navigate back to permissions list
      goto(resolve('/chat/[serverId]/server-admin/permissions', { serverId: serverSegment }));
    }
  }

  const permissionsHref = $derived(
    resolve('/chat/[serverId]/server-admin/permissions', { serverId: serverSegment })
  );

  const metadataChanged = $derived(
    role && (editDisplayName !== role.displayName || editDescription !== role.description)
  );
  const canEditPingable = $derived(role?.name !== 'everyone');
</script>

<PageTitle title={`${role?.displayName ?? 'Edit Role'} | Server Admin`} />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title="Edit Role"
    subtitle={role?.displayName ?? 'Loading...'}
    backHref={permissionsHref}
    backLabel="Back to permissions"
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
            <Checkbox
              id="pingable"
              bind:checked={editPingable}
              label="Allow people to ping this role"
              onchange={savePingable}
              disabled={saving || savingPingable || !canEditPingable}
              description={canEditPingable
                ? 'Pingable roles appear in @ autocomplete and notify assigned room members.'
                : 'Use @all for room-wide delivery; everyone is not a role ping handle.'}
            />
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
            <Checkbox
              id="pingable"
              bind:checked={editPingable}
              label="Allow people to ping this role"
              onchange={savePingable}
              disabled={saving || savingPingable || !canEditPingable}
              description={canEditPingable
                ? 'Pingable roles appear in @ autocomplete and notify assigned room members.'
                : 'Use @all for room-wide delivery; everyone is not a role ping handle.'}
            />
            <div class="flex gap-2">
              <Button
                variant="primary"
                disabled={!metadataChanged || saving || savingPingable}
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

      <!-- Permissions matrix: full per-role allow/deny across server, groups, and rooms. -->
      {#if canManageRoles && role}
        <Hint>
          {#if role.name === 'owner'}
            Owners are always granted all permissions. The matrix is read-only because owner
            permissions are not stored as editable grants or denials.
          {:else}
            This role's grants and denials across every scope. Combined with the user's other roles
            at resolution time — use the per-user matrix to see what an individual user ends up
            with.
          {/if}
        </Hint>
        <RolePermissionsMatrix roleName={role.name} />
      {/if}

      <!-- Users with this role -->
      <Panel title="Users with this Role" icon="iconify uil--users-alt">
        {#if role?.name === 'everyone'}
          <p class="text-muted">All server members have the everyone role implicitly.</p>
        {:else}
          <UserList
            users={roleUsers}
            clickable={canAssignRoles}
            emptyMessage="No users have this role"
            onUserClick={(user) =>
              goto(
                resolve('/chat/[serverId]/server-admin/members/[userId]', {
                  serverId: serverSegment,
                  userId: user.id
                })
              )}
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
