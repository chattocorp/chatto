<!-- @component
Compact modal shell using the command palette's frame, width, and typography.
ModalSurface owns dismissal and focus restoration; content owns its layout.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import ModalSurface from './ModalSurface.svelte';

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
  class="mt-[15vh] max-h-none max-w-none overflow-visible text-inherit"
>
  {#snippet children(close)}
    <div class="command-palette">
      {@render content(close)}
    </div>
  {/snippet}
</ModalSurface>
