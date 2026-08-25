<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { createInfiniteQuery, createQuery } from '@tanstack/svelte-query';
  import { createBotAPI } from '$lib/api-client/bots';
  import { createUserAPI } from '$lib/api-client/users';
  import { viewerResponseToState } from '$lib/api-client/viewer';
  import { DataTable, Panel } from '$lib/components/admin';
  import UserIdentity from '$lib/components/users/UserIdentity.svelte';
  import { useDebounce } from '$lib/hooks/useDebounce.svelte';
  import { m } from '$lib/i18n/messages';
  import { serverIdToSegment } from '$lib/navigation';
  import { queryClient } from '$lib/query/client';
  import { settingsQueryKeys } from '$lib/query/settings';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { Dialog, FormDialog, Hint, PageTitle, PaneContent, PaneHeader } from '$lib/ui';
  import { Button, TextInput, validate, z } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { SvelteSet } from 'svelte/reactivity';
  import { onDestroy } from 'svelte';

  const PAGE_SIZE = 20;
  const serverScope = useServerScope();
  const supportsBots = $derived(serverScope.store.serverInfo.supportsFeature('botAccounts'));
  const canCreateBots = $derived.by(() => {
    const viewer = serverScope.store.projection.viewer;
    return viewer
      ? (viewerResponseToState(viewer).viewerPermissions['bot.create'] ?? false)
      : false;
  });

  let searchInput = $state('');
  let activeSearch = $state('');
  let scrollContainer = $state<HTMLDivElement>();
  const searchDebounce = useDebounce();
  let componentActive = true;

  onDestroy(() => {
    componentActive = false;
  });

  const botsQuery = createInfiniteQuery(
    () => {
      const serverId = serverScope.serverId;
      const connection = serverScope.connection;
      const search = activeSearch;
      return {
        queryKey: settingsQueryKeys.bots(serverId, connection, search),
        queryFn: ({ pageParam, signal }) =>
          connection
            .getAPI(createBotAPI)
            .listBots({ search: search || null, limit: PAGE_SIZE, offset: pageParam }, { signal }),
        initialPageParam: 0,
        getNextPageParam: (lastPage, _pages, lastPageParam) =>
          lastPage.hasMore && lastPage.bots.length > 0
            ? lastPageParam + lastPage.bots.length
            : undefined,
        enabled: supportsBots
      };
    },
    () => queryClient
  );

  const bots = $derived.by(() => {
    const seen = new SvelteSet<string>();
    return (botsQuery.data?.pages ?? []).flatMap((page) =>
      page.bots.filter((bot) => {
        if (seen.has(bot.id)) return false;
        seen.add(bot.id);
        return true;
      })
    );
  });
  const totalCount = $derived(botsQuery.data?.pages.at(-1)?.totalCount ?? bots.length);
  const ownerUserIds = $derived.by(() => {
    const ids = new SvelteSet<string>();
    for (const bot of bots) ids.add(bot.ownerUserId);
    return [...ids];
  });
  const ownersQuery = createQuery(
    () => {
      const serverId = serverScope.serverId;
      const connection = serverScope.connection;
      const userIds = ownerUserIds;
      return {
        queryKey: [...settingsQueryKeys.botsRoot(serverId, connection), 'owners', userIds],
        queryFn: async () => {
          const api = connection.getAPI(createUserAPI);
          const batches = [];
          for (let offset = 0; offset < userIds.length; offset += 100) {
            batches.push(api.batchGetUsers(userIds.slice(offset, offset + 100)));
          }
          return (await Promise.all(batches)).flat();
        },
        enabled: supportsBots && userIds.length > 0
      };
    },
    () => queryClient
  );
  const ownersById = $derived(
    new Map((ownersQuery.data ?? []).map((owner) => [owner.id, owner]))
  );

  let createVisible = $state(false);
  let createLogin = $state('');
  let createDisplayName = $state('');
  let createLoading = $state(false);
  let createError = $state<string | null>(null);
  let apiKeyVisible = $state(false);
  let apiKey = $state('');
  let createdBotId = $state<string | null>(null);

  const botLoginSchema = z
    .string()
    .min(2, m('common.validation.username_min'))
    .max(32, m('common.validation.username_max'))
    .regex(/^[a-zA-Z0-9][a-zA-Z0-9._-]*$/, m('common.validation.username_charset'))
    .refine((value) => !value.endsWith('.'), m('common.validation.username_end_alphanumeric'))
    .refine((value) => value.toLowerCase().endsWith('_bot'), m('settings.bots.username_hint'));
  const normalizedCreateLogin = $derived(createLogin.trim());
  const createLoginError = $derived(
    normalizedCreateLogin ? validate(botLoginSchema, normalizedCreateLogin) : undefined
  );

  function scheduleSearch(event: Event) {
    const value = event.currentTarget instanceof HTMLInputElement ? event.currentTarget.value : '';
    searchInput = value;
    searchDebounce.run(() => {
      const nextSearch = value.trim();
      if (nextSearch !== activeSearch) activeSearch = nextSearch;
    }, 300);
  }

  async function loadMore() {
    if (botsQuery.isPending || botsQuery.isFetchingNextPage || !botsQuery.hasNextPage) return;
    await botsQuery.fetchNextPage();
  }

  function botHref(botId: string) {
    return resolve('/chat/[serverId]/manage/server/bots/[botId]', {
      serverId: serverIdToSegment(serverScope.serverId),
      botId
    });
  }

  function openCreate() {
    if (!canCreateBots) return;
    createLogin = '';
    createDisplayName = '';
    createError = null;
    createVisible = true;
  }

  async function createBot() {
    if (!canCreateBots || !normalizedCreateLogin || createLoginError) return;
    const serverId = serverScope.serverId;
    const connection = serverScope.connection;
    createLoading = true;
    createError = null;
    try {
      const created = await connection.getAPI(createBotAPI).createBot({
        login: normalizedCreateLogin,
        displayName: createDisplayName.trim()
      });
      if (
        !componentActive ||
        !serverScope.isCurrent() ||
        serverId !== serverScope.serverId ||
        connection.queryScope !== serverScope.connection.queryScope
      )
        return;
      createdBotId = created.bot.id;
      createVisible = false;
      apiKey = created.apiKey;
      apiKeyVisible = true;
      toast.success(m('settings.bots.created'));
      void queryClient.invalidateQueries({
        queryKey: settingsQueryKeys.botsRoot(serverId, connection)
      });
    } catch (error) {
      if (!componentActive) return;
      createError = error instanceof Error ? error.message : m('settings.bots.create_failed');
    } finally {
      if (componentActive) createLoading = false;
    }
  }

  async function copyAPIKey() {
    await navigator.clipboard.writeText(apiKey);
    toast.success(m('settings.bots.key_copied'));
  }

  function closeAPIKey() {
    const botId = createdBotId;
    apiKeyVisible = false;
    apiKey = '';
    createdBotId = null;
    if (botId) {
      void goto(
        resolve('/chat/[serverId]/manage/server/bots/[botId]', {
          serverId: serverIdToSegment(serverScope.serverId),
          botId
        })
      );
    }
  }
