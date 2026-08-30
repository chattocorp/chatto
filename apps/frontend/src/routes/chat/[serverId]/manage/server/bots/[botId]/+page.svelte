<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { createQuery } from '@tanstack/svelte-query';
  import { createBotAPI, type Bot } from '$lib/api-client/bots';
  import { createUserAPI } from '$lib/api-client/users';
  import { viewerResponseToState } from '$lib/api-client/viewer';
  import { CopyId } from '$lib/ui';
  import Panel from '$lib/ui/Panel.svelte';
  import BotCredentialSection, {
    type BotCredentialSectionItem
  } from '$lib/components/bots/BotCredentialSection.svelte';
  import AvatarEditor from '$lib/components/users/AvatarEditor.svelte';
  import { UserPermissionsMatrix } from '$lib/components/rbac';
  import UserCombobox from '$lib/components/users/UserCombobox.svelte';
  import UserIdentity from '$lib/components/users/UserIdentity.svelte';
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import { serverIdToSegment } from '$lib/navigation';
  import { queryClient } from '$lib/query/client';
  import { adminQueryKeys } from '$lib/query/admin';
  import { settingsQueryKeys } from '$lib/query/settings';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { ConfirmDialog, FormDialog, Hint, PageTitle, PaneContent, PaneHeader } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { formatDateTime, timeFormatSettingsFor } from '$lib/utils/formatTime';
  import { onDestroy } from 'svelte';

  const serverScope = useServerScope();
  const botId = $derived(page.params.botId!);
  const supportsBots = $derived(serverScope.store.serverInfo.supportsFeature('botAccounts'));
  const supportsIncomingWebhooks = $derived(
    serverScope.store.serverInfo.supportsFeature('botIncomingWebhooks')
  );
  const supportsMultipleAPIKeys = $derived(
    serverScope.store.serverInfo.supportsFeature('botMultipleApiKeys')
  );
  const supportsOwnerReassignment = $derived(
    serverScope.store.serverInfo.supportsFeature('botOwnerReassignment')
  );
  const supportsUserAvatars = $derived(
    serverScope.store.serverInfo.supportsFeature('userAvatars')
  );
  const viewerState = $derived.by(() => {
    const viewer = serverScope.store.projection.viewer;
    return viewer ? viewerResponseToState(viewer) : null;
  });
  const canManageBots = $derived(viewerState?.viewerPermissions['bot.manage'] ?? false);
  const canManageAccounts = $derived(serverScope.store.permissions.canAdminManageAccounts);
  const canReassignOwner = $derived(canManageBots);
  const backHref = $derived(
    resolve('/chat/[serverId]/manage/server/bots', {
      serverId: serverIdToSegment(serverScope.serverId)
    })
  );

  const botQuery = createQuery(
    () => {
      const serverId = serverScope.serverId;
      const connection = serverScope.connection;
      const targetBotId = botId;
      return {
        queryKey: settingsQueryKeys.bot(serverId, connection, targetBotId),
        queryFn: ({ signal }) => connection.getAPI(createBotAPI).getBot(targetBotId, { signal }),
        enabled: supportsBots && !!targetBotId
      };
    },
    () => queryClient
  );

  const bot = $derived(botQuery.data ?? null);
  const ownerQuery = createQuery(
    () => {
      const serverId = serverScope.serverId;
      const connection = serverScope.connection;
      const ownerUserId = bot?.ownerUserId ?? '';
      return {
        queryKey: [...settingsQueryKeys.bot(serverId, connection, botId), 'owner', ownerUserId],
        queryFn: () => connection.getAPI(createUserAPI).batchGetUsers([ownerUserId]),
        enabled: supportsBots && !!ownerUserId
      };
    },
    () => queryClient
  );
  const owner = $derived(ownerQuery.data?.[0] ?? null);
  const canOperateBot = $derived(
    !!bot && (bot.ownerUserId === viewerState?.user.id || canManageBots)
  );
  const canEditAvatar = $derived(canOperateBot || canManageAccounts);
  const targetKey = $derived(
    `${serverScope.serverId}:${serverScope.connection.queryScope}:${botId}`
  );
  let componentActive = true;
  let deleteVisible = $state(false);
  let deleteLoading = $state(false);
  let reassignVisible = $state(false);
  let reassignOwnerUserId = $state('');
  let reassignOwnerText = $state('');
  let reassignLoading = $state(false);
  let reassignError = $state<string | null>(null);

  onDestroy(() => {
    componentActive = false;
  });

  const timeSettings = $derived(
    timeFormatSettingsFor(serverScope.store.currentUser.user?.settings)
  );
  const activeLocale = $derived(getLocale());

  function botAPI() {
    return serverScope.connection.getAPI(createBotAPI);
  }

  function userAPI() {
    return serverScope.connection.getAPI(createUserAPI);
  }

  function isCurrentTarget(mutationTarget: string): boolean {
    return componentActive && serverScope.isCurrent() && mutationTarget === targetKey;
  }

  function cacheBot(updated: Bot) {
    queryClient.setQueryData(
      settingsQueryKeys.bot(serverScope.serverId, serverScope.connection, updated.id),
      updated
    );
    void queryClient.invalidateQueries({
      queryKey: settingsQueryKeys.botsRoot(serverScope.serverId, serverScope.connection)
    });
  }

  function refreshBot() {
    void queryClient.invalidateQueries({
      queryKey: settingsQueryKeys.botsRoot(serverScope.serverId, serverScope.connection)
    });
  }

  async function uploadAvatar(file: File): Promise<boolean> {
    if (!bot) return false;
    const mutationTarget = targetKey;
    const updated = await userAPI().uploadAvatar(bot.id, file);
    if (!isCurrentTarget(mutationTarget) || !bot) return false;
    cacheBot({ ...bot, avatarUrl: updated.avatarUrl });
    return true;
  }

  async function deleteAvatar(): Promise<boolean> {
    if (!bot) return false;
    const mutationTarget = targetKey;
    const updated = await userAPI().deleteAvatar(bot.id);
    if (!isCurrentTarget(mutationTarget) || !bot) return false;
    cacheBot({ ...bot, avatarUrl: updated.avatarUrl });
    return true;
  }

  async function createAPIKey(name: string): Promise<string | null> {
    if (!bot) return null;
    const mutationTarget = targetKey;
    try {
      const created = await botAPI().createBotAPIKey(bot.id, name);
      if (!isCurrentTarget(mutationTarget)) return null;
      refreshBot();
      toast.success(m('settings.bots.key_created_toast'));
      return created.apiKey;
    } catch (error) {
      if (isCurrentTarget(mutationTarget)) {
        toast.error(error instanceof Error ? error.message : m('settings.bots.key_create_failed'));
      }
      return null;
    }
  }

  async function revokeAPIKey(keyId: string): Promise<boolean> {
    if (!bot) return false;
    const mutationTarget = targetKey;
    try {
      const updated = await botAPI().revokeBotAPIKey(bot.id, keyId);
      if (!isCurrentTarget(mutationTarget)) return false;
      cacheBot(updated);
      toast.success(m('settings.bots.key_revoked'));
      return true;
    } catch (error) {
      if (isCurrentTarget(mutationTarget)) {
        toast.error(error instanceof Error ? error.message : m('settings.bots.key_revoke_failed'));
      }
      return false;
    }
  }

  async function createWebhook(name: string): Promise<string | null> {
    if (!bot) return null;
    const mutationTarget = targetKey;
    try {
      const created = await botAPI().createBotIncomingWebhook(bot.id, name);
      if (!isCurrentTarget(mutationTarget)) return null;
      refreshBot();
      toast.success(m('settings.bots.webhook_created'));
      return created.webhookUrl;
    } catch (error) {
      if (isCurrentTarget(mutationTarget)) {
        toast.error(
          error instanceof Error ? error.message : m('settings.bots.webhook_create_failed')
        );
      }
      return null;
    }
  }

  async function revokeWebhook(webhookId: string): Promise<boolean> {
    if (!bot) return false;
    const mutationTarget = targetKey;
    try {
      const updated = await botAPI().revokeBotIncomingWebhook(bot.id, webhookId);
      if (!isCurrentTarget(mutationTarget)) return false;
      cacheBot(updated);
      toast.success(m('settings.bots.webhook_revoked'));
      return true;
    } catch (error) {
      if (isCurrentTarget(mutationTarget)) {
        toast.error(
          error instanceof Error ? error.message : m('settings.bots.webhook_revoke_failed')
        );
      }
      return false;
    }
  }

  function openReassignOwner() {
    reassignOwnerUserId = '';
    reassignOwnerText = '';
    reassignError = null;
    reassignVisible = true;
  }

  async function reassignOwner() {
    if (!bot || !canReassignOwner || reassignOwnerUserId === bot.ownerUserId) return;
    const mutationTarget = targetKey;
    reassignLoading = true;
    reassignError = null;
    try {
      const reassigned = await botAPI().reassignBotOwner(bot.id, reassignOwnerUserId);
      if (!isCurrentTarget(mutationTarget)) return;
      cacheBot(reassigned);
      void queryClient.invalidateQueries({
        queryKey: adminQueryKeys.userPermissions(
          serverScope.serverId,
          serverScope.connection,
          bot.id
        ),
        exact: true
      });
      reassignVisible = false;
      toast.success(m('settings.bots.owner_reassigned'));
    } catch (error) {
      if (!isCurrentTarget(mutationTarget)) return;
      reassignError =
        error instanceof Error ? error.message : m('settings.bots.owner_reassign_failed');
    } finally {
      if (isCurrentTarget(mutationTarget)) reassignLoading = false;
    }
  }

  async function deleteBot() {
    if (!bot) return;
    const mutationTarget = targetKey;
    deleteLoading = true;
    try {
      await botAPI().deleteBot(bot.id);
      if (!isCurrentTarget(mutationTarget)) return;
      queryClient.removeQueries({
        queryKey: settingsQueryKeys.bot(serverScope.serverId, serverScope.connection, bot.id),
        exact: true
      });
      void queryClient.invalidateQueries({
        queryKey: settingsQueryKeys.botsRoot(serverScope.serverId, serverScope.connection)
      });
      toast.success(m('settings.bots.deleted'));
      await goto(
        resolve('/chat/[serverId]/manage/server/bots', {
          serverId: serverIdToSegment(serverScope.serverId)
        })
      );
    } catch (error) {
      if (isCurrentTarget(mutationTarget)) {
        toast.error(error instanceof Error ? error.message : m('settings.bots.delete_failed'));
      }
    } finally {
      if (isCurrentTarget(mutationTarget)) deleteLoading = false;
    }
  }

  function formatDate(value: Date | null): string {
    return value ? formatDateTime(value, timeSettings, activeLocale) : '—';
  }

  function formatLastUsed(
    credential: Pick<Bot['apiKeys'][number], 'lastUsedState' | 'lastUsedAt'>,
    unavailable: string,
    noUseRecorded: string
  ): string {
    if (credential.lastUsedState === 'unavailable') {
      return unavailable;
    }
    if (credential.lastUsedState === 'no_use_recorded' || !credential.lastUsedAt) {
      return noUseRecorded;
    }
    return formatDate(credential.lastUsedAt);
  }

  const apiKeyItems = $derived<BotCredentialSectionItem[]>(
    (bot?.apiKeys ?? []).map((key) => ({
      id: key.id,
      name: key.name,
      createdAt: formatDate(key.createdAt),
      lastUsed: formatLastUsed(
        key,
        m('settings.bots.key_last_used_unavailable'),
        m('settings.bots.key_no_use_recorded')
      )
    }))
  );
  const webhookItems = $derived<BotCredentialSectionItem[]>(
    (bot?.incomingWebhooks ?? []).map((webhook) => ({
      id: webhook.id,
      name: webhook.name || m('settings.bots.webhook_title'),
      createdAt: formatDate(webhook.createdAt),
      lastUsed: formatLastUsed(
        webhook,
        m('settings.bots.webhook_last_used_unavailable'),
        m('settings.bots.webhook_no_use_recorded')
      )
    }))
  );
