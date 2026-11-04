<!-- @component
Groups preview actions in the right rim owned by PreviewFrame.
The inline row variant serves file and audio cards and full-screen viewers.
PreviewFrame owns desktop reveal and touch-sheet presentation.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    children,
    layout = 'column'
  }: {
    children: Snippet;
    /** Inline rows remain in the card's normal layout. */
    layout?: 'column' | 'row';
  } = $props();
</script>

<div
  class={layout === 'column' ? 'preview-toolbar' : 'flex max-w-full flex-wrap items-center gap-1'}
>
  {@render children()}
</div>
