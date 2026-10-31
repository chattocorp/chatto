<!-- @component
Groups preview actions in a top visor, or an inline row for file and audio cards.
Overlay mode requires a positioned parent with group/preview.
Hover and focus reveal the visor; touch devices keep it visible.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    children,
    layout = 'overlay'
  }: {
    children: Snippet;
    /** Inline rows remain in the card's normal layout. */
    layout?: 'overlay' | 'row';
  } = $props();
</script>

<div
  class={layout === 'overlay' ? 'preview-toolbar' : 'flex max-w-full flex-wrap items-center gap-1'}
>
  {@render children()}
</div>
