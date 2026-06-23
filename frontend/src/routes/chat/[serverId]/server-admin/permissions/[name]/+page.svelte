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
  import * as m from '$lib/i18n/messages';

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

<PageTitle
  title={m['admin.common.server_admin_page_title']({
    title: role?.displayName ?? m['admin.permissions.edit_role_title']()
  })}
/>

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title={m['admin.permissions.edit_role_title']()}
    subtitle={role?.displayName ?? m['common.loading']()}
    backHref={permissionsHref}
    backLabel={m['admin.permissions.back_to_permissions']()}
    showMobileNav
  />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    {#if loading}
      <div class="text-muted">{m['admin.permissions.loading_role']()}</div>
    {:else if !role}
      <div class="text-danger">{m['admin.permissions.role_not_found']()}</div>
    {:else if !canManageRoles}
      <div class="text-danger">
        {m['admin.permissions.need_manage_edit']()}
      </div>
    {:else}
      {#if error}
        <FormError {error} />
      {/if}

      <!-- Role Metadata -->
      <Panel title={m['admin.common.role_details']()} icon="iconify uil--info-circle">
        <div class="flex flex-col gap-4">
          <div>
            <div class="mb-1 text-sm font-medium">{m['rbac.role_form.name']()}</div>
            <code class="rounded bg-surface-200 px-2 py-1">{role.name}</code>
            <p class="mt-1 text-xs text-muted">{m['rbac.role_form.name_locked']()}</p>
          </div>

          {#if role.isSystem}
            <div>
              <div class="mb-1 text-sm font-medium">{m['rbac.role_form.display_name']()}</div>
              <div class="text-text">{role.displayName}</div>
            </div>
            <div>
              <div class="mb-1 text-sm font-medium">{m['rbac.role_form.description']()}</div>
              <div class="text-muted">{role.description}</div>
            </div>
            <p class="text-sm text-muted">{m['admin.permissions.system_metadata_locked']()}</p>
            <Checkbox
              id="pingable"
              bind:checked={editPingable}
              label={m['rbac.role_form.pingable']()}
              onchange={savePingable}
              disabled={saving || savingPingable || !canEditPingable}
              description={canEditPingable
                ? m['rbac.role_form.pingable_description']()
                : m['admin.permissions.everyone_pingable_description']()}
            />
          {:else}
            <TextInput
              id="displayName"
              testid="role-form-display-name"
              label={m['rbac.role_form.display_name']()}
              bind:value={editDisplayName}
            />
            <TextArea
              id="description"
              testid="role-form-description"
              label={m['rbac.role_form.description']()}
              bind:value={editDescription}
            />
            <Checkbox
              id="pingable"
              bind:checked={editPingable}
              label={m['rbac.role_form.pingable']()}
              onchange={savePingable}
              disabled={saving || savingPingable || !canEditPingable}
              description={canEditPingable
                ? m['rbac.role_form.pingable_description']()
                : m['admin.permissions.everyone_pingable_description']()}
            />
            <div class="flex gap-2">
              <Button
                variant="primary"
                disabled={!metadataChanged || saving || savingPingable}
                onclick={saveMetadata}
              >
                {saving ? m['rbac.role_form.saving']() : m['admin.permissions.save_changes']()}
              </Button>
            </div>

            <!-- Delete Role -->
            <div class="mt-4 border-t border-border pt-4">
              <div class="mb-2 text-sm font-medium text-danger">
                {m['admin.common.danger_zone']()}
              </div>
              <p class="mb-3 text-sm text-muted">
                {m['admin.permissions.delete_role_description']()}
              </p>
              <Button variant="danger" onclick={() => (showDeleteConfirm = true)}>
                {m['rbac.delete_role.action']()}
              </Button>
            </div>
          {/if}
        </div>
      </Panel>

      <!-- Permissions matrix: full per-role allow/deny across server, groups, and rooms. -->
      {#if canManageRoles && role}
        <Hint>
          {#if role.name === 'owner'}
            {m['admin.permissions.owner_permissions_hint']()}
          {:else}
            {m['admin.permissions.role_permissions_hint']()}
          {/if}
        </Hint>
        <RolePermissionsMatrix roleName={role.name} />
      {/if}

      <!-- Users with this role -->
      <Panel title={m['admin.permissions.users_with_role']()} icon="iconify uil--users-alt">
        {#if role?.name === 'everyone'}
          <p class="text-muted">{m['admin.permissions.everyone_implicit']()}</p>
        {:else}
          <UserList
            users={roleUsers}
            clickable={canAssignRoles}
            emptyMessage={m['admin.permissions.no_users_with_role']()}
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
