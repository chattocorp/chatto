<!--
@component

The standard durable content frame. It provides a title band and an inset work
plane for forms, summaries, record tables, and dense matrices. Inside
PaneContent, it becomes a flat section below 768px. Standalone panels keep
their frames.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    title,
    titleContent,
    subtitle,
    icon,
    count,
    children,
    actions,
    noPadding = false,
    fillHeight = false,
    titleHiddenOnNarrow = false,
    inlineActions = false
  }: {
    title?: string;
    /** Optional rich title content, with title kept as a plain-text fallback. */
    titleContent?: Snippet;
    /** Follows the title and wraps below it when the available width is too small. */
    subtitle?: string | Snippet;
    icon?: string;
    count?: number;
    children: Snippet;
    actions?: Snippet;
    noPadding?: boolean;
    /** Let a dense child, such as a matrix, fill a flex page's remaining height. */
    fillHeight?: boolean;
    /** Visually hide the title below 768px while retaining its accessible heading. */
    titleHiddenOnNarrow?: boolean;
    /**
     * Keep compact actions on the title row in narrow panels; a long title
     * truncates instead. Use it for card grids with a short title and one
     * small action, such as room groups.
     */
    inlineActions?: boolean;
  } = $props();
</script>

<div
  data-panel
  class={[
    'overflow-hidden panel-shell panel-shell-raised',
    fillHeight ? 'flex min-h-0 flex-1 flex-col' : 'shrink-0'
  ]}
>
  {#if title}
    <div
      class={[
        'flex flex-wrap items-center justify-between gap-x-4 gap-y-3 panel-header px-6 py-3',
        titleHiddenOnNarrow && !subtitle && !actions && 'max-md:py-0'
      ]}
    >
      <!-- The 18rem basis moves the actions below the title when the panel, not the viewport, is too narrow. -->
      <div
        class={[
          'flex min-w-0 flex-wrap items-baseline gap-x-3 gap-y-1',
          inlineActions ? 'flex-1' : 'flex-[1_1_18rem]'
        ]}
      >
        <h2
          class={[
            'flex items-center gap-2 text-base font-semibold text-text-top',
            inlineActions && 'min-w-0',
            titleHiddenOnNarrow && 'max-md:sr-only'
          ]}
        >
          {#if icon}
            <span aria-hidden="true" class={icon}></span>
          {/if}
          {#if titleContent}{@render titleContent()}{:else if inlineActions}<span
              class="min-w-0 truncate">{title}</span
            >{:else}{title}{/if}
          {#if count !== undefined}
            <span class="text-muted">({count})</span>
          {/if}
        </h2>
        {#if subtitle}
          <p class="max-w-full min-w-0 text-sm text-muted">
            {#if typeof subtitle === 'string'}
              {subtitle}
            {:else}
              {@render subtitle()}
            {/if}
          </p>
        {/if}
      </div>
      {#if actions}
        <div class={['flex items-center gap-2', inlineActions && 'shrink-0']}>
          {@render actions()}
        </div>
      {/if}
    </div>
  {/if}
  <div
    data-panel-frame
    class={[title || noPadding ? 'px-1 pb-1' : 'p-1', fillHeight && 'flex min-h-0 flex-1 flex-col']}
  >
    <div
      class={[
        'panel-inset',
        noPadding ? 'panel-inset-flush' : 'p-5',
        fillHeight && 'flex min-h-0 flex-1 flex-col'
      ]}
    >
      {@render children()}
    </div>
  </div>
</div>
