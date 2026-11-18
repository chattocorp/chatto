<!--
@component

Two columns of settings panels for a page with several short form panels.
Each column stacks its panels with the standard page gap. The columns sit side
by side when the content column is at least 48rem wide, and stack in one
column, `start` first, when it is narrower.

Give each panel `fillHeight` when a column holds one panel and both panels
must end on the same line. Keep a wide panel, such as a table or a matrix,
outside this component, so that it spans the content column.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    start,
    end
  }: {
    /** Panels of the first column. They come first in the one-column layout. */
    start: Snippet;
    /** Panels of the second column. */
    end: Snippet;
  } = $props();
</script>

<div class="@container">
  <div class="grid gap-6 @min-[48rem]:grid-cols-2">
    <div class="flex min-w-0 flex-col gap-6">{@render start()}</div>
    <div class="flex min-w-0 flex-col gap-6">{@render end()}</div>
  </div>
</div>
