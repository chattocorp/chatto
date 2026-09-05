<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  const { Story } = defineMeta({
    title: 'Components/Search presentation',
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  import type { MessageSearchResult } from '$lib/api-client/messageSearch';
  import { MessageSearchState } from '$lib/api-client/messageSearch';
  import { RoomKind } from '$lib/api-client/roomDirectory';
  import { Panel } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import ClampedMessagePreview from '../../../routes/chat/[serverId]/[roomId]/ClampedMessagePreview.svelte';
  import SearchAvailability from './SearchAvailability.svelte';
  import SearchResult from './SearchResult.svelte';
  import SearchResults from './SearchResults.svelte';

  const resultsState = {
    error: false,
    loading: false,
    loadingMore: false,
    hasSearched: false,
    results: [],
    nextCursor: null,
    loadMore: async () => {}
  };
  import { createPresenceCache } from '$lib/state/presenceCache.svelte';
  import { createUserProfileCache } from '$lib/state/userProfiles.svelte';

  createPresenceCache();
  createUserProfileCache();

  const timestampSettings = { effectiveTimezone: 'UTC', effectiveHour12: false };
  const result: MessageSearchResult = {
    id: 'message-1',
    roomId: 'room-1',
    roomName: 'general',
    roomKind: RoomKind.CHANNEL,
    actorId: 'user-1',
    actor: { id: 'user-1', login: 'alex', displayName: 'Alex', avatarUrl: null, deleted: false },
    body: 'The **release checklist** is ready.\n\nPlease review the changes before Friday.\n\n- Check the upgrade steps.\n- Test the new server.\n- Record the result.\n\nThank you for your help.',
    createdAt: '2026-09-05T10:00:00Z',
    threadRootEventId: 'thread-1',
    attachmentCount: 2,
    relevanceScore: 1
  };
  let availabilityState = $state(MessageSearchState.UNAVAILABLE);
  let opened = $state(false);
</script>

<Story name="Availability and retry" asChild>
  <div class="w-full max-w-2xl">
    <SearchAvailability
      state={availabilityState}
      checking={false}
      error={false}
      onRetry={() => (availabilityState = MessageSearchState.READY)}
      checkingClass="flex min-h-64 items-center justify-center text-muted"
    >
      {#snippet frame(content)}<Panel>{@render content()}</Panel>{/snippet}
      <Panel title="Search query">
        <p>Search is ready.</p>
        <Button
          variant="secondary"
          onclick={() => (availabilityState = MessageSearchState.UNAVAILABLE)}
        >
          Show unavailable state
        </Button>
      </Panel>
    </SearchAvailability>
  </div>
</Story>

<Story name="Checking availability" asChild>
  <div class="w-full max-w-2xl">
    <SearchAvailability
      state={MessageSearchState.UNSPECIFIED}
      checking
      error={false}
      onRetry={() => {}}
      checkingClass="flex min-h-64 items-center justify-center text-muted"
    >
      {#snippet frame(content)}<Panel>{@render content()}</Panel>{/snippet}
      <p>Search is ready.</p>
    </SearchAvailability>
  </div>
</Story>

<Story name="Indexing in sidebar" asChild>
  <div class="flex h-96 w-80 flex-col">
    <SearchAvailability
      state={MessageSearchState.INDEXING}
      checking={false}
      error={false}
      onRetry={() => {}}
      checkingClass="flex min-h-32 flex-1 items-center justify-center p-4 text-center text-sm text-muted"
    >
      {#snippet frame(content)}
        <div class="flex min-h-0 flex-1 flex-col justify-center p-4">{@render content()}</div>
      {/snippet}
      <p>Search is ready.</p>
    </SearchAvailability>
  </div>
</Story>

<Story name="Page result" asChild>
  <div class="w-full max-w-2xl">
    <Panel title="Results" noPadding>
      <ol class="selectable-list gap-4">
        <li>
          <SearchResult
            {result}
            {timestampSettings}
            timestampLocale="en-GB"
            onOpen={() => (opened = true)}
          >
            {#snippet headerMeta()}
              <span class="text-xs text-muted">#general</span>
              <time class="text-xs text-muted" datetime={result.createdAt}>5 September, 10:00</time>
            {/snippet}
          </SearchResult>
        </li>
      </ol>
    </Panel>
    {#if opened}<p role="status">Result selected.</p>{/if}
  </div>
</Story>

<Story name="Sidebar result" asChild>
  <div class="w-80">
    <ol class="selectable-list gap-3 py-2">
      <li>
        <SearchResult
          {result}
          {timestampSettings}
          timestampLocale="en-GB"
          class="group/search-result"
          aria-label={`Alex: ${result.body}`}
          attachmentClass="text-xs"
          onOpen={() => (opened = true)}
        >
          {#snippet preview(content)}
            <div class="pointer-events-none" inert>
              <ClampedMessagePreview>{@render content()}</ClampedMessagePreview>
            </div>
          {/snippet}
          {#snippet headerMeta()}
            <time class="text-xs text-muted" datetime={result.createdAt}>5 September, 10:00</time>
          {/snippet}
        </SearchResult>
      </li>
    </ol>
    {#if opened}<p role="status">Result selected.</p>{/if}
  </div>
</Story>

<Story name="Result states" asChild>
  <div class="grid w-full max-w-4xl gap-4 md:grid-cols-2">
    <Panel title="Page prompt" noPadding>
      <SearchResults store={resultsState}><span>Result</span></SearchResults>
    </Panel>
    <Panel title="Sidebar loading" noPadding>
      <SearchResults store={{ ...resultsState, loading: true }} compact
        ><span>Result</span></SearchResults
      >
    </Panel>
    <Panel title="Empty results" noPadding>
      <SearchResults store={{ ...resultsState, hasSearched: true }}
        ><span>Result</span></SearchResults
      >
    </Panel>
    <Panel title="Search error" noPadding>
      <SearchResults store={{ ...resultsState, error: true }}><span>Result</span></SearchResults>
    </Panel>
  </div>
</Story>
