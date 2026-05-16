<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { graphql } from '$lib/gql';
  import { UserPermissionsMatrix } from '$lib/components/rbac';
  import { Hint } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';

  const connection = useConnection();
  const userId = $derived(page.params.userId!);
  const serverSegment = $derived(serverIdToSegment(getActiveServer()));

  let displayName = $state<string | null>(null);
  let login = $state<string | null>(null);
  let loadError = $state<string | null>(null);

  async function loadMember(uid: string) {
    const resp = await connection().client.query(
      graphql(`
        query UserPermissionsHeader($userId: ID!) {
          user(id: $userId) {
            id
            login
            displayName
          }
        }
      `),
      { userId: uid }
    );
    if (uid !== userId) return;
    if (resp.error) {
      loadError = resp.error.message;
      return;
    }
    if (!resp.data?.user) {
      loadError = 'User not found.';
      return;
    }
    displayName = resp.data.user.displayName;
    login = resp.data.user.login;
  }

  $effect(() => {
    if (userId) void loadMember(userId);
  });
</script>

<PageTitle title={`${displayName ?? 'User'} permissions | Server Admin`} />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title="User Permissions"
    subtitle={displayName ? `${displayName}${login ? ` (@${login})` : ''}` : 'Loading…'}
    backHref={resolve('/chat/[serverId]/(chrome)/server-admin/members/[userId]', {
      serverId: serverSegment,
      userId
    })}
    backLabel="Back to member"
    showMobileNav
  />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    {#if loadError}
      <Hint tone="danger">{loadError}</Hint>
    {:else}
      <Hint tone="info">
        Each cell shows this user's explicit override at a scope (solid) layered over the role-derived
        baseline (faded). Click a cell to cycle <strong>none → allow → deny → none</strong>. User-level
        overrides outrank every role grant.
      </Hint>
      <UserPermissionsMatrix {userId} />
    {/if}
  </div>
</div>
