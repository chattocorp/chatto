<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    title,
    subtitle,
    icon,
    count,
    children,
    actions,
    noPadding = false
  }: {
    title?: string;
    subtitle?: string;
    icon?: string;
    count?: number;
    children: Snippet;
    actions?: Snippet;
    noPadding?: boolean;
  } = $props();
</script>

<div class={['panel-shell panel-shell-raised', noPadding && 'overflow-hidden']}>
  {#if title}
    <div class="flex items-center justify-between gap-4 rounded-t-lg panel-header px-5 py-4">
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
          <p class="text-sm text-muted">{subtitle}</p>
        {/if}
      </div>
      {#if actions}
        <div class="flex items-center gap-2">
          {@render actions()}
        </div>
      {/if}
    </div>
  {/if}
  <div class={noPadding ? '' : 'p-5'}>
    {@render children()}
  </div>
</div>
