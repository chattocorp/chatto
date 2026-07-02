<!--
@component

Hover toolbar for call participant tiles. It intentionally reuses Chatto's
quiet surface and border language instead of media-player chrome.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    testId,
    forceVisible = false,
    children
  }: {
    testId?: string;
    forceVisible?: boolean;
    children: Snippet;
  } = $props();
</script>

<div
  class={[
    'call-tile-action-toolbar pointer-events-none absolute top-3.5 right-2 z-10 flex gap-0.5 rounded-md border border-border bg-surface-100/95 p-0.5 opacity-0 transition-opacity group-hover/media:opacity-100 group-focus-within/media:opacity-100',
    forceVisible && 'opacity-100'
  ]}
  data-testid={testId}
>
  {@render children()}
</div>
