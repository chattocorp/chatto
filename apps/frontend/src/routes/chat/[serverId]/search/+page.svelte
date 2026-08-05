<!--
@component

Server-local message search. Query text and hydrated results remain transient
in the active server store so browser Back can restore the current search.
-->
<script lang="ts">
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { Panel } from '$lib/components/admin';
  import MessageView from '$lib/components/messages/MessageView.svelte';
  import type { UserAvatarUserView } from '$lib/render/users';
  import type { MessageSearchResult } from '$lib/api-client/messageSearch';
  import { RoomKind } from '$lib/api-client/roomDirectory';
  import { serverIdToSegment } from '$lib/navigation';
  import { MessageSearchOrder, MessageSearchState } from '$lib/state/server/messageSearch.svelte';
  import { getLocale } from '$lib/i18n/runtime';
  import { useDebouncedMessageSearch } from '$lib/hooks/useDebouncedMessageSearch.svelte';
  import { useLoadMoreWhenVisible } from '$lib/hooks/useLoadMoreWhenVisible.svelte';
  import { buildMessageLinkPath } from '$lib/messageLinks';
  import { formatDateTime, timeFormatSettingsFor } from '$lib/utils/formatTime';
  import {
    EmptyState,
    Hint,
    PageTitle,
    PaneContent,
    PaneHeader,
    ScrollFader,
    SegmentedControl
  } from '$lib/ui';
  import { Button, TextInput } from '$lib/ui/form';
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

  function scheduleSearch(event: Event): void {
    search.schedule((event.currentTarget as HTMLInputElement).value);
  }

  function setOrder(nextOrder: MessageSearchOrder): void {
    search.sync();
    store.order = nextOrder;
    if (store.query.trim()) search.submitNow();
  }

  function resultActor(result: MessageSearchResult): UserAvatarUserView | null {
    if (!result.actor) return null;
    return {
      ...result.actor,
      presenceStatus: PresenceStatus.OFFLINE
    };
  }

  function formatTimestamp(value: string): string {
    return value ? formatDateTime(value, timeFormatSettings, activeLocale) : '';
  }

  function navigateToResult(result: MessageSearchResult): void {
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- buildMessageLinkPath() returns a resolved app route
    void goto(buildMessageLinkPath(serverId, result.roomId, result.id, result.threadRootEventId));
  }

  function openResult(event: MouseEvent, result: MessageSearchResult): void {
    // A search result is one navigation target. Links rendered inside the
    // shared message view are presentation here and must not override it.
    event.preventDefault();
    navigateToResult(result);
  }

  function openResultFromKeyboard(event: KeyboardEvent, result: MessageSearchResult): void {
    if (event.target !== event.currentTarget || event.key !== 'Enter') return;
    event.preventDefault();
    navigateToResult(result);
  }
</script>

<PageTitle title={m('search.title')} />

