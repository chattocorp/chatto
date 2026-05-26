<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { graphql } from '$lib/gql';
  import { Panel } from '$lib/components/admin';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { FormError } from '$lib/ui/form';
  import { RoleForm } from '$lib/components/rbac';

  const connection = useConnection();

  let name = $state('');
  let displayName = $state('');
  let description = $state('');
  let creating = $state(false);
  let error = $state<string | null>(null);
  let canManageRoles = $state(false);
  let loading = $state(true);

  async function loadPermissions() {
    loading = true;

    const resp = await connection().client.query(
      graphql(`
        query SpaceRolesNewCheck {
          server {
            viewerCanManageRoles
          }
        }
      `),
      {}
    );

    if (resp.error || !resp.data?.server) {
      error = 'Failed to load instance';
      loading = false;
      return;
    }

    canManageRoles = resp.data.server.viewerCanManageRoles;
    loading = false;
  }

  $effect(() => {
    loadPermissions();
  });

  async function createRole() {
    creating = true;
    error = null;

    const resp = await connection().client.mutation(
      graphql(`
        mutation CreateRoleNewPage($input: CreateRoleInput!) {
          createRole(input: $input) {
            name
            displayName
            description
          }
        }
      `),
      {
        input: {
          name: name.trim(),
          displayName: displayName.trim(),
          description: description.trim()
        }
      }
    );

    if (resp.error) {
      error = resp.error.message;
      creating = false;
      return;
    }

    // Navigate to the new role's detail page
    goto(resolve('/chat/[serverId]/server-admin/roles/[name]', { serverId: serverIdToSegment(getActiveServer()), name: name.trim() }));
  }

</script>

<PageTitle title="Create Role | Space Admin" />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title="Create Role"
    subtitle="Create a new role for this space"
    backHref={resolve('/chat/[serverId]/server-admin/roles', { serverId: serverIdToSegment(getActiveServer()) })}
    backLabel="Back to roles"
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
