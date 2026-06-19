<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { Panel } from '$lib/components/admin';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { FormError } from '$lib/ui/form';
  import { RoleForm } from '$lib/components/rbac';
  import {
    CreateAdminRoleRequest,
    GetAdminRoleCapabilitiesRequest
  } from '$lib/pb/chatto/api/v1/chat_pb';
  import { withActiveServerWireClient } from '$lib/wire/activeServerClient';

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
    error = null;

    try {
      const resp = await withActiveServerWireClient((client) =>
        client.getAdminRoleCapabilities(new GetAdminRoleCapabilitiesRequest())
      );
      canManageRoles = resp.viewerCanManageRoles;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load server role permissions';
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    void loadPermissions();
  });

  async function createRole() {
    creating = true;
    error = null;

    try {
      await withActiveServerWireClient((client) =>
        client.createAdminRole(
          new CreateAdminRoleRequest({
            name: name.trim(),
            displayName: displayName.trim(),
            description: description.trim(),
            pingable
          })
        )
      );

      goto(
        resolve('/chat/[serverId]/server-admin/permissions/[name]', {
          serverId: serverIdToSegment(getActiveServer()),
          name: name.trim()
        })
      );
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to create role';
      creating = false;
    }
  }
</script>

<PageTitle title="Create Role | Space Admin" />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title="Create Role"
    subtitle="Create a new role for this space"
    backHref={resolve('/chat/[serverId]/server-admin/permissions', {
      serverId: serverIdToSegment(getActiveServer())
    })}
    backLabel="Back to permissions"
    showMobileNav
  />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    {#if loading}
      <div class="text-muted">Loading...</div>
    {:else if !canManageRoles}
      <div class="text-danger">
        You need the <code class="rounded bg-surface-200 px-1">roles.manage</code> permission to create
        roles.
      </div>
    {:else}
      {#if error}
        <FormError {error} />
      {/if}

      <Panel title="Role Details" icon="iconify uil--plus-circle">
        <RoleForm
          bind:name
          bind:displayName
          bind:description
          bind:pingable
          saving={creating}
          submitLabel="Create Role"
          savingLabel="Creating..."
          onSubmit={createRole}
        />
        <p class="mt-4 text-sm text-muted">
          After creating the role, you can assign permissions to it on the edit page.
        </p>
      </Panel>
    {/if}
  </div>
</div>
