<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    title,
    maxWidth,
    bordered = false,
    actions,
    children
  }: {
    title: string;
    maxWidth?: 'max-w-md' | 'max-w-lg' | 'max-w-xl' | 'max-w-2xl';
    bordered?: boolean;
    actions?: Snippet;
    children: Snippet;
  } = $props();
</script>

<section class={[maxWidth, bordered && 'border-t border-border pt-6']}>
  <div class="mb-4 flex items-center justify-between gap-3">
    <h3 class="text-sm font-semibold text-muted uppercase">{title}</h3>
    {#if actions}
      <div class="flex shrink-0 items-center gap-2">
        {@render actions()}
      </div>
    {/if}
  </div>
  {@render children()}
</section>
