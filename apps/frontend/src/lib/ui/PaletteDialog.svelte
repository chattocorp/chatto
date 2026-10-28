<!-- @component
Responsive modal shell using the command palette's frame and typography.
Narrow screens use a pane below the app header, with its own back header.
ModalSurface owns dismissal and focus restoration; content owns its layout.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import ModalSurface from './ModalSurface.svelte';
  import PaletteFrame from './PaletteFrame.svelte';

  let {
    visible = $bindable(false),
    title,
    onclose,
    children: content
  }: {
    visible?: boolean;
    title: string;
    onclose?: () => void;
    children: Snippet<[close: () => void]>;
  } = $props();
</script>

<ModalSurface
  bind:visible
  ariaLabel={title}
  {onclose}
  autofocusContent
  class="palette-dialog text-inherit"
>
  {#snippet children(close)}
    <PaletteFrame {title} onBack={close}>
      {@render content(close)}
    </PaletteFrame>
  {/snippet}
</ModalSurface>