</script>

<PageTitle title={m('admin.common.server_admin_page_title', { title: m('settings.bots.title') })} />
<PaneHeader title={m('settings.bots.title')} subtitle={m('settings.bots.subtitle')} showMobileNav />

<PaneContent bind:scrollContainer>
  {#if !supportsBots}
    <Hint tone="warning">{m('settings.bots.unsupported')}</Hint>
  {:else}
    <div class="flex flex-col gap-6">
      {#if !canCreateBots}
        <Hint>{m('settings.bots.create_permission_required')}</Hint>
      {/if}

      <div class="max-w-md">
        <TextInput
          id="bot-search"
          label={m('settings.bots.list_title')}
          labelHidden
          leadingIcon="iconify icon-[uil--search]"
          bind:value={searchInput}
          oninput={scheduleSearch}
        />
      </div>

      {#if botsQuery.error}
        <Hint tone="danger">{botsQuery.error.message}</Hint>
      {/if}

      <Panel title={m('settings.bots.list_title')} count={totalCount} noPadding>
        {#snippet actions()}
          {#if canCreateBots}
            <Button size="sm" onclick={openCreate}>
              <span class="iconify icon-[uil--plus]" aria-hidden="true"></span>
              {m('settings.bots.create')}
            </Button>
          {/if}
        {/snippet}
        <DataTable
          items={bots}
          columns={3}
          emptyMessage={botsQuery.isPending
            ? m('settings.bots.loading')
            : m('settings.bots.empty_body')}
          hasMore={botsQuery.hasNextPage && !botsQuery.error}
          loadingMore={botsQuery.isFetchingNextPage}
          onLoadMore={loadMore}
          loadMoreRoot={scrollContainer}
          onRowClick={(bot) => goto(botHref(bot.id))}
        >
          {#snippet header()}
            <th class="table-header-cell">{m('settings.bots.singular')}</th>
            <th class="table-header-cell">{m('settings.bots.username')}</th>
            <th class="table-header-cell">{m('settings.bots.owner')}</th>
          {/snippet}
          {#snippet row(bot)}
            {@const owner = ownersById.get(bot.ownerUserId)}
            <td class="px-4 py-3">
              <UserIdentity
                user={{
                  id: bot.id,
                  login: bot.login,
                  displayName: bot.displayName,
                  avatarUrl: bot.avatarUrl,
                  deleted: false,
                  isBot: true,
                  presenceStatus: PresenceStatus.OFFLINE
                }}
              />
            </td>
            <td class="px-4 py-3">
              <a
                class="link text-muted"
                href={botHref(bot.id)}
                onclick={(event) => event.stopPropagation()}>@{bot.login}</a
              >
            </td>
            <td class="px-4 py-3">
              {#if owner}
                <UserIdentity user={{ ...owner, presenceStatus: PresenceStatus.OFFLINE }} />
              {:else if ownersQuery.isPending}
                <span
                  class="skeleton block h-8 w-32 rounded-md"
                  aria-label={m('common.loading')}
                ></span>
              {:else}
                <span class="text-muted">{m('common.unknown')}</span>
              {/if}
            </td>
          {/snippet}
        </DataTable>
      </Panel>
    </div>
  {/if}
</PaneContent>

<FormDialog
  bind:visible={createVisible}
  title={m('settings.bots.create_title')}
  submitLabel={m('settings.bots.create')}
  submitIcon="iconify icon-[uil--robot]"
  loading={createLoading}
  disabled={!normalizedCreateLogin || !!createLoginError || !createDisplayName.trim()}
  error={createError}
  onsubmit={createBot}
  onclose={() => (createVisible = false)}
>
  <TextInput
    id="bot-login"
    label={m('settings.bots.username')}
    description={normalizedCreateLogin ? undefined : m('settings.bots.username_hint')}
    error={createLoginError}
    maxlength={32}
    required
    autofocus
    bind:value={createLogin}
  />
  <TextInput
    id="bot-display-name"
    label={m('settings.bots.display_name')}
    maxlength={32}
    required
    bind:value={createDisplayName}
  />
</FormDialog>

<Dialog
  bind:visible={apiKeyVisible}
  title={m('settings.bots.api_key_title')}
  size="lg"
  onclose={closeAPIKey}
>
  <div class="flex flex-col gap-4">
    <Hint tone="warning">{m('settings.bots.api_key_warning')}</Hint>
    <div class="flex items-center gap-3 surface-box p-3">
      <code class="min-w-0 flex-1 overflow-x-auto text-sm whitespace-nowrap select-all"
        >{apiKey}</code
      >
      <Button size="sm" variant="secondary" onclick={copyAPIKey}>
        <span class="iconify icon-[uil--copy]" aria-hidden="true"></span>
        {m('common.copy_to_clipboard')}
      </Button>
    </div>
    <div class="flex justify-end">
      <Button defaultAction onclick={closeAPIKey}>{m('common.got_it')}</Button>
    </div>
  </div>
</Dialog>
