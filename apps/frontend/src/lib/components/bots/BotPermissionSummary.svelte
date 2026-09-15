<!-- @component Public bot permissions. The query refreshes every 30 seconds
while mounted and discards cached data when the profile closes. -->
<script lang="ts">
  import { createInfiniteQuery } from '@tanstack/svelte-query';
  import { createBotAPI } from '$lib/api-client/bots';
  import { m } from '$lib/i18n/messages';
  import { queryClient } from '$lib/query/client';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { HelpTooltip } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import {
    groupBotPermissions,
    mergeBotPermissionPages,
    type BotPermissionGroup
  } from './botPermissionText';

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
  const active = $derived(groupBotPermissions(entries.filter((entry) => entry.active)));
  const inactive = $derived(groupBotPermissions(entries.filter((entry) => !entry.active)));
</script>

{#snippet permissionGroups(groups: BotPermissionGroup[])}
  {#each groups as group (group.id)}
    <div class="space-y-1">
      <h4 class="break-words font-medium"><bdi>{group.label}</bdi></h4>
      <ul class="list-disc space-y-1 ps-5">
        {#each group.actions as action (action.id)}
          <li class="break-words">{action.text}</li>
        {/each}
      </ul>
    </div>
  {/each}
{/snippet}

<section class="mt-6 space-y-3" aria-label={m('chat.profile.permissions.title')}>
  <div class="flex items-center gap-2">
    <h3 class="font-semibold text-text-top">{m('chat.profile.permissions.title')}</h3>
    <HelpTooltip>{m('chat.profile.permissions.note')}</HelpTooltip>
  </div>
  {#if query.isError}
    <p role="alert" class="text-muted">{m('chat.profile.permissions.error')}</p>
    <Button variant="secondary" onclick={() => query.refetch()}>{m('common.retry')}</Button>
  {:else if query.isPending}
    <p class="text-muted" aria-busy="true">{m('common.loading')}</p>
  {:else}
    {#if active.length > 0}
      {@render permissionGroups(active)}
    {:else if !query.hasNextPage}
      <p class="text-muted">{m('chat.profile.permissions.empty')}</p>
    {/if}
    {#if inactive.length > 0}
      <details class="space-y-3 text-muted">
        <summary class="cursor-pointer font-medium"
          >{m('chat.profile.permissions.inactive_title')}</summary
        >
        <p>{m('chat.profile.permissions.inactive_note')}</p>
        {@render permissionGroups(inactive)}
      </details>
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
