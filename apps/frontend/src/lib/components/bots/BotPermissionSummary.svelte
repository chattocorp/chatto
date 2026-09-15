<!-- @component Public bot permissions. The query refreshes every 30 seconds
while mounted and discards cached data when the profile closes. -->
<script lang="ts">
  import { createInfiniteQuery } from '@tanstack/svelte-query';
  import { createPermissionAPI } from '$lib/api-client/permissions';
  import { m } from '$lib/i18n/messages';
  import { queryClient } from '$lib/query/client';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import RoomGroupSection from '$lib/components/chat/RoomGroupSection.svelte';
  import { serverStorageKey } from '$lib/storage/serverStorage';
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
        scope.connection
          .getAPI(createPermissionAPI)
          .getUserPermissionSummary(botId, pageParam, signal),
      getNextPageParam: (page) => page.nextOffset,
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
    <div class="flex items-start gap-2">
      <span class="mt-0.5 sidebar-icon text-muted" aria-hidden="true">
        <span class={['iconify', group.icon]}></span>
      </span>
      <div class="min-w-0 flex-1 space-y-1">
        <h4 class="font-medium break-words"><bdi>{group.label}</bdi></h4>
        <ul role="list" class="space-y-1 text-muted">
          {#each group.actions as action (action.id)}
            <li class="break-words">{action.text}</li>
          {/each}
        </ul>
      </div>
    </div>
  {/each}
{/snippet}

{#snippet permissionContent()}
  <div class="space-y-4 px-1 pt-2 pb-2">
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
  </div>
{/snippet}

<div class="-mx-4 mt-6">
  <RoomGroupSection
    label={m('chat.profile.permissions.title')}
    persistKey={serverStorageKey(scope.serverId, 'bot-profile-permissions-collapsed')}
    items={[{ id: 'permissions' }]}
    item={permissionContent}
    separated
  />
</div>
