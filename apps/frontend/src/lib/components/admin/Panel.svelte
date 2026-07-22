<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    title,
    subtitle,
    icon,
    count,
    children,
    actions,
    noPadding = false,
    fillHeight = false
  }: {
    title?: string;
    subtitle?: string | Snippet;
    icon?: string;
    count?: number;
    children: Snippet;
    actions?: Snippet;
    noPadding?: boolean;
    /** Let a dense child, such as a matrix, fill a flex page's remaining height. */
    fillHeight?: boolean;
  } = $props();
</script>

<div
  class={[
    'overflow-hidden panel-shell panel-shell-raised',
    fillHeight ? 'flex min-h-0 flex-1 flex-col' : 'shrink-0'
  ]}
>
  {#if title}
    <div class="flex items-center justify-between gap-4 panel-header px-6 py-3">
      <div class="min-w-0">
        <h2 class="flex items-center gap-2 text-base font-semibold text-text-top">
          {#if icon}
            <span class={icon}></span>
          {/if}
          {title}
          {#if count !== undefined}
            <span class="text-muted">({count})</span>
          {/if}
        </h2>
        {#if subtitle}
          <p class="text-sm text-muted">
            {#if typeof subtitle === 'string'}
              {subtitle}
            {:else}
              {@render subtitle()}
            {/if}
          </p>
        {/if}
      </div>
      {#if actions}
        <div class="flex items-center gap-2">
          {@render actions()}
        </div>
      {/if}
    </div>
  {/if}
  <div class={[title || noPadding ? 'px-1 pb-1' : 'p-1', fillHeight && 'flex min-h-0 flex-1 flex-col']}>
    <div
      class={[
        'panel-inset',
        noPadding ? 'overflow-hidden' : 'p-5',
        fillHeight && 'flex min-h-0 flex-1 flex-col'
      ]}
    >
      {@render children()}
    </div>
  </div>
</div>
