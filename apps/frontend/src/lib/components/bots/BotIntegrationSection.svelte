<script lang="ts" generics="Item extends { id: string }">
  import type { Snippet } from 'svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import LoadingFog from '$lib/ui/LoadingFog.svelte';

  let {
    title,
    description,
    testId,
    items,
    empty,
    loading = false,
    actions,
    details,
    itemActions,
    footer
  }: {
    title: string;
    description: string;
    testId: string;
    items: Item[];
    empty: string;
    /** Show a pending content block before the first item is available. */
    loading?: boolean;
    actions: Snippet;
    details: Snippet<[Item]>;
    itemActions: Snippet<[Item]>;
    footer?: Snippet;
  } = $props();
</script>

<!-- @component Shared collection layout for bot API keys and incoming/outbound webhooks. Callers own mutations and dialogs. -->
<Panel {title} subtitle={description} noPadding {actions}>
  {#if items.length > 0}
    <div class="selectable-list" data-testid={testId}>
      {#each items as item (item.id)}
        <div class="flex flex-col gap-4 selectable-list-item px-5 py-4 sm:flex-row sm:flex-wrap sm:items-center">
          <div class="min-w-0 flex-1 sm:basis-80">{@render details(item)}</div>
          <div class="flex shrink-0 flex-wrap justify-end gap-2">{@render itemActions(item)}</div>
        </div>
      {/each}
    </div>
  {:else if loading}
    <LoadingFog class="m-5 h-32" />
  {:else}
    <div class="p-5 text-muted" data-testid={testId}>{empty}</div>
  {/if}
  {@render footer?.()}
</Panel>
