<script lang="ts">
  import { goto } from '$app/navigation';
  import { getActiveInstanceSpaceId } from '$lib/state/activeInstance.svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { instanceIdToSegment } from '$lib/navigation';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { useConnection } from '$lib/state/instance/connection.svelte';
  import { graphql } from '$lib/gql';
  import { Panel } from '$lib/components/admin';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button, FormError } from '$lib/ui/form';
  import { RoleForm } from '$lib/components/rbac';

  const getInstanceId = getActiveInstance();
  const connection = useConnection();
  const spaceId = $derived(getActiveInstanceSpaceId()());

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
          instance {
            viewerCanManageRoles
          }
        }
      `),
      {}
    );

    if (resp.error || !resp.data?.instance) {
      error = 'Failed to load instance';
      loading = false;
      return;
    }

    canManageRoles = resp.data.instance.viewerCanManageRoles;
    loading = false;
  }

  $effect(() => {
    if (spaceId) {
      loadPermissions();
    }
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
    goto(resolve('/chat/[instanceId]/(chrome)/server-admin/roles/[name]', { instanceId: instanceIdToSegment(getInstanceId()), name: name.trim() }));
  }

  function goBack() {
    goto(resolve('/chat/[instanceId]/(chrome)/server-admin/roles', { instanceId: instanceIdToSegment(getInstanceId()) }));
  }
</script>

<PageTitle title="Create Role | Space Admin" />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader title="Create Role" subtitle="Create a new role for this space" showMobileNav>
    {#snippet actions()}
      <Button variant="secondary" onclick={goBack}>Cancel</Button>
    {/snippet}
  </PaneHeader>

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
