<!-- @component
Shared palette content frame. Narrow screens use a full-height pane with a
back header; wider screens retain the compact floating menu. The owner keeps
the native dialog and its dismissal lifecycle.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import PaneHeader from './PaneHeader.svelte';

  let {
    title,
    onBack,
    children
  }: {
    title: string;
    onBack: () => void;
    children: Snippet;
  } = $props();
</script>

<div class="command-palette">
  <div class="shrink-0 md:hidden">
    <PaneHeader {title} {onBack} />
  </div>
  {@render children()}
</div>
