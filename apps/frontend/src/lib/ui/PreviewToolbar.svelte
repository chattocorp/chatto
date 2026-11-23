<!-- @component
Preview actions: one pill in the top end corner of a preview frame. Every
preview uses it, for viewing and management actions alike. The pill floats over
the content and reserves no space. It shows on hover or keyboard focus, and
always for touch input. The owner must be a positioned `group/preview` frame.
A text row below the pill must use `preview-actions-clearance`.

The full attachment viewer is not a preview: it uses `layout="row"` to put its
controls in its own control bar.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    children,
    viewing,
    layout = 'corner'
  }: {
    /** Permission-gated management actions, after the viewing actions. */
    children?: Snippet;
    /** Viewing and playback actions available to any reader, before the management actions. */
    viewing?: Snippet;
    /** `corner` is the preview pill; `row` is an inline row for viewer control bars. */
    layout?: 'corner' | 'row';
  } = $props();
</script>

<!-- Actions are direct children so the frame can count them for its clearance. -->
<div
  class={layout === 'corner' ? 'preview-actions' : 'flex max-w-full flex-wrap items-center gap-1'}
>
  {@render viewing?.()}
  {@render children?.()}
</div>
