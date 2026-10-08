<script lang="ts">
  import type { Snippet } from 'svelte';

  let { title, tabs }: { title: string; tabs?: Snippet } = $props();
</script>

<h1>{title}</h1>
{@render tabs?.()}
