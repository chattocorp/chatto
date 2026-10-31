<!-- @component
Centers a single page surface within the available pane. The optional header
stays above the scroll viewport. Tall content starts at the top and scrolls
normally, so its first and last controls remain reachable on small screens.
Text alignment belongs to the caller. Panels use the normal mobile page style.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import ScrollFader from './ScrollFader.svelte';

  let {
    children,
    header
  }: {
    children: Snippet;
    /** Fixed page navigation, usually a PaneHeader. */
    header?: Snippet;
  } = $props();
</script>

<div class="pane-page">
  {#if header}
    <div class="shrink-0" data-page-reveal>{@render header()}</div>
  {/if}
  <ScrollFader top bottom>
    <div class="pane-content flex flex-1 flex-col p-4 md:p-6">
      <!-- Auto margins absorb spare space but become zero when content is tall. -->
      <div class="m-auto w-full max-w-pane shrink-0" data-page-reveal>
        {@render children()}
      </div>
    </div>
  </ScrollFader>
</div>
