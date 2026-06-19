<script lang="ts">
  import { afterNavigate, goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { Panel, UserList } from '$lib/components/admin';
  import { Hint } from '$lib/ui';
  import { toast } from '$lib/ui/toast';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button, Checkbox, TextInput, TextArea, FormError } from '$lib/ui/form';
  import { DeleteRoleModal, RolePermissionsMatrix, type Role } from '$lib/components/rbac';
  import {
    DeleteAdminRoleRequest,
    GetAdminRoleRequest,
    UpdateAdminRoleRequest
  } from '$lib/pb/chatto/api/v1/chat_pb';
  import type { AdminRoleView } from '$lib/pb/chatto/api/v1/chat_pb';
  import type { User } from '$lib/pb/chatto/core/v1/models_pb';
  import { withActiveServerWireClient } from '$lib/wire/activeServerClient';

  type RoleUser = { id: string; login: string; displayName: string };

  const serverSegment = $derived(serverIdToSegment(getActiveServer()));
  const roleName = $derived(page.params.name!);

  let role = $state<Role | null>(null);
  let roleUsers = $state<RoleUser[]>([]);
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
  let requestId = 0;
  let loadedRoleName = '';

  async function loadData() {
    const targetRoleName = roleName;
    if (!targetRoleName) return;

    const currentRequest = ++requestId;
    loading = true;
    error = null;

    try {
      const resp = await withActiveServerWireClient((client) =>
        client.getAdminRole(new GetAdminRoleRequest({ name: targetRoleName }))
      );
      if (currentRequest !== requestId) return;

      role = roleFromWire(resp.role);
      roleUsers = resp.users.map(userFromWire);
      canManageRoles = resp.viewerCanManageRoles;
      canAssignRoles = resp.viewerCanAssignRoles;

      if (role) {
        editDisplayName = role.displayName;
        editDescription = role.description;
        editPingable = role.pingable;
      }
      loadedRoleName = targetRoleName;
    } catch (e) {
      if (currentRequest !== requestId) return;
      error = e instanceof Error ? e.message : 'Failed to load role';
      role = null;
    } finally {
      if (currentRequest === requestId) {
        loading = false;
      }
    }
  }

  afterNavigate(() => {
    if (roleName && roleName !== loadedRoleName) {
      void loadData();
    }
  });

  async function saveMetadata() {
    if (!role || savingPingable) return;

    saving = true;
    error = null;

    try {
      const resp = await withActiveServerWireClient((client) =>
        client.updateAdminRole(
          new UpdateAdminRoleRequest({
            name: role?.name ?? '',
            displayName: editDisplayName,
            description: editDescription
          })
        )
      );
      const updated = roleFromWire(resp.role);
      if (updated) {
        role = updated;
        editDisplayName = updated.displayName;
        editDescription = updated.description;
        editPingable = updated.pingable;
      }
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to update role';
    } finally {
      saving = false;
    }
  }

  async function savePingable(event: Event) {
    if (!role || !canEditPingable || saving) return;

    const target = event.currentTarget as HTMLInputElement;
    const nextPingable = target.checked;
    const previousPingable = role.pingable;

    if (nextPingable === previousPingable) return;

    savingPingable = true;
    error = null;

    try {
      const resp = await withActiveServerWireClient((client) =>
        client.updateAdminRole(
          new UpdateAdminRoleRequest({
            name: role?.name ?? '',
            displayName: role?.displayName ?? '',
            description: role?.description ?? '',
            pingable: nextPingable
          })
        )
      );
      const updated = roleFromWire(resp.role);
      if (!updated) {
        throw new Error('Failed to update role ping setting');
      }
      role = updated;
      editPingable = updated.pingable;
      toast.success(updated.pingable ? 'Role pings enabled' : 'Role pings disabled');
    } catch (e) {
      editPingable = previousPingable;
      error = e instanceof Error ? e.message : 'Failed to update role ping setting';
    } finally {
      savingPingable = false;
    }
  }

  async function deleteRole() {
    if (!role || role.isSystem) return;

    deleting = true;
    error = null;

    try {
      await withActiveServerWireClient((client) =>
        client.deleteAdminRole(new DeleteAdminRoleRequest({ name: role?.name ?? '' }))
      );
      goto(resolve('/chat/[serverId]/server-admin/permissions', { serverId: serverSegment }));
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to delete role';
      deleting = false;
      showDeleteConfirm = false;
    }
  }

  const permissionsHref = $derived(
    resolve('/chat/[serverId]/server-admin/permissions', { serverId: serverSegment })
  );

  const metadataChanged = $derived(
    role && (editDisplayName !== role.displayName || editDescription !== role.description)
  );
  const canEditPingable = $derived(role?.name !== 'everyone');

  function roleFromWire(value: AdminRoleView | undefined): Role | null {
    if (!value) return null;
    return {
      name: value.name,
      displayName: value.displayName,
      description: value.description,
      permissions: [...value.permissions],
      permissionDenials: [...value.permissionDenials],
      isSystem: value.isSystem,
      position: value.position,
      pingable: value.pingable
    };
  }

  function userFromWire(value: User): RoleUser {
    return {
      id: value.id,
      login: value.login,
      displayName: value.displayName
    };
  }
</script>

<PageTitle title={`${role?.displayName ?? 'Edit Role'} | Space Admin`} />

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
            at resolution time — use the per-user matrix to see what an individual user ends up with.
          {/if}
        </Hint>
        <RolePermissionsMatrix roleName={role.name} />
      {/if}

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
