<!--
@component

Server-local message search. Query text and hydrated results remain transient
in the active server store so browser Back can restore the current search.
-->
<script lang="ts">
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import Panel from '$lib/ui/Panel.svelte';
  import SearchResult from '$lib/components/search/SearchResult.svelte';
  import SearchAvailability from '$lib/components/search/SearchAvailability.svelte';
  import type { MessageSearchResult } from '$lib/api-client/messageSearch';
  import { RoomKind } from '$lib/api-client/roomDirectory';
  import { serverIdToSegment } from '$lib/navigation';
  import { MessageSearchOrder, MessageSearchState } from '$lib/state/server/messageSearch.svelte';
  import { getLocale } from '$lib/i18n/runtime';
  import { useDebouncedMessageSearch } from '$lib/hooks/useDebouncedMessageSearch.svelte';
  import SearchResults from '$lib/components/search/SearchResults.svelte';
  import { buildMessageLinkPath } from '$lib/messageLinks';
  import { formatDateTime, timeFormatSettingsFor } from '$lib/utils/formatTime';
  import { Hint, PageTitle, PaneContent, PaneHeader, ScrollFader, SegmentedControl } from '$lib/ui';
  import { TextInput } from '$lib/ui/form';
  import { m } from '$lib/i18n/messages';

  const serverScope = useServerScope();

  const serverId = $derived(serverScope.serverId);
  const serverStore = $derived(serverScope.store);
  const store = $derived(serverStore.messageSearch);
  const timeFormatSettings = $derived(
    timeFormatSettingsFor(serverStore.currentUser.user?.settings)
  );
  const activeLocale = $derived(getLocale());
  const orderOptions = $derived([
    { value: MessageSearchOrder.RELEVANCE, label: m('search.order.relevance') },
    { value: MessageSearchOrder.NEWEST, label: m('search.order.newest') }
  ]);
  const search = useDebouncedMessageSearch({
    getStore: () => store,
    getInput: (query) => ({ query, order: store.order })
  });
  $effect(() => {
    void store.ensureStatus();
  });

  function submit(event: SubmitEvent): void {
    event.preventDefault();
    search.submitNow();
  }

  function scheduleSearch(event: Event): void {
    search.schedule((event.currentTarget as HTMLInputElement).value);
  }

  function setOrder(nextOrder: MessageSearchOrder): void {
    search.sync();
    store.order = nextOrder;
    if (store.query.trim()) search.submitNow();
  }

  function formatTimestamp(value: string): string {
    return value ? formatDateTime(value, timeFormatSettings, activeLocale) : '';
  }

  function navigateToResult(result: MessageSearchResult): void {
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- buildMessageLinkPath() returns a resolved app route
    void goto(buildMessageLinkPath(serverId, result.roomId, result.id, result.threadRootEventId));
  }
</script>

<PageTitle title={m('search.title')} />

<div class="pane-page">
  <PaneHeader title={m('search.title')} showMobileNav />

  <PaneContent fillHeight>
    <div class="flex min-h-0 flex-1 flex-col gap-6">
      <SearchAvailability
        state={store.status.state}
        checking={store.statusLoading && !store.statusLoaded}
        error={store.statusError}
        onRetry={() => void store.refreshStatus()}
        checkingClass="flex min-h-64 items-center justify-center text-muted"
      >
        {#snippet frame(content)}
          <Panel>{@render content()}</Panel>
        {/snippet}
        <Panel title={m('search.query.label')}>
          <form class="flex flex-wrap items-stretch gap-2" onsubmit={submit}>
            <div class="min-w-64 flex-1">
              <TextInput
                label={m('search.query.label')}
                labelHidden
                bind:value={store.query}
                placeholder={m('search.query.placeholder')}
                leadingIcon="icon-[uil--search]"
                autocomplete="off"
                autofocus
                oninput={scheduleSearch}
              />
            </div>
            <SegmentedControl
              label={m('search.order.label')}
              options={orderOptions}
              value={store.order}
              onchange={setOrder}
            />
          </form>

          {#if store.status.state === MessageSearchState.DEGRADED}
            <div class="mt-4">
              <Hint tone="warning">{m('search.degraded')}</Hint>
            </div>
          {/if}
        </Panel>

        <Panel title={m('search.results')} noPadding fillHeight>
          <ScrollFader top bottom keyboardFocusable={false} class="min-h-0 flex-1">
            <SearchResults {store}>
              {#snippet children(result)}
                <SearchResult
                  {result}
                  data-search-result-id={result.id}
                  viewerLogin={serverStore.currentUser.user?.login}
                  timestampSettings={timeFormatSettings}
                  timestampLocale={activeLocale}
                  onOpen={navigateToResult}
                >
                  {#snippet headerMeta()}
                    <a
                      class="min-w-0 truncate text-xs text-muted hover:text-text hover:underline"
                      href={resolve('/chat/[serverId]/[roomId]', {
                        serverId: serverIdToSegment(serverId),
                        roomId: result.roomId
                      })}
                    >
                      {#if result.roomKind === RoomKind.DM}
                        {m('room.title.direct_message')}
                      {:else}
                        <bdi>#{result.roomName ?? m('search.scope.room')}</bdi>
                      {/if}
                    </a>
                    {#if result.createdAt}
                      <span class="text-xs text-muted" aria-hidden="true">·</span>
                      <!-- eslint-disable svelte/no-navigation-without-resolve -- buildMessageLinkPath() returns a resolved app route -->
                      <a
                        class="min-w-0 truncate text-xs text-muted hover:text-text hover:underline"
                        href={buildMessageLinkPath(
                          serverId,
                          result.roomId,
                          result.id,
                          result.threadRootEventId
                        )}
                      >
                        <time datetime={result.createdAt}>{formatTimestamp(result.createdAt)}</time>
                      </a>
                      <!-- eslint-enable svelte/no-navigation-without-resolve -->
                    {/if}
                  {/snippet}
                </SearchResult>
              {/snippet}
            </SearchResults>
          </ScrollFader>
        </Panel>
      </SearchAvailability>
    </div>
  </PaneContent>
</div>
