<!--
@component

Server-local message search. Query text and hydrated results remain transient
in the active server store so browser Back can restore the current search.
-->
<script lang="ts">
  import type { Attachment } from 'svelte/attachments';
  import { tick } from 'svelte';
  import { resolve } from '$app/paths';
  import MessageView from '$lib/components/messages/MessageView.svelte';
  import { PresenceStatus, type UserAvatarUserView } from '$lib/render/types';
  import type { MessageSearchResult } from '$lib/api-client/messageSearch';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { hour12ForTimeFormat } from '$lib/state/userSettings.svelte';
  import { MessageSearchOrder, MessageSearchState } from '$lib/state/server/messageSearch.svelte';
  import { getLocale } from '$lib/i18n/runtime';
  import { formatDateTime } from '$lib/utils/formatTime';
  import { EmptyState, PageTitle, PaneHeader, ToggleChip } from '$lib/ui';
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
</script>

<PageTitle title={m['search.title']()} />

<div class="flex h-full w-full flex-col">
  <PaneHeader title={m['search.title']()} showMobileNav />

  <div class="flex min-h-0 flex-1 flex-col overflow-y-auto">
    <div class="mx-auto flex w-full max-w-4xl flex-1 flex-col p-4 md:p-6">
      {#if store.statusLoading && !store.statusLoaded}
        <div class="flex min-h-64 items-center justify-center text-muted" aria-live="polite">
          <span class="mr-2 iconify animate-spin uil--spinner-alt" aria-hidden="true"></span>
          {m['search.checking']()}
        </div>
      {:else if store.statusError || store.status.state === MessageSearchState.UNAVAILABLE}
        <EmptyState icon="uil--cloud-slash" title={m['search.unavailable.title']()}>
          <p>{m['search.unavailable.description']()}</p>
          <div class="mt-4">
            <Button variant="secondary" onclick={() => void store.refreshStatus()}>
              {m['common.retry']()}
            </Button>
          </div>
        </EmptyState>
      {:else if store.status.state === MessageSearchState.DISABLED}
        <EmptyState icon="uil--search-alt" title={m['search.disabled.title']()}>
          {m['search.disabled.description']()}
        </EmptyState>
      {:else if store.status.state === MessageSearchState.STARTING || store.status.state === MessageSearchState.INDEXING}
        <EmptyState icon="uil--database" title={m['search.indexing.title']()}>
          <p>{m['search.indexing.description']()}</p>
          <div class="mt-4">
            <Button variant="secondary" onclick={() => void store.refreshStatus()}>
              {m['search.check_again']()}
            </Button>
          </div>
        </EmptyState>
      {:else}
        <form class="flex flex-col gap-3" onsubmit={submit}>
          <div class="flex items-end gap-2">
            <div class="min-w-0 flex-1">
              <TextInput
                label={m['search.query.label']()}
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
          </div>

          <div class="flex items-center justify-between gap-2">
            <span class="text-sm text-muted">{m['search.scope.all_rooms']()}</span>
            <div class="flex items-center gap-2" aria-label={m['search.order.label']()}>
              <ToggleChip
                pressed={store.order === MessageSearchOrder.RELEVANCE}
                onclick={() => setOrder(MessageSearchOrder.RELEVANCE)}
                >{m['search.order.relevance']()}</ToggleChip
              >
              <ToggleChip
                pressed={store.order === MessageSearchOrder.NEWEST}
                onclick={() => setOrder(MessageSearchOrder.NEWEST)}
                >{m['search.order.newest']()}</ToggleChip
              >
            </div>
          </div>
        </form>

        {#if store.status.state === MessageSearchState.DEGRADED}
          <div class="mt-4 rounded-md border border-warning/25 bg-warning/8 px-3 py-2 text-warning">
            {m['search.degraded']()}
          </div>
        {/if}

        <div class="mt-4 min-h-72 border-t border-border pt-2" aria-live="polite">
          {#if store.loading}
            <div class="flex min-h-64 items-center justify-center text-muted">
              <span class="mr-2 iconify animate-spin uil--spinner-alt" aria-hidden="true"></span>
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
            <ol class="divide-y divide-border">
              {#each store.results as result (result.id)}
                <li class="py-2 first:pt-0 last:pb-0">
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
                    class="my-0.5"
                  >
                    {#snippet headerMeta()}
                      <span class="inline-flex min-w-0 items-center gap-1 text-xs text-muted">
                        <span class="shrink-0" aria-hidden="true">#</span>
                        <a
                          class="min-w-0 truncate hover:text-text hover:underline"
                          href={resolve('/chat/[serverId]/[roomId]', {
                            serverId: serverIdToSegment(serverId),
                            roomId: result.roomId
                          })}
                        >
                          {result.roomName ?? m['search.scope.room']()}
                        </a>
                      </span>
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
      {/if}
    </div>
  </div>
</div>
