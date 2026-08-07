<script lang="ts">
  import type { Snippet } from 'svelte';

  let { children }: { children?: Snippet } = $props();
</script>

<div data-testid="chat-root-component-stub">
  {#if children}
    {@render children()}
  {/if}
</div>
