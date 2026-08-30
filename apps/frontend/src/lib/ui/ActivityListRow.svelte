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
    dimmed = false,
    important = false,
    rowAttributes = {}
  }: {
    children: Snippet;
    leading?: Snippet;
    actions?: Snippet;
    onclick: () => void;
    disabled?: boolean;
    interactive?: boolean;
    pending?: boolean;
    dimmed?: boolean;
    important?: boolean;
    rowAttributes?: HTMLAttributes<HTMLDivElement>;
  } = $props();
</script>

<div
  {...rowAttributes}
  class={[
    'group flex w-full items-center gap-3 selectable-list-item px-3 py-2.5 transition-colors',
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
      pending && 'cursor-wait',
      dimmed && 'text-muted'
    ]}
    {disabled}
    {onclick}
  >
    {#if leading}{@render leading()}{/if}
    {@render children()}
  </button>
  {#if actions}
    <div class="hover-reveal-action flex shrink-0 items-center">
      {@render actions()}
    </div>
  {/if}
</div>