<div class="pane-page">
  <PaneHeader title={m('search.title')} showMobileNav />

  <PaneContent fillHeight>
    <div class="flex min-h-0 flex-1 flex-col gap-6">
      {#if store.statusLoading && !store.statusLoaded}
        <Panel>
          <div class="flex min-h-64 items-center justify-center text-muted" aria-live="polite">
            <span class="iconify mr-2 icon-[uil--spinner-alt] animate-spin" aria-hidden="true"
            ></span>
            {m('search.checking')}
          </div>
        </Panel>
      {:else if store.statusError || store.status.state === MessageSearchState.UNAVAILABLE}
        <Panel>
          <EmptyState icon="icon-[uil--cloud-slash]" title={m('search.unavailable.title')}>
            <p>{m('search.unavailable.description')}</p>
            <div class="mt-4">
              <Button variant="secondary" onclick={() => void store.refreshStatus()}>
                {m('common.retry')}
              </Button>
            </div>
          </EmptyState>
        </Panel>
      {:else if store.status.state === MessageSearchState.DISABLED}
        <Panel>
          <EmptyState icon="icon-[uil--search-alt]" title={m('search.disabled.title')}>
            {m('search.disabled.description')}
          </EmptyState>
        </Panel>
      {:else if store.status.state === MessageSearchState.STARTING || store.status.state === MessageSearchState.INDEXING}
        <Panel>
          <EmptyState icon="icon-[uil--database]" title={m('search.indexing.title')}>
            <p>{m('search.indexing.description')}</p>
            <div class="mt-4">
              <Button variant="secondary" onclick={() => void store.refreshStatus()}>
                {m('search.check_again')}
              </Button>
            </div>
          </EmptyState>
        </Panel>
      {:else}
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
            <div class="flex min-h-full flex-col" aria-live="polite">
              {#if store.error}
                <EmptyState icon="icon-[uil--exclamation-triangle]" title={m('search.error.title')}>
                  {m('search.error.description')}
                </EmptyState>
              {:else if store.hasSearched && !store.loading && store.results.length === 0 && !store.nextCursor}
                <EmptyState icon="icon-[uil--search-minus]" title={m('search.no_results.title')}>
                  {m('search.no_results.description')}
                </EmptyState>
              {:else if !store.hasSearched}
                <EmptyState icon="icon-[uil--search]" title={m('search.prompt.title')}>
                  {m('search.prompt.description')}
                </EmptyState>
              {:else}
                <ol class="selectable-list gap-4">
                  {#each store.results as result (result.id)}
                    <li>
                      <div
                        role="link"
                        tabindex="0"
                        data-search-result-id={result.id}
                        class="cursor-pointer selectable-list-item"
                        onclick={(event) => openResult(event, result)}
                        onkeydown={(event) => openResultFromKeyboard(event, result)}
                      >
                        <MessageView
                          eventId={result.id}
                          actor={resultActor(result)}
                          displayName={result.actor?.displayName ||
                            result.actor?.login ||
                            m('common.unknown')}
                          missingActorIsDeleted={false}
                          body={result.body}
                          viewerLogin={serverStore.currentUser.user?.login}
                          timestampSettings={timeFormatSettings}
                          timestampLocale={activeLocale}
                          rowClass="hover:bg-transparent md:mx-0 md:pr-2"
                        >
                          {#snippet headerMeta()}
                            <a
                              class="min-w-0 truncate text-xs text-muted hover:text-text hover:underline"
                              href={resolve('/chat/[serverId]/[roomId]', {
                                serverId: serverIdToSegment(serverId),
                                roomId: result.roomId
                              })}
                            >
                              {result.roomKind === RoomKind.DM
                                ? m('room.title.direct_message')
                                : `#${result.roomName ?? m('search.scope.room')}`}
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
                                <time datetime={result.createdAt}
                                  >{formatTimestamp(result.createdAt)}</time
                                >
                              </a>
                              <!-- eslint-enable svelte/no-navigation-without-resolve -->
                            {/if}
                          {/snippet}

                          {#snippet afterBody()}
                            {#if result.attachmentCount > 0}
                              <p class="inline-flex items-center gap-1 text-sm text-muted">
                                <span class="iconify icon-[uil--paperclip]" aria-hidden="true"
                                ></span>
                                {m('search.attachments', { count: result.attachmentCount })}
                              </p>
                            {/if}
                          {/snippet}
                        </MessageView>
                      </div>
                    </li>
                  {/each}
                </ol>
                {#if store.nextCursor}
                  <div
                    {@attach loadMoreWhenVisible}
                    class="flex h-12 items-center justify-center text-muted"
                  >
                    {#if store.loadingMore}
                      <span
                        class="iconify mr-2 icon-[uil--spinner-alt] animate-spin"
                        aria-hidden="true"
                      ></span>
                      {m('search.loading_more')}
                    {/if}
                  </div>
                {/if}
              {/if}
            </div>
          </ScrollFader>
        </Panel>
      {/if}
    </div>
  </PaneContent>
</div>
