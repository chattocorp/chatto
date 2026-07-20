<!--
@component

Searches messages on one server. Results are intentionally transient and are
cleared when the dialog closes so plaintext is not retained beyond the task.
-->
<script lang="ts">
  import type { Attachment } from 'svelte/attachments';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import Dialog from '$lib/ui/Dialog.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import ToggleChip from '$lib/ui/ToggleChip.svelte';
  import Button from '$lib/ui/form/Button.svelte';
  import TextInput from '$lib/ui/form/TextInput.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import { getUserSettings } from '$lib/state/userSettings.svelte';
  import { formatDateTime } from '$lib/utils/formatTime';
  import { getLocale } from '$lib/i18n/runtime';
  import { MessageSearchOrder, MessageSearchState } from '$lib/state/server/messageSearch.svelte';
  import * as m from '$lib/i18n/messages';

  let {
    serverId,
    roomId = null,
    roomName = null,
    onclose
  }: {
    serverId: string;
    roomId?: string | null;
    roomName?: string | null;
    onclose: () => void;
  } = $props();

  const store = $derived(serverRegistry.getStore(serverId).messageSearch);
  const userSettings = getUserSettings();
  const activeLocale = $derived(getLocale());
  let query = $state('');
  // The modal is recreated for each shallow-routed invocation, so the scope is
  // intentionally captured once and can then be widened locally.
  // svelte-ignore state_referenced_locally
  let scopedRoomId = $state(roomId);
  // svelte-ignore state_referenced_locally
  let scopedRoomName = $state(roomName);
  let order = $state(MessageSearchOrder.RELEVANCE);
  let hasSearched = $state(false);

  $effect(() => {
    void store.ensureStatus();
  });

  function close(): void {
    store.clearResults();
    onclose();
  }

  function submit(event: SubmitEvent): void {
    event.preventDefault();
    const trimmed = query.trim();
    if (!trimmed || !store.available) return;
    hasSearched = true;
    void store.search({
      query: trimmed,
      roomIds: scopedRoomId ? [scopedRoomId] : [],
      order
    });
  }

  function clearRoomScope(): void {
    scopedRoomId = null;
    scopedRoomName = null;
    if (hasSearched && query.trim()) {
      void store.search({ query: query.trim(), roomIds: [], order });
    }
  }

  function setOrder(nextOrder: MessageSearchOrder): void {
    order = nextOrder;
    if (hasSearched && query.trim()) {
      void store.search({
        query: query.trim(),
        roomIds: scopedRoomId ? [scopedRoomId] : [],
        order
      });
    }
  }

  function openResult(result: { roomId: string; id: string }): void {
    store.clearResults();
    void goto(
      resolve('/chat/[serverId]/[roomId]/m/[messageId]', {
        serverId: serverIdToSegment(serverId),
        roomId: result.roomId,
        messageId: result.id
      })
    );
  }

  function loadMoreWhenVisible(node: HTMLElement): ReturnType<Attachment> {
    if (typeof IntersectionObserver === 'undefined') return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) void store.loadMore();
      },
      { rootMargin: '160px 0px' }
    );
    observer.observe(node);
    return () => observer.disconnect();
  }

  function formatTimestamp(value: string): string {
    return value ? formatDateTime(value, userSettings, activeLocale) : '';
  }

  function progressPercent(): number | null {
    const indexed = store.status.indexedEventCount;
    const target = store.status.targetEventCount;
    if (indexed === null || target === null || target === 0n) return null;
    return Math.min(100, Math.round((Number(indexed) / Number(target)) * 100));
  }
</script>

