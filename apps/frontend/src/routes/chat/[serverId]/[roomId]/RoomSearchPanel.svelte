<!--
@component

Room-scoped message search for the room sidebar. Its store is retained per room
so switching rooms cannot leak a query or plaintext results into another room.
-->
<script lang="ts">
  import type { Attachment } from 'svelte/attachments';
  import SearchResult from '$lib/components/search/SearchResult.svelte';
  import SearchAvailability from '$lib/components/search/SearchAvailability.svelte';
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import {
    MessageSearchOrder,
    MessageSearchState,
    type MessageSearchStore
  } from '$lib/state/server/messageSearch.svelte';
  import { useDebouncedMessageSearch } from '$lib/hooks/useDebouncedMessageSearch.svelte';
  import { useLoadMoreWhenVisible } from '$lib/hooks/useLoadMoreWhenVisible.svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { EmptyState, Hint, ScrollFader } from '$lib/ui';
  import { TextInput } from '$lib/ui/form';
  import { formatDateTime, timeFormatSettingsFor } from '$lib/utils/formatTime';
  import ClampedMessagePreview from './ClampedMessagePreview.svelte';

  let {
    roomId,
    store,
    onOpenResult
  }: {
    roomId: string;
    store: MessageSearchStore;
    onOpenResult?: (messageEventId: string, threadRootEventId: string | null) => void;
  } = $props();

  const serverScope = useServerScope();
  const userSettings = $derived(
    timeFormatSettingsFor(serverScope.store.currentUser.user?.settings)
  );
  const activeLocale = $derived(getLocale());
  const search = useDebouncedMessageSearch({
    getStore: () => store,
    getInput: (query) => ({ query, roomId, order: MessageSearchOrder.RELEVANCE })
  });
  const loadMoreWhenVisible = useLoadMoreWhenVisible({
    getCursor: () => store.nextCursor,
    loadMore: () => store.loadMore(),
    hasError: () => store.error
  });

  $effect(() => {
    void store.ensureStatus();
  });

  function submit(event: SubmitEvent): void {
    event.preventDefault();
    search.submitNow();
  }

  const focusSearchField: Attachment<HTMLFormElement> = (form) => {
    queueMicrotask(() => {
      if (!form.isConnected) return;
      form.querySelector<HTMLInputElement>('input')?.focus();
    });
  };

  function scheduleSearch(event: Event): void {
    search.schedule((event.currentTarget as HTMLInputElement).value);
  }

  function formatTimestamp(value: string): string {
    return value ? formatDateTime(value, userSettings, activeLocale) : '';
  }
</script>

<SearchAvailability
  state={store.status.state}
  checking={store.statusLoading && !store.statusLoaded}
  error={store.statusError}
  onRetry={() => void store.refreshStatus()}
  checkingClass="flex min-h-32 flex-1 items-center justify-center p-4 text-center text-sm text-muted"
>
  {#snippet frame(content, checking)}
    {#if checking}
      {@render content()}
    {:else}
      <div class="flex min-h-0 flex-1 flex-col justify-center p-4">{@render content()}</div>
    {/if}
  {/snippet}
  <div class="flex min-h-0 flex-1 flex-col">
    <div class="border-b border-border p-2">
      <form onsubmit={submit} {@attach focusSearchField}>
        <TextInput
          label={m('search.query.label')}
          labelHidden
          testid="room-search-query"
          bind:value={store.query}
          placeholder={m('search.query.placeholder')}
          leadingIcon="icon-[uil--search]"
          autocomplete="off"
          oninput={scheduleSearch}
        />
      </form>
      {#if store.status.state === MessageSearchState.DEGRADED}
        <div class="mt-2">
          <Hint tone="warning">{m('search.degraded')}</Hint>
        </div>
      {/if}
    </div>

    <ScrollFader top bottom keyboardFocusable={false} class="min-h-0 flex-1">
      <div class="flex min-h-full flex-col" aria-live="polite">
        {#if store.error}
          <EmptyState icon="icon-[uil--exclamation-triangle]" title={m('search.error.title')}>
            {m('search.error.description')}
          </EmptyState>
        {:else if store.loading && store.results.length === 0}
          <div class="flex min-h-32 flex-1 items-center justify-center p-4 text-sm text-muted">
            <span class="iconify me-2 icon-[uil--spinner-alt] animate-spin" aria-hidden="true"
            ></span>
            {m('search.searching')}
          </div>
        {:else if store.hasSearched && store.results.length === 0 && !store.nextCursor}
          <EmptyState icon="icon-[uil--search-minus]" title={m('search.no_results.title')}>
            {m('search.no_results.description')}
          </EmptyState>
        {:else if !store.hasSearched}
          <EmptyState icon="icon-[uil--search]" title={m('search.prompt.title')} />
        {:else}
          <ol class="selectable-list gap-3 py-2">
            {#each store.results as result (result.id)}
              <li>
                <SearchResult
                  {result}
                  aria-label={`${result.actor?.displayName || result.actor?.login || m('common.unknown')}: ${result.body}`}
                  data-room-search-result-id={result.id}
                  class="group/search-result"
                  viewerLogin={serverScope.store.currentUser.user?.login}
                  timestampSettings={userSettings}
                  timestampLocale={activeLocale}
                  attachmentClass="text-xs"
                  onOpen={(result) => onOpenResult?.(result.id, result.threadRootEventId)}
                >
                  {#snippet preview(content)}
                    <div class="pointer-events-none" inert data-room-search-result-preview>
                      <ClampedMessagePreview>{@render content()}</ClampedMessagePreview>
                    </div>
                  {/snippet}
                  {#snippet headerMeta()}
                    {#if result.createdAt}
                      <time class="text-xs text-muted" datetime={result.createdAt}>
                        {formatTimestamp(result.createdAt)}
                      </time>
                    {/if}
                  {/snippet}
                </SearchResult>
              </li>
            {/each}
          </ol>
          {#if store.nextCursor}
            <div
              {@attach loadMoreWhenVisible}
              class="flex h-12 items-center justify-center text-sm text-muted"
            >
              {#if store.loadingMore}
                <span class="iconify me-2 icon-[uil--spinner-alt] animate-spin" aria-hidden="true"
                ></span>
                {m('search.loading_more')}
              {/if}
            </div>
          {/if}
        {/if}
      </div>
    </ScrollFader>
  </div>
</SearchAvailability>
