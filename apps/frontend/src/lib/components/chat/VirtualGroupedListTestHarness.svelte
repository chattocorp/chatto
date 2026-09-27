<!--
@component

Mounts `VirtualGroupedList` in a fixed-height scroll container for component
tests. Rows have a fixed inline height because tests run without the app
stylesheet.
-->
<script lang="ts">
  import VirtualGroupedList from './VirtualGroupedList.svelte';
  import type { VirtualListGroup } from './groupedListItems';

  type Row = { id: string };

  let { groups }: { groups: VirtualListGroup<Row>[] } = $props();
  let scrollEl = $state<HTMLDivElement>();
</script>

{#snippet row(entry: Row)}
  <div style="height: 48px" data-testid="virtual-row">{entry.id}</div>
{/snippet}

<div
  bind:this={scrollEl}
  style="height: 300px; overflow-y: auto; display: flex; flex-direction: column"
  data-testid="virtual-viewport"
>
  <VirtualGroupedList {groups} item={row} scrollRef={scrollEl} itemSize={50} />
</div>
