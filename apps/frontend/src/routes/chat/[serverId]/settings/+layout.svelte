<script lang="ts">
  import { useServerScope } from '$lib/state/server/scope.svelte';

  let { children } = $props();
  const scope = useServerScope();
</script>

<!-- Account forms initialize drafts only from complete live account data.
     Refreshing that data keeps the mounted forms and their edits intact. -->
{#if scope.store.currentUser.user}
  {@render children?.()}
{/if}
