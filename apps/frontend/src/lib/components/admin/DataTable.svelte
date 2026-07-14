<script lang="ts" generics="T">
  import type { Snippet } from 'svelte';
  import type { Attachment } from 'svelte/attachments';
  import * as m from '$lib/i18n/messages';

  let {
    items,
    columns,
    header,
    row,
    emptyMessage = m['ui.data_table.empty'](),
    onRowClick,
    getKey,
    getGroupKey,
    group,
    hoverable = true,
    hasMore = false,
    loadingMore = false,
    onLoadMore,
    loadMoreRoot,
    loadMoreRootMargin = '0px 0px 160px 0px',
    loadingMoreMessage = m['ui.data_table.loading_more']()
  }: {
    items: T[];
    columns: number;
    header: Snippet;
    row: Snippet<[T]>;
    emptyMessage?: string;
    onRowClick?: (item: T) => void;
    getKey?: (item: T, index: number) => unknown;
    getGroupKey?: (item: T, index: number) => string | null | undefined;
    group?: Snippet<[T]>;
    /**
     * Whether rows highlight on hover. Defaults to `true` for the standard
     * "list of records" treatment; pass `false` for matrix-style tables
     * where individual cells (not rows) are interactive and a row tint
     * would be visual noise.
     */
    hoverable?: boolean;
    /** Whether another page can be loaded when the table end is reached. */
    hasMore?: boolean;
    /** Whether the next page is currently loading. */
    loadingMore?: boolean;
    /** Called when the trailing sentinel reaches the configured scroll root. */
    onLoadMore?: () => void | Promise<void>;
    /** Scroll container used as the IntersectionObserver root. */
    loadMoreRoot?: HTMLElement;
    /** IntersectionObserver root margin for the trailing sentinel. */
    loadMoreRootMargin?: string;
    /** Compact status text shown in the trailing loading row. */
    loadingMoreMessage?: string;
  } = $props();

  let loadMoreInFlight = false;

  // Default key function: use id if present, otherwise key by item identity.
  function defaultGetKey(item: T): unknown {
    if (item && typeof item === 'object' && 'id' in item) {
      return (item as { id: string | number }).id;
    }
    return item;
  }

  const keyFn = $derived(getKey ?? defaultGetKey);

  function shouldRenderGroup(item: T, index: number): boolean {
    if (!group || !getGroupKey) return false;

    const current = getGroupKey(item, index);
    if (!current) return false;
    if (index === 0) return true;

    return current !== getGroupKey(items[index - 1], index - 1);
  }

  function triggerLoadMore(callback = onLoadMore) {
    if (!hasMore || loadingMore || loadMoreInFlight || !callback) return;

    loadMoreInFlight = true;
    try {
      void Promise.resolve(callback()).finally(() => {
        loadMoreInFlight = false;
      });
    } catch (e) {
      loadMoreInFlight = false;
      throw e;
    }
  }

  const loadMoreSentinel: Attachment<HTMLTableRowElement> = (element) => {
    const root = loadMoreRoot;
    if (!root || !onLoadMore || !hasMore || loadingMore) return;

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) {
          triggerLoadMore(onLoadMore);
        }
      },
      {
        root,
        rootMargin: loadMoreRootMargin
      }
    );

    observer.observe(element);

    return () => observer.disconnect();
  };
</script>

<table class="w-full [&_thead_th]:whitespace-nowrap">
  <thead>
    <tr class="panel-header text-left text-sm text-muted">
      {@render header()}
    </tr>
  </thead>
  <tbody>
    {#each items as item, index (keyFn(item, index))}
      {#if shouldRenderGroup(item, index)}
        <tr class="border-b border-border bg-surface/80">
          <td colspan={columns} class="px-4 py-2">
            {@render group?.(item)}
          </td>
        </tr>
      {/if}
      <tr
        class={[
          'border-b border-border last:border-0',
          hoverable ? 'hover:bg-surface-emphasized/40' : '',
          onRowClick ? 'cursor-pointer' : ''
        ]}
        onclick={() => onRowClick?.(item)}
      >
        {@render row(item)}
      </tr>
    {:else}
      <tr>
        <td colspan={columns} class="px-4 py-8 text-center text-muted">{emptyMessage}</td>
      </tr>
    {/each}

    {#if hasMore || loadingMore}
      <tr
        class="border-b border-border last:border-0"
        aria-hidden={!loadingMore}
        {@attach hasMore ? loadMoreSentinel : undefined}
      >
        <td
          colspan={columns}
          class={loadingMore ? 'px-4 py-3 text-center text-sm text-muted' : 'h-px p-0'}
        >
          {#if loadingMore}
            <span class="inline-flex items-center gap-2" aria-live="polite">
              <span class="iconify animate-spin text-base uil--spinner" aria-hidden="true"></span>
              {loadingMoreMessage}
            </span>
          {/if}
        </td>
      </tr>
    {/if}
  </tbody>
</table>
