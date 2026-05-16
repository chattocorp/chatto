<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { graphql } from '$lib/gql';
  import { RolePermissionsMatrix } from '$lib/components/rbac';
  import { Hint } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';

  const connection = useConnection();
  const roleName = $derived(page.params.name!);
  const serverSegment = $derived(serverIdToSegment(getActiveServer()));

  let displayName = $state<string | null>(null);
  let loadError = $state<string | null>(null);

  async function loadRole(name: string) {
    const resp = await connection().client.query(
      graphql(`
        query RolePermissionsHeader($name: String!) {
          server {
            role(name: $name) {
              name
              displayName
            }
          }
        }
      `),
      { name }
    );
    if (name !== roleName) return;
    if (resp.error) {
      loadError = resp.error.message;
      return;
    }
    const r = resp.data?.server?.role;
    if (!r) {
      loadError = 'Role not found.';
      return;
    }
    displayName = r.displayName;
  }

  $effect(() => {
    if (roleName) void loadRole(roleName);
  });
</script>

<PageTitle title={`${displayName ?? 'Role'} permissions | Server Admin`} />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title="Role Permissions"
    subtitle={displayName ? `${displayName} (@${roleName})` : 'Loading…'}
    backHref={resolve('/chat/[serverId]/(chrome)/server-admin/roles/[name]', {
      serverId: serverSegment,
      name: roleName
    })}
    backLabel="Back to role"
    showMobileNav
  />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    {#if loadError}
      <Hint tone="danger">{loadError}</Hint>
    {:else}
      <Hint tone="info">
        Each cell shows this role's explicit override at a scope (solid) layered over the role's
        baseline inherited from broader scopes (faded). Click a cell to cycle
        <strong>none → allow → deny → none</strong>. The role's grants combine with other roles a
        user holds — use the per-user matrix to see what an individual user ends up with.
      </Hint>
      <RolePermissionsMatrix {roleName} />
    {/if}
  </div>
</div>
