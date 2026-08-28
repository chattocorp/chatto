<script lang="ts">
  let { user }: { user: { displayName: string } } = $props();
</script>

<span data-testid="user-avatar">{user.displayName}</span>
