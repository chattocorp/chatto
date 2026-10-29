<!-- @component Shared interaction and visual shell for a newest-first activity-list row. -->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLAttributes } from 'svelte/elements';

  let {
    children,
    leading,
    actions,
    onclick,
    disabled = false,
    interactive = true,
    pending = false,
    important = false,
    compact = false,
    overlayActions = false,
    rowAttributes = {}
  }: {
    children: Snippet;
    leading?: Snippet;
    actions?: Snippet;
    onclick: () => void;
    disabled?: boolean;
    interactive?: boolean;
    pending?: boolean;
    important?: boolean;
    /** Reduce vertical padding for compact activity feeds. */
    compact?: boolean;
    /** On hover-capable devices, reveal actions over the trailing content without reserving a column. */
    overlayActions?: boolean;
    rowAttributes?: HTMLAttributes<HTMLDivElement>;
  } = $props();
</script>

<div
  {...rowAttributes}
  class={[
    'group relative flex w-full items-center gap-3 selectable-list-item px-3 transition-colors',
    compact ? 'min-h-11 py-0.5' : 'py-2.5',
    interactive ? 'cursor-pointer' : 'cursor-default',
    important && 'bg-attention/5',
    rowAttributes.class
  ]}
>
  <button
    type="button"
    class={[
      'flex min-w-0 flex-1 items-center gap-3 rounded-md text-start focus-visible:outline-2 focus-visible:outline-action',
      interactive ? 'cursor-pointer' : 'cursor-default',
      pending && 'cursor-wait'
    ]}
    {disabled}
    {onclick}
  >
    {#if leading}{@render leading()}{/if}
    {@render children()}
  </button>
  {#if actions}
    <div
      class={[
        'hover-reveal-action flex shrink-0 items-center',
        overlayActions &&
          '[@media(hover:hover)_and_(pointer:fine)]:absolute [@media(hover:hover)_and_(pointer:fine)]:end-3 [@media(hover:hover)_and_(pointer:fine)]:top-1/2 [@media(hover:hover)_and_(pointer:fine)]:-translate-y-1/2'
      ]}
    >
      {@render actions()}
    </div>
  {/if}
</div>
