<script lang="ts">
  import type { RoleUser } from '$lib/api-client/roles';

  let { users }: { users: RoleUser[] } = $props();
</script>

<div data-testid="role-users">
  {#each users as user (user.id)}
    <span>{user.displayName}</span>
  {/each}
</div>
