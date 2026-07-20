<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { createRoleAPI } from '$lib/api-client/roles';
  import { Panel } from '$lib/components/admin';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { FormError } from '$lib/ui/form';
  import { RoleForm } from '$lib/components/rbac';
  import * as m from '$lib/i18n/messages';

  const connection = useConnection();

  let name = $state('');
  let displayName = $state('');
  let description = $state('');
  let pingable = $state(false);
  let creating = $state(false);
  let error = $state<string | null>(null);
  let canManageRoles = $state(false);
  let loading = $state(true);

  async function loadPermissions() {
    loading = true;

    try {
      const resp = await roleAPI().listAdminRoles();
      canManageRoles = resp.viewerCanManageRoles;
    } catch {
      error = m['admin.permissions.load_instance_failed']();
      loading = false;
      return;
    }

    loading = false;
  }

  $effect(() => {
    loadPermissions();
  });

  async function createRole() {
    creating = true;
    error = null;

    try {
      await roleAPI().createRole({
        name: name.trim(),
        displayName: displayName.trim(),
        description: description.trim(),
        pingable
      });
    } catch (err) {
      error = err instanceof Error ? err.message : m['admin.permissions.load_instance_failed']();
      creating = false;
      return;
    }

    // Navigate to the new role's detail page
    goto(
      resolve('/chat/[serverId]/manage/server/permissions/[name]', {
        serverId: serverIdToSegment(getActiveServer()),
        name: name.trim()
      })
    );
  }

  function roleAPI() {
    const conn = connection();
    return createRoleAPI({
      baseUrl: conn.connectBaseUrl,
      bearerToken: conn.bearerToken
    });
  }
</script>

<PageTitle
  title={m['admin.common.server_admin_page_title']({
    title: m['admin.permissions.create_role_title']()
  })}
/>

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title={m['admin.permissions.create_role_title']()}
    subtitle={m['admin.permissions.create_role_subtitle']()}
    backHref={resolve('/chat/[serverId]/manage/server/permissions', {
      serverId: serverIdToSegment(getActiveServer())
    })}
    backLabel={m['admin.permissions.back_to_permissions']()}
    showMobileNav
  />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    {#if loading}
      <div class="text-muted">{m['admin.common.loading']()}</div>
    {:else if !canManageRoles}
      <div class="text-danger">
        {m['admin.permissions.need_manage_create']()}
      </div>
    {:else}
      {#if error}
        <FormError {error} />
      {/if}

      <Panel title={m['admin.common.role_details']()} icon="iconify uil--plus-circle">
        <RoleForm
          bind:name
          bind:displayName
          bind:description
          bind:pingable
          saving={creating}
          submitLabel={m['admin.permissions.create_role_action']()}
          savingLabel={m['admin.permissions.creating_role']()}
          onSubmit={createRole}
        />
        <p class="mt-4 text-sm text-muted">
          {m['admin.permissions.create_after_hint']()}
        </p>
      </Panel>
    {/if}
  </div>
</div>
