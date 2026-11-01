<!-- @component Exercises diagram loading and remounting in a virtualized thread. -->
<script lang="ts">
  import { Virtualizer } from 'virtua/svelte';
  import MarkdownHtml from './MarkdownHtml.svelte';

  let { html }: { html: string[] } = $props();
  let scroller = $state<HTMLDivElement>();
</script>

<div
  bind:this={scroller}
  class="prose"
  style="height: 400px; width: 360px; overflow: auto"
  data-testid="diagram-timeline"
>
  <Virtualizer data={html} scrollRef={scroller} itemSize={60}>
    {#snippet children(value: string, index: number)}
      <div data-diagram-row={index}><MarkdownHtml html={value} /></div>
    {/snippet}
  </Virtualizer>
</div>
