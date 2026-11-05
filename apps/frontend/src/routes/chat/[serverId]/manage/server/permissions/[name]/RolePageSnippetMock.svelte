<script lang="ts">
  import type { Snippet } from 'svelte';

  let { children, tabs }: { children?: Snippet; tabs?: Snippet } = $props();
</script>

{@render tabs?.()}
{@render children?.()}
