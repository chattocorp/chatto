<!--
@component

Room-scoped message search for the room sidebar. Its store is retained per room
so switching rooms cannot leak a query or plaintext results into another room.
-->
<script lang="ts">
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import type { Attachment } from 'svelte/attachments';
  import type { MessageSearchResult } from '$lib/api-client/messageSearch';
  import MessageView from '$lib/components/messages/MessageView.svelte';
  import * as m from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import type { UserAvatarUserView } from '$lib/render/users';
  import {
    MessageSearchOrder,
    MessageSearchState,
    type MessageSearchStore
  } from '$lib/state/server/messageSearch.svelte';
  import { useDebouncedMessageSearch } from '$lib/hooks/useDebouncedMessageSearch.svelte';
  import { useLoadMoreWhenVisible } from '$lib/hooks/useLoadMoreWhenVisible.svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { EmptyState, Hint, ScrollFader } from '$lib/ui';
  import { Button, TextInput } from '$lib/ui/form';
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

  function searchNow(): void {
    search.submitNow();
  }

  function submit(event: SubmitEvent): void {
    event.preventDefault();
    searchNow();
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

  function resultActor(result: MessageSearchResult): UserAvatarUserView | null {
    if (!result.actor) return null;
    return { ...result.actor, presenceStatus: PresenceStatus.OFFLINE };
  }

  function formatTimestamp(value: string): string {
    return value ? formatDateTime(value, userSettings, activeLocale) : '';
  }

  function openResult(event: MouseEvent, result: MessageSearchResult): void {
    event.preventDefault();
    onOpenResult?.(result.id, result.threadRootEventId);
  }

  function openResultFromKeyboard(event: KeyboardEvent, result: MessageSearchResult): void {
    if (event.target !== event.currentTarget || event.key !== 'Enter') return;
    event.preventDefault();
    onOpenResult?.(result.id, result.threadRootEventId);
  }
</script>

{#if store.statusLoading && !store.statusLoaded}
  <div class="flex min-h-32 flex-1 items-center justify-center p-4 text-center text-sm text-muted">
    <span class="mr-2 iconify animate-spin icon-[uil--spinner-alt]" aria-hidden="true"></span>
    {m['search.checking']()}
  </div>
{:else if store.statusError || store.status.state === MessageSearchState.UNAVAILABLE}
  <div class="flex min-h-0 flex-1 flex-col justify-center p-4">
    <EmptyState icon="icon-[uil--cloud-slash]" title={m['search.unavailable.title']()}>
      <p>{m['search.unavailable.description']()}</p>
      <div class="mt-4">
        <Button variant="secondary" onclick={() => void store.refreshStatus()}>
          {m['common.retry']()}
        </Button>
      </div>
    </EmptyState>
  </div>
{:else if store.status.state === MessageSearchState.DISABLED}
  <div class="flex min-h-0 flex-1 flex-col justify-center p-4">
    <EmptyState icon="icon-[uil--search-alt]" title={m['search.disabled.title']()}>
      {m['search.disabled.description']()}
    </EmptyState>
  </div>
{:else if store.status.state === MessageSearchState.STARTING || store.status.state === MessageSearchState.INDEXING}
  <div class="flex min-h-0 flex-1 flex-col justify-center p-4">
    <EmptyState icon="icon-[uil--database]" title={m['search.indexing.title']()}>
      <p>{m['search.indexing.description']()}</p>
      <div class="mt-4">
        <Button variant="secondary" onclick={() => void store.refreshStatus()}>
          {m['search.check_again']()}
        </Button>
      </div>
    </EmptyState>
  </div>
{:else}
  <div class="flex min-h-0 flex-1 flex-col">
    <div class="border-b border-border p-2">
      <form onsubmit={submit} {@attach focusSearchField}>
        <TextInput
          label={m['search.query.label']()}
          labelHidden
          testid="room-search-query"
          bind:value={store.query}
          placeholder={m['search.query.placeholder']()}
          leadingIcon="icon-[uil--search]"
          autocomplete="off"
          oninput={scheduleSearch}
        />
      </form>
      {#if store.status.state === MessageSearchState.DEGRADED}
        <div class="mt-2">
          <Hint tone="warning">{m['search.degraded']()}</Hint>
        </div>
      {/if}
    </div>

    <ScrollFader top bottom keyboardFocusable={false} class="min-h-0 flex-1">
      <div class="flex min-h-full flex-col" aria-live="polite">
        {#if store.error}
          <EmptyState icon="icon-[uil--exclamation-triangle]" title={m['search.error.title']()}>
            {m['search.error.description']()}
          </EmptyState>
        {:else if store.loading && store.results.length === 0}
          <div class="flex min-h-32 flex-1 items-center justify-center p-4 text-sm text-muted">
            <span class="mr-2 iconify animate-spin icon-[uil--spinner-alt]" aria-hidden="true"></span>
            {m['search.searching']()}
          </div>
        {:else if store.hasSearched && store.results.length === 0 && !store.nextCursor}
          <EmptyState icon="icon-[uil--search-minus]" title={m['search.no_results.title']()}>
            {m['search.no_results.description']()}
          </EmptyState>
        {:else if !store.hasSearched}
          <EmptyState icon="icon-[uil--search]" title={m['search.prompt.title']()} />
        {:else}
          <ol class="selectable-list gap-3 py-2">
            {#each store.results as result (result.id)}
              <li>
                <div
                  role="link"
                  tabindex="0"
                  aria-label={`${result.actor?.displayName || result.actor?.login || m['common.unknown']()}: ${result.body}`}
                  data-room-search-result-id={result.id}
                  class="group/search-result cursor-pointer selectable-list-item"
                  onclick={(event) => openResult(event, result)}
                  onkeydown={(event) => openResultFromKeyboard(event, result)}
                >
                  <div class="pointer-events-none" inert data-room-search-result-preview>
                    <ClampedMessagePreview>
                      <MessageView
                        eventId={result.id}
                        actor={resultActor(result)}
                        displayName={result.actor?.displayName ||
                          result.actor?.login ||
                          m['common.unknown']()}
                        missingActorIsDeleted={false}
                        body={result.body}
                        viewerLogin={serverScope.store.currentUser.user?.login}
                        timestampSettings={userSettings}
                        timestampLocale={activeLocale}
                        rowClass="hover:bg-transparent md:mx-0 md:pr-2"
                      >
                        {#snippet headerMeta()}
                          {#if result.createdAt}
                            <time class="text-xs text-muted" datetime={result.createdAt}>
                              {formatTimestamp(result.createdAt)}
                            </time>
                          {/if}
                        {/snippet}

                        {#snippet afterBody()}
                          {#if result.attachmentCount > 0}
                            <p class="inline-flex items-center gap-1 text-xs text-muted">
                              <span class="iconify icon-[uil--paperclip]" aria-hidden="true"></span>
                              {m['search.attachments']({ count: result.attachmentCount })}
                            </p>
                          {/if}
                        {/snippet}
                      </MessageView>
                    </ClampedMessagePreview>
                  </div>
                </div>
              </li>
            {/each}
          </ol>
          {#if store.nextCursor}
            <div
              {@attach loadMoreWhenVisible}
              class="flex h-12 items-center justify-center text-sm text-muted"
            >
              {#if store.loadingMore}
                <span class="mr-2 iconify animate-spin icon-[uil--spinner-alt]" aria-hidden="true"></span>
                {m['search.loading_more']()}
              {/if}
            </div>
          {/if}
        {/if}
      </div>
    </ScrollFader>
  </div>
{/if}
