<!-- @component
Responsive dialog shell using the command palette's frame and typography.
Narrow screens use a pane below the app header, with its own back header.
Narrow panes leave the app header interactive; wider palettes are modal.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import { paletteDialog } from './paletteDialog';
  import { modalViewport } from './modalViewport';
  import PaletteFrame from './PaletteFrame.svelte';

  let {
    visible = $bindable(false),
    title,
    class: className,
    onclose,
    children: content
  }: {
    visible?: boolean;
    title: string;
    class?: string;
    onclose?: () => void;
    children: Snippet<[close: () => void]>;
  } = $props();

  function close() {
    visible = false;
    onclose?.();
  }
</script>

<dialog
  {@attach (node) => paletteDialog(node, visible, close)}
  {@attach modalViewport}
  aria-label={title}
  class={[
    'palette-dialog border-none bg-transparent p-0 text-inherit backdrop:bg-black/50',
    className
  ]}
>
  {#if visible}
    <PaletteFrame {title} onBack={close}>
      {@render content(close)}
    </PaletteFrame>
  {/if}
</dialog>
