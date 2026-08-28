<script lang="ts">
  import type { Snippet } from 'svelte';

  let { actions }: { actions?: Snippet } = $props();
</script>

{#if actions}
  {@render actions()}
{/if}