<Dialog visible title={m['search.title']()} size="lg" onclose={close}>
  {#if store.statusLoading && !store.statusLoaded}
    <div class="flex min-h-64 items-center justify-center text-muted" aria-live="polite">
      <span class="mr-2 iconify animate-spin uil--spinner-alt" aria-hidden="true"></span>
      {m['search.checking']()}
    </div>
  {:else if store.statusError || store.status.state === MessageSearchState.UNAVAILABLE}
    <div class="flex min-h-64 flex-col">
      <EmptyState icon="uil--cloud-slash" title={m['search.unavailable.title']()}>
        <p>{m['search.unavailable.description']()}</p>
        <div class="mt-4">
          <Button variant="secondary" onclick={() => void store.refreshStatus()}>
            {m['common.retry']()}
          </Button>
        </div>
      </EmptyState>
    </div>
  {:else if store.status.state === MessageSearchState.DISABLED}
    <div class="flex min-h-64 flex-col">
      <EmptyState icon="uil--search-alt" title={m['search.disabled.title']()}>
        {m['search.disabled.description']()}
      </EmptyState>
    </div>
  {:else if store.status.state === MessageSearchState.STARTING || store.status.state === MessageSearchState.INDEXING}
    <div class="flex min-h-64 flex-col">
      <EmptyState icon="uil--database" title={m['search.indexing.title']()}>
        <p>{m['search.indexing.description']()}</p>
        {#if progressPercent() !== null}
          <div
            class="mx-auto mt-4 w-56"
            aria-label={m['search.indexing.progress']({ percent: progressPercent()! })}
          >
            <div class="h-1.5 overflow-hidden rounded-full bg-surface-emphasized">
              <div class="h-full bg-action" style:width={`${progressPercent()}%`}></div>
            </div>
            <p class="mt-2 tabular-nums">
              {m['search.indexing.progress']({ percent: progressPercent()! })}
            </p>
          </div>
        {/if}
        <div class="mt-4">
          <Button variant="secondary" onclick={() => void store.refreshStatus()}>
            {m['search.check_again']()}
          </Button>
        </div>
      </EmptyState>
    </div>
  {:else}
    <form class="flex flex-col gap-3" onsubmit={submit}>
      <div class="flex items-end gap-2">
        <div class="min-w-0 flex-1">
          <TextInput
            label={m['search.query.label']()}
            bind:value={query}
            placeholder={m['search.query.placeholder']()}
            leadingIcon="uil--search"
            autocomplete="off"
            autofocus
          />
        </div>
        <Button type="submit" disabled={!query.trim()} loading={store.loading}>
          {m['search.action']()}
        </Button>
      </div>

      <div class="flex flex-wrap items-center justify-between gap-2">
        <div class="flex flex-wrap items-center gap-2">
          {#if scopedRoomId}
            <ToggleChip pressed onclick={clearRoomScope} title={m['search.scope.clear']()}>
              <span class="uil--hashtag iconify" aria-hidden="true"></span>
              {scopedRoomName ?? m['search.scope.room']()}
              <span class="iconify uil--times" aria-hidden="true"></span>
            </ToggleChip>
          {:else}
            <span class="text-xs text-muted">{m['search.scope.all_rooms']()}</span>
          {/if}
        </div>
        <div class="flex items-center gap-2" aria-label={m['search.order.label']()}>
          <ToggleChip
            pressed={order === MessageSearchOrder.RELEVANCE}
            onclick={() => setOrder(MessageSearchOrder.RELEVANCE)}
            >{m['search.order.relevance']()}</ToggleChip
          >
          <ToggleChip
            pressed={order === MessageSearchOrder.NEWEST}
            onclick={() => setOrder(MessageSearchOrder.NEWEST)}
            >{m['search.order.newest']()}</ToggleChip
          >
        </div>
      </div>
    </form>

    {#if store.status.state === MessageSearchState.DEGRADED}
      <div
        class="mt-4 rounded-md border border-warning/25 bg-warning/8 px-3 py-2 text-sm text-warning"
      >
        {m['search.degraded']()}
      </div>
    {/if}

    <div class="mt-4 min-h-72 border-t border-text/10 pt-2" aria-live="polite">
      {#if store.loading}
        <div class="flex min-h-64 items-center justify-center text-muted">
          <span class="mr-2 iconify animate-spin uil--spinner-alt" aria-hidden="true"></span>
          {m['search.searching']()}
        </div>
      {:else if store.error}
        <div class="flex min-h-64 flex-col">
          <EmptyState icon="uil--exclamation-triangle" title={m['search.error.title']()}>
            {m['search.error.description']()}
          </EmptyState>
        </div>
      {:else if hasSearched && store.results.length === 0}
        <div class="flex min-h-64 flex-col">
          <EmptyState icon="uil--search-minus" title={m['search.no_results.title']()}>
            {m['search.no_results.description']()}
          </EmptyState>
        </div>
      {:else if !hasSearched}
        <div class="flex min-h-64 flex-col">
          <EmptyState icon="uil--search" title={m['search.prompt.title']()}>
            {m['search.prompt.description']()}
          </EmptyState>
        </div>
      {:else}
        <ol class="divide-y divide-text/10">
          {#each store.results as result (result.id)}
            <li>
              <button
                type="button"
                class="group w-full cursor-pointer rounded-md px-2 py-3 text-left transition-colors hover:bg-surface-emphasized focus-visible:outline-2 focus-visible:outline-action"
                onclick={() => openResult(result)}
              >
                <div class="flex items-center gap-2 text-xs text-muted">
                  <span class="font-semibold text-text">
                    {result.actor?.displayName || result.actor?.login || m['common.unknown']()}
                  </span>
                  <span aria-hidden="true">·</span>
                  <span class="inline-flex min-w-0 items-center gap-0.5">
                    <span class="uil--hashtag iconify shrink-0" aria-hidden="true"></span>
                    <span class="truncate">{result.roomName ?? m['search.scope.room']()}</span>
                  </span>
                  {#if result.createdAt}
                    <span aria-hidden="true">·</span>
                    <time class="truncate" datetime={result.createdAt}
                      >{formatTimestamp(result.createdAt)}</time
                    >
                  {/if}
                </div>
                {#if result.body}
                  <p class="mt-1 line-clamp-3 text-sm break-words whitespace-pre-wrap text-text/90">
                    {result.body}
                  </p>
                {/if}
                {#if result.attachmentCount > 0}
                  <p class="mt-1 inline-flex items-center gap-1 text-xs text-muted">
                    <span class="iconify uil--paperclip" aria-hidden="true"></span>
                    {m['search.attachments']({ count: result.attachmentCount })}
                  </p>
                {/if}
              </button>
            </li>
          {/each}
        </ol>
        {#if store.nextCursor}
          <div
            {@attach loadMoreWhenVisible}
            class="flex h-12 items-center justify-center text-sm text-muted"
          >
            {#if store.loadingMore}
              <span class="mr-2 iconify animate-spin uil--spinner-alt" aria-hidden="true"></span>
              {m['search.loading_more']()}
            {/if}
          </div>
        {/if}
      {/if}
    </div>
  {/if}
</Dialog>
