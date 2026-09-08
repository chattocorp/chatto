<!-- @component Retained webhook failures. Mounted on demand; the query owns paging. -->
<script lang="ts">
  import { createInfiniteQuery } from '@tanstack/svelte-query';
  import { createBotAPI } from '$lib/api-client/bots';
  import { m } from '$lib/i18n/messages';
  import { queryClient } from '$lib/query/client';
  import { settingsQueryKeys } from '$lib/query/settings';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import BotWebhookFailureDetails from './BotWebhookFailureDetails.svelte';
  import { Button } from '$lib/ui/form';

  let { botId, webhookId }: { botId: string; webhookId: string } = $props();
  const scope = useServerScope();
  const query = createInfiniteQuery(
    () => ({
      queryKey: [
        ...settingsQueryKeys.bot(scope.serverId, scope.connection, botId),
        'failures',
        webhookId
      ],
      initialPageParam: '',
      refetchInterval: 30000,
      queryFn: ({ pageParam, signal }) =>
        scope.connection
          .getAPI(createBotAPI)
          .listWebhookFailures(botId, webhookId, pageParam, signal),
      getNextPageParam: (page) => page.nextCursor || undefined,
      // Remounting starts a fresh bounded history, rather than keeping expired rows.
      gcTime: 0
    }),
    () => queryClient
  );
  const failures = $derived(query.data?.pages.flatMap((page) => page.failures) ?? []);
</script>

<div class="space-y-4" data-testid="webhook-failure-history">
  {#if query.isError}
    <p role="alert">{m('settings.bots.outbound.load_error')}</p>
    <Button variant="secondary" onclick={() => query.refetch()}>{m('common.retry')}</Button>
  {:else if query.isPending}
    <p class="text-muted">{m('common.loading')}</p>
  {:else if failures.length === 0}
    <p class="text-muted">{m('settings.bots.outbound.no_failures')}</p>
  {/if}

  {#each failures as failure (failure.id)}
    <div class="space-y-1">
      {#if failure.completedAt}
        <p>{failure.completedAt.toDate().toLocaleString()}</p>
      {/if}
      <BotWebhookFailureDetails {failure} />
    </div>
  {/each}

  {#if query.hasNextPage}
    <Button
      variant="secondary"
      disabled={query.isFetchingNextPage}
      onclick={() => query.fetchNextPage()}
    >
      {m('settings.bots.outbound.more_failures')}
    </Button>
  {/if}
</div>
