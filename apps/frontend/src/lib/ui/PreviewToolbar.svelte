<!-- @component
Groups preview actions below the content on the frame rim.
Viewing actions sit at the start; management actions sit at the end.
Use the inline row for file, audio, and viewer controls. Actions stay visible.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    children,
    viewing,
    layout = 'footer'
  }: {
    /** Permission-gated management actions, aligned to the end. */
    children?: Snippet;
    /** Viewing and playback actions available to any reader, aligned to the start. */
    viewing?: Snippet;
    /** Footer rows sit below the canvas; inline rows share space with card content. */
    layout?: 'footer' | 'row';
  } = $props();
</script>

<div
  class={layout === 'footer'
    ? 'preview-toolbar'
    : 'flex max-w-full flex-wrap items-center gap-x-3 gap-y-1'}
>
  {#if viewing}
    <div class="flex max-w-full flex-wrap items-center gap-1 empty:hidden" data-preview-viewing>
      {@render viewing()}
    </div>
  {/if}
  {#if children}
    <div
      class="ms-auto flex max-w-full flex-wrap items-center gap-1 empty:hidden"
      data-preview-management
    >
      {@render children()}
    </div>
  {/if}
</div>
