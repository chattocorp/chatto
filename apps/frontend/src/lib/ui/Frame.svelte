<!--
@component
Application content boundary. Desktop content appears raised in light mode and
recessed in dark mode. The shared overlay does not intercept pointer input.
Mobile content stays edge-to-edge.
-->
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
  class="flex min-h-0 flex-1 bg-background transition-opacity duration-300 md:app-frame-inset md:overflow-hidden md:rounded-2xl {className}"
>
  {@render children()}
</div>
