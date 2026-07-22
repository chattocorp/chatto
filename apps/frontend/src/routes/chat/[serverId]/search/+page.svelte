<!--
@component

Server-local message search. Query text and hydrated results remain transient
in the active server store so browser Back can restore the current search.
-->
<script lang="ts">
  import type { Attachment } from 'svelte/attachments';
  import { tick } from 'svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { Panel } from '$lib/components/admin';
  import MessageView from '$lib/components/messages/MessageView.svelte';
  import { PresenceStatus, type UserAvatarUserView } from '$lib/render/types';
  import type { MessageSearchResult } from '$lib/api-client/messageSearch';
  import { RoomKind } from '$lib/api-client/roomDirectory';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { hour12ForTimeFormat } from '$lib/state/userSettings.svelte';
  import { MessageSearchOrder, MessageSearchState } from '$lib/state/server/messageSearch.svelte';
  import { getLocale } from '$lib/i18n/runtime';
  import { formatDateTime } from '$lib/utils/formatTime';
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
  import * as m from '$lib/i18n/messages';

  const serverId = $derived(getActiveServer());
  const serverStore = $derived(serverRegistry.getStore(serverId));
  const store = $derived(serverStore.messageSearch);
  const timeFormatSettings = $derived.by(() => {
    const settings = serverStore.currentUser.user?.settings;
    return {
      effectiveTimezone: settings?.timezone || undefined,
      effectiveHour12:
        settings?.timeFormat === undefined ? undefined : hour12ForTimeFormat(settings.timeFormat)
    };
  });
  const activeLocale = $derived(getLocale());
  const orderOptions = $derived([
    { value: MessageSearchOrder.RELEVANCE, label: m['search.order.relevance']() },
    { value: MessageSearchOrder.NEWEST, label: m['search.order.newest']() }
  ]);
  $effect(() => {
    void store.ensureStatus();
  });

  function submit(event: SubmitEvent): void {
    event.preventDefault();
    const trimmed = store.query.trim();
    if (!trimmed || !store.available) return;
    void store.search({ query: trimmed, order: store.order });
  }

  function setOrder(nextOrder: MessageSearchOrder): void {
    store.order = nextOrder;
    if (store.hasSearched && store.query.trim()) {
      void store.search({ query: store.query.trim(), order: store.order });
    }
  }

  function resultActor(result: MessageSearchResult): UserAvatarUserView | null {
    if (!result.actor) return null;
    return {
      ...result.actor,
      presenceStatus: PresenceStatus.Offline
    };
  }

  function loadMoreWhenVisible(node: HTMLElement): ReturnType<Attachment> {
    if (typeof IntersectionObserver === 'undefined') return;
    let loadingVisiblePages = false;
    const loadVisiblePages = async (): Promise<void> => {
      if (loadingVisiblePages) return;
      loadingVisiblePages = true;
      try {
        do {
          const cursor = store.nextCursor;
          await store.loadMore();
          await tick();
          if (store.error || store.nextCursor === cursor) break;
          const bounds = node.getBoundingClientRect();
          if (bounds.top > window.innerHeight + 160 || bounds.bottom < -160) break;
        } while (store.nextCursor && node.isConnected);
      } finally {
        loadingVisiblePages = false;
      }
    };
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) void loadVisiblePages();
      },
      { rootMargin: '160px 0px' }
    );
    observer.observe(node);
    return () => observer.disconnect();
  }

  function formatTimestamp(value: string): string {
    return value ? formatDateTime(value, timeFormatSettings, activeLocale) : '';
  }

  function navigateToResult(result: MessageSearchResult): void {
    if (result.threadRootEventId) {
      void goto(
        resolve('/chat/[serverId]/[roomId]/[threadId]/m/[messageId]', {
          serverId: serverIdToSegment(serverId),
          roomId: result.roomId,
          threadId: result.threadRootEventId,
          messageId: result.id
        })
      );
      return;
    }
    void goto(
      resolve('/chat/[serverId]/[roomId]/m/[messageId]', {
        serverId: serverIdToSegment(serverId),
        roomId: result.roomId,
        messageId: result.id
      })
    );
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

<PageTitle title={m['search.title']()} />

<div class="pane-page">
  <PaneHeader title={m['search.title']()} showMobileNav />

  <PaneContent fillHeight>
    <div class="flex min-h-0 flex-1 flex-col gap-6">
      {#if store.statusLoading && !store.statusLoaded}
        <Panel>
          <div class="flex min-h-64 items-center justify-center text-muted" aria-live="polite">
            <span class="mr-2 iconify animate-spin uil--spinner-alt" aria-hidden="true"></span>
            {m['search.checking']()}
          </div>
        </Panel>
      {:else if store.statusError || store.status.state === MessageSearchState.UNAVAILABLE}
        <Panel>
          <EmptyState icon="uil--cloud-slash" title={m['search.unavailable.title']()}>
            <p>{m['search.unavailable.description']()}</p>
            <div class="mt-4">
              <Button variant="secondary" onclick={() => void store.refreshStatus()}>
                {m['common.retry']()}
              </Button>
            </div>
          </EmptyState>
        </Panel>
      {:else if store.status.state === MessageSearchState.DISABLED}
        <Panel>
          <EmptyState icon="uil--search-alt" title={m['search.disabled.title']()}>
            {m['search.disabled.description']()}
          </EmptyState>
        </Panel>
      {:else if store.status.state === MessageSearchState.STARTING || store.status.state === MessageSearchState.INDEXING}
        <Panel>
          <EmptyState icon="uil--database" title={m['search.indexing.title']()}>
            <p>{m['search.indexing.description']()}</p>
            <div class="mt-4">
              <Button variant="secondary" onclick={() => void store.refreshStatus()}>
                {m['search.check_again']()}
              </Button>
            </div>
          </EmptyState>
        </Panel>
      {:else}
        <Panel title={m['search.query.label']()}>
          <form class="flex flex-wrap items-stretch gap-2" onsubmit={submit}>
            <div class="min-w-64 flex-1">
              <TextInput
                label={m['search.query.label']()}
                labelHidden
                bind:value={store.query}
                placeholder={m['search.query.placeholder']()}
                leadingIcon="uil--search"
                autocomplete="off"
                autofocus
              />
            </div>
            <Button type="submit" disabled={!store.query.trim()} loading={store.loading}>
              {m['search.action']()}
            </Button>
            <SegmentedControl
              label={m['search.order.label']()}
              options={orderOptions}
              value={store.order}
              onchange={setOrder}
            />
          </form>

          {#if store.status.state === MessageSearchState.DEGRADED}
            <div class="mt-4">
              <Hint tone="warning">{m['search.degraded']()}</Hint>
            </div>
          {/if}
        </Panel>

        <Panel title={m['search.results']()} noPadding fillHeight>
          <ScrollFader top bottom class="min-h-0 flex-1">
            <div class="flex min-h-full flex-col" aria-live="polite">
              {#if store.loading}
                <div class="flex flex-1 items-center justify-center text-muted">
                  <span class="mr-2 iconify animate-spin uil--spinner-alt" aria-hidden="true"
                  ></span>
                  {m['search.searching']()}
                </div>
              {:else if store.error}
                <EmptyState icon="uil--exclamation-triangle" title={m['search.error.title']()}>
                  {m['search.error.description']()}
                </EmptyState>
              {:else if store.hasSearched && store.results.length === 0 && !store.nextCursor}
                <EmptyState icon="uil--search-minus" title={m['search.no_results.title']()}>
                  {m['search.no_results.description']()}
                </EmptyState>
              {:else if !store.hasSearched}
                <EmptyState icon="uil--search" title={m['search.prompt.title']()}>
                  {m['search.prompt.description']()}
                </EmptyState>
              {:else}
                <ol class="flex flex-col gap-4 p-1">
                  {#each store.results as result (result.id)}
                    <li>
                      <div
                        role="link"
                        tabindex="0"
                        data-search-result-id={result.id}
                        class="cursor-pointer rounded-md focus-visible:outline-2 focus-visible:outline-action"
                        onclick={(event) => openResult(event, result)}
                        onkeydown={(event) => openResultFromKeyboard(event, result)}
                      >
                        <MessageView
                          eventId={result.id}
                          actor={resultActor(result)}
                          displayName={result.actor?.displayName ||
                            result.actor?.login ||
                            m['common.unknown']()}
                          missingActorIsDeleted={false}
                          body={result.body}
                          timestampSettings={timeFormatSettings}
                          timestampLocale={activeLocale}
                          rowClass="md:mx-0 md:pr-2"
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
                                ? m['room.title.direct_message']()
                                : `#${result.roomName ?? m['search.scope.room']()}`}
                            </a>
                            {#if result.createdAt}
                              <span class="text-xs text-muted" aria-hidden="true">·</span>
                              <a
                                class="min-w-0 truncate text-xs text-muted hover:text-text hover:underline"
                                href={result.threadRootEventId
                                  ? resolve('/chat/[serverId]/[roomId]/[threadId]/m/[messageId]', {
                                      serverId: serverIdToSegment(serverId),
                                      roomId: result.roomId,
                                      threadId: result.threadRootEventId,
                                      messageId: result.id
                                    })
                                  : resolve('/chat/[serverId]/[roomId]/m/[messageId]', {
                                      serverId: serverIdToSegment(serverId),
                                      roomId: result.roomId,
                                      messageId: result.id
                                    })}
                              >
                                <time datetime={result.createdAt}
                                  >{formatTimestamp(result.createdAt)}</time
                                >
                              </a>
                            {/if}
                          {/snippet}

                          {#snippet afterBody()}
                            {#if result.attachmentCount > 0}
                              <p class="inline-flex items-center gap-1 text-sm text-muted">
                                <span class="iconify uil--paperclip" aria-hidden="true"></span>
                                {m['search.attachments']({ count: result.attachmentCount })}
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
                      <span class="mr-2 iconify animate-spin uil--spinner-alt" aria-hidden="true"
                      ></span>
                      {m['search.loading_more']()}
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
