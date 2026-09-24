<!-- @component Public bot permissions. The query refreshes every 30 seconds
while mounted and discards cached data when the profile closes. -->
<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { createPermissionAPI, type MatrixData } from '$lib/api-client/permissions';
  import { createEffectivePermissionAPI } from '$lib/api-client/effectivePermissions';
  import { m } from '$lib/i18n/messages';
  import { queryClient } from '$lib/query/client';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import RoomGroupSection from '$lib/components/chat/RoomGroupSection.svelte';
  import { serverStorageKey } from '$lib/storage/serverStorage';
  import { Button } from '$lib/ui/form';
  import LoadingFog from '$lib/ui/LoadingFog.svelte';
  import {
    groupBotPermissions,
    compactEffectivePermissions,
    inactiveBotGrants,
    type BotPermissionGroup
  } from './botPermissionText';

  let {
    botId,
    botOwnerId,
    class: className = 'mt-6'
  }: {
    botId: string;
    botOwnerId?: string;
    /** Spacing supplied by the profile that owns this section. */
    class?: string;
  } = $props();
  const scope = useServerScope();
  const query = createQuery(
    () => ({
      queryKey: [
        'server',
        scope.serverId,
        'session',
        scope.connection.queryScope,
        'bot-permissions',
        botId
      ],
      queryFn: ({ signal }) =>
        scope.connection
          .getAPI(createEffectivePermissionAPI)
          .listEffectivePermissions(botId, signal),
      refetchInterval: 30_000,
      gcTime: 0
    }),
    () => queryClient
  );
  const active = $derived(groupBotPermissions(compactEffectivePermissions(query.data ?? [])));
  const canManage = $derived(
    !!scope.store?.projection.viewer?.user?.profile &&
      !scope.store.projection.viewer.user.profile.bot &&
      (scope.store.projection.viewer.user.profile.id === botOwnerId ||
        scope.store.projection.viewer.viewerPermissions?.permissions.some(
          (entry) => entry.permission === 'bot.manage' && entry.granted
        ))
  );
  const configuration = createQuery(
    () => ({
      queryKey: [
        'server',
        scope.serverId,
        'session',
        scope.connection.queryScope,
        'bot-permission-configuration',
        botId
      ],
      enabled: canManage,
      queryFn: async ({ signal }) => {
        const api = scope.connection.getAPI(createPermissionAPI);
        const matrix: MatrixData = { applicablePermissions: [], scopes: [], cells: [] };
        let offset = 0;
        while (true) {
          const page = await api.getUserPermissionMatrix(botId, {
            signal,
            page: { limit: 100, offset }
          });
          if (!page) throw new Error('Missing bot permission configuration');
          matrix.applicablePermissions.push(...page.applicablePermissions);
          matrix.scopes.push(...page.scopes);
          matrix.cells.push(...page.cells);
          if (!page.page.hasMore) break;
          if (!page.scopes.length) throw new Error('Empty bot configuration scope page');
          offset += page.scopes.length;
        }
        matrix.applicablePermissions = [...new Set(matrix.applicablePermissions)];
        return matrix;
      },
      refetchInterval: 30_000,
      gcTime: 0
    }),
    () => queryClient
  );
  const inactive = $derived(
    canManage && !configuration.isError && configuration.data
      ? groupBotPermissions(inactiveBotGrants(configuration.data), true)
      : []
  );
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
      <LoadingFog class="h-24 w-full" />
    {:else}
      {#if active.length > 0}
        {@render permissionGroups(active)}
      {:else}
        <p class="text-muted">{m('chat.profile.permissions.empty')}</p>
      {/if}
      {#if inactive.length > 0 || (canManage && configuration.isError)}
        <details class="space-y-3 text-muted">
          <summary class="cursor-pointer font-medium"
            >{m('chat.profile.permissions.inactive_title')}</summary
          >
          <p>{m('chat.profile.permissions.inactive_note')}</p>
          {#if configuration.isError}
            <p role="alert">{m('chat.profile.permissions.error')}</p>
            <Button variant="secondary" onclick={() => configuration.refetch()}
              >{m('common.retry')}</Button
            >
          {:else}
            {@render permissionGroups(inactive)}
          {/if}
        </details>
      {/if}
    {/if}
  </div>
{/snippet}

<div class={['-mx-4', className]}>
  <RoomGroupSection
    label={m('chat.profile.permissions.title')}
    persistKey={serverStorageKey(scope.serverId, 'bot-profile-permissions-collapsed')}
    items={[{ id: 'permissions' }]}
    item={permissionContent}
    separated
  />
</div>
