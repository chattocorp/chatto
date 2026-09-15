<!-- @component Public bot permissions. The query refreshes every 30 seconds
while mounted and discards cached data when the profile closes. -->
<script lang="ts">
  import { createInfiniteQuery } from '@tanstack/svelte-query';
  import { createBotAPI } from '$lib/api-client/bots';
  import { m } from '$lib/i18n/messages';
  import { queryClient } from '$lib/query/client';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { Button } from '$lib/ui/form';
  import { botPermissionText, mergeBotPermissionPages } from './botPermissionText';

  let { botId }: { botId: string } = $props();
  const scope = useServerScope();
  const query = createInfiniteQuery(
    () => ({
      queryKey: [
        'server',
        scope.serverId,
        'session',
        scope.connection.queryScope,
        'bot-permissions',
        botId
      ],
      initialPageParam: 0,
      queryFn: ({ pageParam, signal }) =>
        scope.connection.getAPI(createBotAPI).listPermissions(botId, pageParam, signal),
      getNextPageParam: (page, _pages, offset) =>
        page.hasMore ? offset + page.permissions.length : undefined,
      refetchInterval: 30_000,
      gcTime: 0
    }),
    () => queryClient
  );
  const entries = $derived(mergeBotPermissionPages(query.data?.pages ?? []));
  const active = $derived(entries.filter((entry) => entry.active));
  const inactive = $derived(entries.filter((entry) => !entry.active));
</script>

<section class="mt-6 space-y-3" aria-label={m('chat.profile.permissions.title')}>
  <h3 class="font-semibold text-text-top">{m('chat.profile.permissions.title')}</h3>
  {#if query.isError}
    <p role="alert" class="text-muted">{m('chat.profile.permissions.error')}</p>
    <Button variant="secondary" onclick={() => query.refetch()}>{m('common.retry')}</Button>
  {:else if query.isPending}
    <p class="text-muted" aria-busy="true">{m('common.loading')}</p>
  {:else}
    <p class="text-muted">{m('chat.profile.permissions.note')}</p>
    {#if active.length > 0}
      <ul class="list-disc space-y-2 ps-5">
        {#each active as entry (`${entry.permission}:${entry.scope}:${entry.scopeId}`)}
          <li class="break-words">{botPermissionText(entry)}</li>
        {/each}
      </ul>
    {:else if !query.hasNextPage}
      <p class="text-muted">{m('chat.profile.permissions.empty')}</p>
    {/if}
    {#if inactive.length > 0}
      <h4 class="font-medium">{m('chat.profile.permissions.inactive_title')}</h4>
      <p class="text-muted">{m('chat.profile.permissions.inactive_note')}</p>
      <ul class="list-disc space-y-2 ps-5 text-muted">
        {#each inactive as entry (`${entry.permission}:${entry.scope}:${entry.scopeId}`)}
          <li class="break-words">{botPermissionText(entry)}</li>
        {/each}
      </ul>
    {/if}
    {#if query.hasNextPage}
      <Button
        variant="secondary"
        disabled={query.isFetchingNextPage}
        onclick={() => query.fetchNextPage()}
      >
        {m('chat.profile.permissions.more')}
      </Button>
    {/if}
  {/if}
</section>
