<!--
@component

Shared frame of the bot detail pages. It loads the bot, renders the header and
the section tabs, and provides the bot to the Overview, Integrations, and
Permissions pages through `botDetailContext`.
-->
<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { onDestroy, type Snippet } from 'svelte';
  import { createBotAPI, type Bot } from '@chatto/client/api/bots';
  import AccountName from '$lib/components/users/AccountName.svelte';
  import { m } from '$lib/i18n/messages';
  import { serverIdToSegment } from '$lib/navigation';
  import { createQuery, queryClient } from '$lib/query/client';
  import { settingsQueryKeys } from '$lib/query/settings';
  import { formatAccountName } from '@chatto/client/timeline/accountName';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { Hint, PageTitle, PaneContent, PaneHeader, TabNav, type TabNavItem } from '$lib/ui';
  import { errorMessage } from '$lib/utils/errorMessage';
  import { provideBotDetail } from './botDetailContext';

  let { children }: { children: Snippet } = $props();

  const serverScope = useServerScope();
  const botId = $derived(page.params.botId!);
  const canManageBots = $derived(serverScope.store.permissions.canManageBots);
  const canManageAccounts = $derived(serverScope.store.permissions.canAdminManageAccounts);
  const viewerId = $derived(serverScope.store.accountId);
  const serverSegment = serverIdToSegment(serverScope.serverId);
  const backHref = resolve('/chat/[serverId]/manage/server/bots', { serverId: serverSegment });

  const botQuery = createQuery(() => {
    const serverId = serverScope.serverId;
    const connection = serverScope.connection;
    const targetBotId = botId;
    return {
      queryKey: settingsQueryKeys.bot(serverId, connection, targetBotId),
      queryFn: ({ signal }) => connection.getAPI(createBotAPI).getBot(targetBotId, { signal }),
      enabled: !!targetBotId
    };
  });

  const bot = $derived(botQuery.data ?? null);
  const canOperateBot = $derived(!!bot && (bot.ownerUserId === viewerId || canManageBots));
  const canEditIdentity = $derived(canOperateBot || canManageAccounts);
  let layoutActive = true;

  onDestroy(() => {
    layoutActive = false;
  });

  function invalidateBots() {
    void queryClient.invalidateQueries({
      queryKey: settingsQueryKeys.botsRoot(serverScope.serverId, serverScope.connection)
    });
  }

  provideBotDetail({
    get botId() {
      return botId;
    },
    get bot() {
      return bot;
    },
    get isPending() {
      return botQuery.isPending;
    },
    get canOperateBot() {
      return canOperateBot;
    },
    get canEditIdentity() {
      return canEditIdentity;
    },
    isCurrentTarget(mutationTarget) {
      return layoutActive && serverScope.isCurrent() && mutationTarget === botId;
    },
    cacheBot(updated: Bot) {
      queryClient.setQueryData(
        settingsQueryKeys.bot(serverScope.serverId, serverScope.connection, updated.id),
        updated
      );
      invalidateBots();
    },
    refreshBot: invalidateBots
  });

  const sectionTabs = $derived.by<TabNavItem[]>(() => {
    if (!bot) return [];
    const params = { serverId: serverSegment, botId };
    const routeId = page.route.id;
    const items: TabNavItem[] = [
      {
        href: resolve('/chat/[serverId]/manage/server/bots/[botId]', params),
        label: m('settings.bots.tabs.overview'),
        icon: 'icon-[uil--robot]',
        current: routeId === '/chat/[serverId]/manage/server/bots/[botId]'
      }
    ];
    if (canOperateBot) {
      items.push(
        {
          href: resolve('/chat/[serverId]/manage/server/bots/[botId]/integrations', params),
          label: m('settings.bots.tabs.integrations'),
          icon: 'icon-[uil--plug]',
          current: routeId === '/chat/[serverId]/manage/server/bots/[botId]/integrations'
        },
        {
          href: resolve('/chat/[serverId]/manage/server/bots/[botId]/permissions', params),
          label: m('settings.bots.tabs.permissions'),
          icon: 'icon-[uil--shield-check]',
          current: routeId === '/chat/[serverId]/manage/server/bots/[botId]/permissions'
        }
      );
    }
    return items;
  });
</script>

{#snippet botName()}<AccountName
    name={bot?.displayName ?? m('settings.bots.title')}
    identity={bot ? { isBot: true } : undefined}
  />{/snippet}

<PageTitle
  title={m('admin.common.server_admin_page_title', {
    title: bot ? formatAccountName(bot.displayName, { isBot: true }) : m('settings.bots.title')
  })}
/>

<div class="pane-page">
  <PaneHeader
    title={botQuery.isPending ? '' : (bot?.displayName ?? m('settings.bots.title'))}
    titleContent={bot ? botName : undefined}
    subtitle={bot ? `@${bot.login}` : undefined}
    {backHref}
  >
    {#snippet tabs()}
      <TabNav label={m('settings.bots.tabs.label')} items={sectionTabs} />
    {/snippet}
  </PaneHeader>

  <PaneContent>
    {#if botQuery.error}
      <Hint tone="danger">{errorMessage(botQuery.error)}</Hint>
    {:else}
      <div class="flex flex-col gap-6">
        {@render children()}
      </div>
    {/if}
  </PaneContent>
</div>
