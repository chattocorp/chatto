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
    'call-tile-action-toolbar pointer-events-none flex shrink-0 gap-0.5 rounded-md border border-text/10 bg-surface-100 p-0.5 opacity-0 shadow-sm transition-opacity group-hover/media:pointer-events-auto group-hover/media:opacity-100 group-focus-within/media:pointer-events-auto group-focus-within/media:opacity-100',
    forceVisible && 'opacity-100'
  ]}
  data-testid={testId}
>
  {@render children()}
</div>
