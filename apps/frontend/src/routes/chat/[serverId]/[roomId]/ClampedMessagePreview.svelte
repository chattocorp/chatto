<!--
@component

Caps a room-search result without creating a nested scroll region. A bottom
fade is rendered only when the message presentation actually overflows.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { Attachment } from 'svelte/attachments';

  let { children }: { children: Snippet } = $props();

  let clipped = $state(false);

  function measureOverflow(node: HTMLElement): ReturnType<Attachment> {
    const update = () => {
      clipped = node.scrollHeight > node.clientHeight + 1;
    };

    update();
    queueMicrotask(update);
    const resizeObserver = new ResizeObserver(update);
    const mutationObserver = new MutationObserver(update);
    resizeObserver.observe(node);
    mutationObserver.observe(node, { childList: true, subtree: true, characterData: true });

    return () => {
      resizeObserver.disconnect();
      mutationObserver.disconnect();
    };
  }
</script>

<div class="relative max-h-40 overflow-hidden" {@attach measureOverflow}>
  {@render children()}
  {#if clipped}
    <div
      class="pointer-events-none absolute inset-x-0 bottom-0 h-12 bg-gradient-to-t from-background via-background/90 to-transparent group-hover/search-result:from-surface group-hover/search-result:via-surface/90 group-focus-visible/search-result:from-surface group-focus-visible/search-result:via-surface/90"
      aria-hidden="true"
      data-room-search-result-fade
    ></div>
  {/if}
</div>
