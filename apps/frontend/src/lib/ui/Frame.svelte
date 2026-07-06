<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    children,
    class: className = ''
  }: {
    children: Snippet;
    class?: string;
  } = $props();
</script>

<div
  class="flex min-h-0 flex-1 bg-background transition-opacity duration-300 md:overflow-hidden md:rounded-2xl md:shadow-lg md:ring-1 md:ring-surface-200/40 {className}"
>
  {@render children()}
</div>
