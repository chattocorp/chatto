<script lang="ts">
  import type { Snippet } from 'svelte';
  import PaneHeaderSkeleton from './PaneHeaderSkeleton.svelte';

  let {
    title,
    subtitle,
    loading = false,
    skeletonButtons = 3,
    prefix,
    afterTitle,
    actions,
    // Deprecated: showMobileNav is no longer used since hamburger menu is always visible
    showMobileNav: _showMobileNav = false
  }: {
    title: string;
    subtitle?: string;
    loading?: boolean;
    skeletonButtons?: number;
    prefix?: Snippet;
    afterTitle?: Snippet;
    actions?: Snippet;
    showMobileNav?: boolean;
  } = $props();
</script>

<div class="flex items-center justify-between border-b border-border px-6 py-4">
  <div class="flex min-w-0 flex-1 items-center gap-3">
    {#if prefix}
      {@render prefix()}
    {/if}
    <div class="flex min-w-0 flex-1 flex-col gap-1 md:flex-row md:items-baseline md:gap-3">
      {#if loading}
        <PaneHeaderSkeleton buttons={skeletonButtons} />
      {:else}
        <div class="flex min-w-0 items-baseline gap-3">
          <h1 class="truncate font-black" {title}>{title}</h1>
          {#if afterTitle}
            <div class="shrink-0">
              {@render afterTitle()}
            </div>
          {/if}
        </div>
      {/if}
      {#if subtitle}
        <span class="hidden truncate text-sm text-muted md:inline">{subtitle}</span>
      {/if}
    </div>
  </div>
  {#if actions}
    <div class="flex items-center gap-2">
      {@render actions()}
    </div>
  {/if}
</div>
