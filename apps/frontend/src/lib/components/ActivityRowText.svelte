<!--
@component

Single-line text body shared by notification and thread activity rows: a title,
an optional responsive location column, and a timestamp. Put @container/activity
on the owning row. Cross-server locations can remain below the title on narrow rows.
`emphasized` renders the title in bold for unread activity; other rows keep
regular weight in muted text. Supporting metadata uses the subdued text utility.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    title,
    location,
    locationOnNarrow = false,
    hideTimeOnNarrow = false,
    time,
    emphasized = false,
    overlayActions = 0,
    testId
  }: {
    title: Snippet;
    /** Optional location column; requires @container/activity on the row and hides below 36rem. */
    location?: Snippet;
    /** Keep cross-server context visible below the title when the location column hides. */
    locationOnNarrow?: boolean;
    /** Below 36rem, keep the time available only to screen readers. */
    hideTimeOnNarrow?: boolean;
    /** Formatted time shown at the end of the title line. */
    time: string;
    emphasized?: boolean;
    /** Number of overlaid actions; hides metadata on hover/focus and reserves space for two. */
    overlayActions?: 0 | 1 | 2;
    testId?: string;
  } = $props();
</script>

<span
  class={[
    'grid min-w-0 flex-1 items-baseline gap-x-4 text-muted',
    hideTimeOnNarrow
      ? 'grid-cols-1 @min-[36rem]/activity:grid-cols-[minmax(0,1fr)_auto]'
      : 'grid-cols-[minmax(0,1fr)_auto]',
    location &&
      '@min-[36rem]/activity:grid-cols-[minmax(0,1fr)_fit-content(40%)_minmax(5rem,max-content)]'
  ]}
  data-testid={testId}
>
  <bdi
    class={[
      'col-start-1 row-start-1 min-w-0 truncate',
      emphasized ? 'font-semibold text-text-top' : 'text-muted'
    ]}
    dir="auto"
  >
    {@render title()}
  </bdi>
  {#if location}
    <span
      class={[
        'min-w-0 items-baseline gap-1 text-sm text-subdued @min-[36rem]/activity:col-start-2 @min-[36rem]/activity:row-start-1 @min-[36rem]/activity:flex @min-[36rem]/activity:flex-nowrap [&>bdi]:min-w-0 [&>bdi]:truncate',
        locationOnNarrow ? 'col-start-1 row-start-2 flex flex-wrap' : 'hidden'
      ]}
    >
      {@render location()}
    </span>
  {/if}
  <span
    class={[
      'col-start-2 row-start-1 text-end text-sm whitespace-nowrap text-subdued tabular-nums',
      hideTimeOnNarrow && 'sr-only @min-[36rem]/activity:not-sr-only',
      location && '@min-[36rem]/activity:col-start-3',
      overlayActions === 2 && '[@media(hover:hover)_and_(pointer:fine)]:min-w-20',
      overlayActions &&
        '[@media(hover:hover)_and_(pointer:fine)]:group-focus-within:opacity-0 [@media(hover:hover)_and_(pointer:fine)]:group-hover:opacity-0'
    ]}
  >
    {time}
  </span>
</span>
