<script lang="ts">
  let { userId }: { userId: string } = $props();
</script>

<div data-testid="user-permissions" data-user-id={userId}></div>
