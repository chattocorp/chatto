<!-- @component
Renders search states and loads more results at the scroll edge. The owner
supplies each result and keeps navigation and the scroll container. Compact
mode preserves the sidebar's loading indicator, spacing, and shorter prompt.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { MessageSearchResult } from '$lib/api-client/messageSearch';
  import type { MessageSearchStore } from '$lib/state/server/messageSearch.svelte';
  import { useLoadMoreWhenVisible } from '$lib/hooks/useLoadMoreWhenVisible.svelte';
  import { EmptyState } from '$lib/ui';
  import { m } from '$lib/i18n/messages';

  let {
    store,
    compact = false,
    children
  }: {
    store: Pick<
      MessageSearchStore,
      'error' | 'loading' | 'loadingMore' | 'hasSearched' | 'results' | 'nextCursor' | 'loadMore'
    >;
    compact?: boolean;
    children: Snippet<[MessageSearchResult]>;
  } = $props();

  const loadMoreWhenVisible = useLoadMoreWhenVisible({
    getCursor: () => store.nextCursor,
    loadMore: () => store.loadMore(),
    hasError: () => store.error
  });
</script>

<div class="flex min-h-full flex-col" aria-live="polite">
  {#if store.error}
    <EmptyState icon="icon-[uil--exclamation-triangle]" title={m('search.error.title')}>
      {m('search.error.description')}
    </EmptyState>
  {:else if compact && store.loading && store.results.length === 0}
    <div class="flex min-h-32 flex-1 items-center justify-center p-4 text-sm text-muted">
      <span class="iconify me-2 icon-[uil--spinner-alt] animate-spin" aria-hidden="true"></span>
      {m('search.searching')}
    </div>
  {:else if store.hasSearched && !store.loading && store.results.length === 0 && !store.nextCursor}
    <EmptyState icon="icon-[uil--search-minus]" title={m('search.no_results.title')}>
      {m('search.no_results.description')}
    </EmptyState>
  {:else if !store.hasSearched}
    <EmptyState icon="icon-[uil--search]" title={m('search.prompt.title')}>
      {#if !compact}{m('search.prompt.description')}{/if}
    </EmptyState>
  {:else}
    <ol class={['selectable-list', compact ? 'gap-3 py-2' : 'gap-4']}>
      {#each store.results as result (result.id)}
        <li>{@render children(result)}</li>
      {/each}
    </ol>
    {#if store.nextCursor}
      <div
        {@attach loadMoreWhenVisible}
        class={['flex h-12 items-center justify-center text-muted', compact && 'text-sm']}
      >
        {#if store.loadingMore}
          <span class="iconify me-2 icon-[uil--spinner-alt] animate-spin" aria-hidden="true"></span>
          {m('search.loading_more')}
        {/if}
      </div>
    {/if}
  {/if}
</div>