</script>

<PageTitle
  title={m('admin.common.server_admin_page_title', {
    title: bot?.displayName ?? m('settings.bots.title')
  })}
/>
<PaneHeader
  title={bot?.displayName ?? m('settings.bots.title')}
  subtitle={bot ? `@${bot.login}` : undefined}
  {backHref}
  loading={botQuery.isPending}
/>

<PaneContent>
  {#if !supportsBots}
    <Hint tone="warning">{m('settings.bots.unsupported')}</Hint>
  {:else if botQuery.error}
    <Hint tone="danger">{botQuery.error.message}</Hint>
  {:else if bot}
    <div class="flex flex-col gap-6">
      <Panel title={bot.displayName} subtitle={`@${bot.login}`}>
        {#snippet actions()}
          {#if canReassignOwner && supportsOwnerReassignment}
            <Button size="sm" variant="secondary" onclick={openReassignOwner}>
              <span class="iconify icon-[uil--exchange]" aria-hidden="true"></span>
              {m('settings.bots.reassign_owner')}
            </Button>
          {/if}
          {#if canOperateBot}
            <Button size="sm" variant="danger-secondary" onclick={() => (deleteVisible = true)}>
              <span class="iconify icon-[uil--trash]" aria-hidden="true"></span>
              {m('common.delete')}
            </Button>
          {/if}
        {/snippet}
        <dl class="grid gap-4 text-sm sm:grid-cols-2 lg:grid-cols-4">
          <div>
            <dt class="text-muted">{m('admin.members.user_id')}</dt>
            <dd class="mt-1"><CopyId value={bot.id} /></dd>
          </div>
          <div>
            <dt class="text-muted">{m('settings.bots.owner')}</dt>
            <dd class="mt-1">
              {#if owner}
                <UserIdentity
                  user={{ ...owner, presenceStatus: PresenceStatus.OFFLINE }}
                  viewerSettings={serverScope.store.currentUser.user?.settings}
                />
              {:else if ownerQuery.isPending}
                <span class="skeleton block h-8 w-32 rounded-md" aria-label={m('common.loading')}
                ></span>
              {:else}
                <span class="text-muted">{m('common.unknown')}</span>
              {/if}
            </dd>
          </div>
          {#if !supportsMultipleAPIKeys}
            <div>
              <dt class="text-muted">{m('settings.bots.key_created')}</dt>
              <dd class="mt-1">{formatDate(bot.apiKeyCreatedAt)}</dd>
            </div>
          {/if}
        </dl>
      </Panel>

      {#if supportsUserAvatars && canEditAvatar}
        {#key targetKey}
          <AvatarEditor user={{ ...bot, isBot: true }} onupload={uploadAvatar} ondelete={deleteAvatar} />
        {/key}
      {/if}

      {#if canOperateBot}
        {#key targetKey}
          {#if supportsMultipleAPIKeys}
            <BotCredentialSection
            idPrefix="bot-api-key"
            testId="bot-api-keys"
            items={apiKeyItems}
            createIcon="iconify icon-[uil--key-skeleton]"
            labels={{
              title: m('settings.bots.key_title'),
              description: m('settings.bots.key_description'),
              create: m('settings.bots.key_create'),
              name: m('settings.bots.key_name'),
              createdAt: m('settings.bots.key_created_at'),
              lastUsed: m('settings.bots.key_last_used'),
              empty: m('settings.bots.key_empty_description'),
              limitReached: m('settings.bots.key_limit_reached'),
              revoke: m('settings.bots.key_revoke'),
              revokeWarning: m('settings.bots.key_revoke_warning'),
              issuedTitle: m('settings.bots.api_key_title'),
              issuedWarning: m('settings.bots.api_key_warning'),
              copied: m('settings.bots.key_copied')
            }}
            oncreate={createAPIKey}
            onrevoke={revokeAPIKey}
            />
          {/if}

          {#if supportsIncomingWebhooks}
            <BotCredentialSection
            idPrefix="bot-webhook"
            testId="bot-incoming-webhooks"
            items={webhookItems}
            createIcon="iconify icon-[uil--link-add]"
            labels={{
              title: m('settings.bots.webhook_title'),
              description: m('settings.bots.webhook_description'),
              create: m('settings.bots.webhook_create'),
              name: m('settings.bots.webhook_name'),
              createdAt: m('settings.bots.webhook_created_at'),
              lastUsed: m('settings.bots.webhook_last_used'),
              empty: m('settings.bots.webhook_empty_description'),
              limitReached: m('settings.bots.webhook_limit_reached'),
              revoke: m('settings.bots.webhook_revoke'),
              revokeWarning: m('settings.bots.webhook_revoke_warning'),
              issuedTitle: m('settings.bots.webhook_url_title'),
              issuedWarning: m('settings.bots.webhook_url_warning'),
              copied: m('settings.bots.webhook_url_copied')
            }}
            oncreate={createWebhook}
            onrevoke={revokeWebhook}
            />
          {/if}
        {/key}

        <UserPermissionsMatrix
          userId={bot.id}
          subjectKind={m('settings.bots.singular')}
          ownerCapped
          decisionMode="binary"
        />
      {/if}
    </div>
  {/if}
</PaneContent>

<FormDialog
  bind:visible={reassignVisible}
  title={m('settings.bots.reassign_owner')}
  submitLabel={m('settings.bots.reassign_owner')}
  loading={reassignLoading}
  disabled={!reassignOwnerUserId || reassignOwnerUserId === bot?.ownerUserId}
  error={reassignError}
  onsubmit={reassignOwner}
  onclose={() => (reassignVisible = false)}
>
  <Hint tone="warning">{m('settings.bots.reassign_owner_warning')}</Hint>
  <UserCombobox
    id="reassign-bot-owner"
    label={m('settings.bots.owner')}
    placeholder={m('admin.members.search_placeholder')}
    humanOnly
    allowFreeform={false}
    bind:value={reassignOwnerUserId}
    bind:text={reassignOwnerText}
  />
</FormDialog>

<ConfirmDialog
  bind:visible={deleteVisible}
  title={m('settings.bots.delete_title')}
  actionLabel={m('common.delete')}
  loading={deleteLoading}
  onconfirm={deleteBot}
  onclose={() => (deleteVisible = false)}
>
  {m('settings.bots.delete_warning', { name: bot?.displayName ?? '' })}
</ConfirmDialog>
