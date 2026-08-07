<script lang="ts">
  import type { Snippet } from 'svelte';

  let { children }: { children?: Snippet } = $props();
</script>

<div data-testid="server-chrome">
  {@render children?.()}
</div>
