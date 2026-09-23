<!--
@component
Application content boundary. Desktop content appears raised in light mode and
recessed in dark mode. The shared overlay does not intercept pointer input.
Narrow touch-capable windows keep content edge-to-edge. Mouse windows keep the
frame at every width. Pane and navigation layouts still depend on available space.
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
  data-testid="app-frame"
  class="flex min-h-0 flex-1 bg-background transition-opacity duration-300 desktop-presentation:app-frame-inset desktop-presentation:overflow-hidden desktop-presentation:rounded-2xl {className}"
>
  {@render children()}
</div>
