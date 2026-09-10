<script lang="ts">
  import type { RoleUser } from '$lib/api-client/roles';

  let {
    users,
    hasMore,
    onLoadMore
  }: { users: RoleUser[]; hasMore?: boolean; onLoadMore?: () => void } = $props();
</script>

<div data-testid="role-users">
  {#each users as user (user.id)}
    <span>{user.displayName}</span>
  {/each}
</div>

{#if hasMore}<button onclick={onLoadMore}>Load next test page</button>{/if}
