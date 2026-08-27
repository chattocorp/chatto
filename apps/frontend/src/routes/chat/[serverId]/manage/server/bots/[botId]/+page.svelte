<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { createQuery } from '@tanstack/svelte-query';
  import { Code, ConnectError } from '@connectrpc/connect';
  import { createBotAPI, type Bot } from '$lib/api-client/bots';
  import { createUserAPI } from '$lib/api-client/users';
  import { viewerResponseToState } from '$lib/api-client/viewer';
  import { CopyId, Panel } from '$lib/components/admin';
  import ShowOnceCredentialDialog from '$lib/components/bots/ShowOnceCredentialDialog.svelte';
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
  import { Button, TextInput, validate, z } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { formatDateTime, timeFormatSettingsFor } from '$lib/utils/formatTime';
  import { onDestroy } from 'svelte';

  const serverScope = useServerScope();
  const botId = $derived(page.params.botId!);
  const supportsBots = $derived(serverScope.store.serverInfo.supportsFeature('botAccounts'));
  const supportsIncomingWebhooks = $derived(
    serverScope.store.serverInfo.supportsFeature('botIncomingWebhooks')
  );
  const supportsOwnerReassignment = $derived(
    serverScope.store.serverInfo.supportsFeature('botOwnerReassignment')
  );
  const canReassignOwner = $derived.by(() => {
    const viewer = serverScope.store.projection.viewer;
    return viewer
      ? (viewerResponseToState(viewer).viewerPermissions['bot.manage'] ?? false)
      : false;
  });
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
  const targetKey = $derived(
    `${serverScope.serverId}:${serverScope.connection.queryScope}:${botId}`
  );
  let componentActive = true;
  let editVisible = $state(false);
  let editLogin = $state('');
  let editDisplayName = $state('');
  let initialEditLogin = $state('');
  let initialEditDisplayName = $state('');
  let editLoading = $state(false);
  let editError = $state<{ targetKey: string; message: string } | null>(null);
  let apiKeyVisible = $state(false);
  let apiKey = $state('');
  let rotateVisible = $state(false);
  let rotateLoading = $state(false);
  let webhookURLVisible = $state(false);
  let webhookURL = $state('');
  let createWebhookVisible = $state(false);
  let createWebhookName = $state('');
  let createWebhookLoading = $state(false);
  let rotateWebhookVisible = $state(false);
  let rotateWebhookId = $state('');
  let rotateWebhookLoading = $state(false);
  let revokeWebhookVisible = $state(false);
  let revokeWebhookId = $state('');
  let revokeWebhookLoading = $state(false);
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

  const botLoginSchema = z
    .string()
    .min(2, m('common.validation.username_min'))
    .max(32, m('common.validation.username_max'))
    .regex(/^[a-zA-Z0-9][a-zA-Z0-9._-]*$/, m('common.validation.username_charset'))
    .refine((value) => !value.endsWith('.'), m('common.validation.username_end_alphanumeric'))
    .refine((value) => value.toLowerCase().endsWith('_bot'), m('settings.bots.username_hint'));
  const normalizedEditLogin = $derived(editLogin.trim());
  const normalizedEditDisplayName = $derived(editDisplayName.trim());
  const editDirty = $derived(
    normalizedEditLogin !== initialEditLogin || normalizedEditDisplayName !== initialEditDisplayName
  );
  const editLoginError = $derived(
    normalizedEditLogin ? validate(botLoginSchema, normalizedEditLogin) : undefined
  );
  const visibleEditError = $derived(editError?.targetKey === targetKey ? editError.message : null);
  const normalizedWebhookName = $derived(createWebhookName.trim());
  const timeSettings = $derived(
    timeFormatSettingsFor(serverScope.store.currentUser.user?.settings)
  );
  const activeLocale = $derived(getLocale());

  function botAPI() {
    return serverScope.connection.getAPI(createBotAPI);
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

  function openEdit() {
    if (!bot) return;
    editLogin = bot.login;
    editDisplayName = bot.displayName;
    initialEditLogin = bot.login;
    initialEditDisplayName = bot.displayName;
    editError = null;
    editVisible = true;
  }

  async function updateBot() {
    if (!bot || !normalizedEditLogin || editLoginError || !editDirty) return;
    const mutationTarget = targetKey;
    editLoading = true;
    editError = null;
    try {
      const updated = await botAPI().updateBot({
        botUserId: bot.id,
        ...(normalizedEditLogin !== initialEditLogin ? { login: normalizedEditLogin } : {}),
        ...(normalizedEditDisplayName !== initialEditDisplayName
          ? { displayName: normalizedEditDisplayName }
          : {})
      });
      if (!isCurrentTarget(mutationTarget)) return;
      cacheBot(updated);
      editVisible = false;
      toast.success(m('settings.bots.updated'));
    } catch (error) {
      if (!isCurrentTarget(mutationTarget)) return;
      const conflict = error instanceof ConnectError && error.code === Code.Aborted;
      const message = conflict
        ? m('settings.bots.update_conflict')
        : error instanceof Error
          ? error.message
          : m('settings.bots.update_failed');
      editError = {
        targetKey: mutationTarget,
        message
      };
      if (conflict) toast.error(message);
    } finally {
      if (isCurrentTarget(mutationTarget)) editLoading = false;
    }
  }

  async function rotateKey() {
    if (!bot) return;
    const mutationTarget = targetKey;
    rotateLoading = true;
    try {
      const rotated = await botAPI().rotateBotAPIKey(bot.id);
      if (!isCurrentTarget(mutationTarget)) return;
      cacheBot(rotated.bot);
      rotateVisible = false;
      apiKey = rotated.apiKey;
      apiKeyVisible = true;
      toast.success(m('settings.bots.key_rotated'));
    } catch (error) {
      if (isCurrentTarget(mutationTarget)) {
        toast.error(error instanceof Error ? error.message : m('settings.bots.rotate_failed'));
      }
    } finally {
      if (isCurrentTarget(mutationTarget)) rotateLoading = false;
    }
  }

  function openCreateWebhook() {
    createWebhookName = '';
    createWebhookVisible = true;
  }

  async function createWebhook() {
    if (!bot || !normalizedWebhookName) return;
    const mutationTarget = targetKey;
    createWebhookLoading = true;
    try {
      const created = await botAPI().createBotIncomingWebhook(bot.id, normalizedWebhookName);
      if (!isCurrentTarget(mutationTarget)) return;
      cacheBot(created.bot);
      createWebhookVisible = false;
      webhookURL = created.webhookUrl;
      webhookURLVisible = true;
      toast.success(m('settings.bots.webhook_created'));
    } catch (error) {
      if (isCurrentTarget(mutationTarget)) {
        toast.error(
          error instanceof Error ? error.message : m('settings.bots.webhook_create_failed')
        );
      }
    } finally {
      if (isCurrentTarget(mutationTarget)) createWebhookLoading = false;
    }
  }

  function openRotateWebhook(webhookId: string) {
    rotateWebhookId = webhookId;
    rotateWebhookVisible = true;
  }

  async function rotateWebhook() {
    if (!bot || !rotateWebhookId) return;
    const mutationTarget = targetKey;
    rotateWebhookLoading = true;
    try {
      const rotated = await botAPI().rotateBotIncomingWebhook(bot.id, rotateWebhookId);
      if (!isCurrentTarget(mutationTarget)) return;
      cacheBot(rotated.bot);
      rotateWebhookVisible = false;
      webhookURL = rotated.webhookUrl;
      webhookURLVisible = true;
      toast.success(m('settings.bots.webhook_rotated'));
    } catch (error) {
      if (isCurrentTarget(mutationTarget)) {
        toast.error(
          error instanceof Error ? error.message : m('settings.bots.webhook_rotate_failed')
        );
      }
    } finally {
      if (isCurrentTarget(mutationTarget)) rotateWebhookLoading = false;
    }
  }

  function openRevokeWebhook(webhookId: string) {
    revokeWebhookId = webhookId;
    revokeWebhookVisible = true;
  }

  async function revokeWebhook() {
    if (!bot || !revokeWebhookId) return;
    const mutationTarget = targetKey;
    revokeWebhookLoading = true;
    try {
      const updated = await botAPI().revokeBotIncomingWebhook(bot.id, revokeWebhookId);
      if (!isCurrentTarget(mutationTarget)) return;
      cacheBot(updated);
      revokeWebhookVisible = false;
      toast.success(m('settings.bots.webhook_revoked'));
    } catch (error) {
      if (isCurrentTarget(mutationTarget)) {
        toast.error(
          error instanceof Error ? error.message : m('settings.bots.webhook_revoke_failed')
        );
      }
    } finally {
      if (isCurrentTarget(mutationTarget)) revokeWebhookLoading = false;
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

  function formatWebhookLastUsed(webhook: Bot['incomingWebhooks'][number]): string {
    if (webhook.lastUsedState === 'unavailable') {
      return m('settings.bots.webhook_last_used_unavailable');
    }
    if (webhook.lastUsedState === 'never' || !webhook.lastUsedAt) {
      return m('settings.bots.webhook_never_used');
    }
    return formatDate(webhook.lastUsedAt);
  }
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
          <Button size="sm" variant="secondary" onclick={openEdit}>
            <span class="iconify icon-[uil--edit]" aria-hidden="true"></span>
            {m('settings.bots.edit')}
          </Button>
          <Button size="sm" variant="warning" onclick={() => (rotateVisible = true)}>
            <span class="iconify icon-[uil--refresh]" aria-hidden="true"></span>
            {m('settings.bots.rotate_key')}
          </Button>
          {#if canReassignOwner && supportsOwnerReassignment}
            <Button size="sm" variant="secondary" onclick={openReassignOwner}>
              <span class="iconify icon-[uil--exchange]" aria-hidden="true"></span>
              {m('settings.bots.reassign_owner')}
            </Button>
          {/if}
          <Button size="sm" variant="danger-secondary" onclick={() => (deleteVisible = true)}>
            <span class="iconify icon-[uil--trash]" aria-hidden="true"></span>
            {m('common.delete')}
          </Button>
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
                <UserIdentity user={{ ...owner, presenceStatus: PresenceStatus.OFFLINE }} />
              {:else if ownerQuery.isPending}
                <span class="skeleton block h-8 w-32 rounded-md" aria-label={m('common.loading')}
                ></span>
              {:else}
                <span class="text-muted">{m('common.unknown')}</span>
              {/if}
            </dd>
          </div>
          <div>
            <dt class="text-muted">{m('settings.bots.key_created')}</dt>
            <dd class="mt-1">{formatDate(bot.apiKeyCreatedAt)}</dd>
          </div>
          <div>
            <dt class="text-muted">{m('settings.bots.key_rotated_at')}</dt>
            <dd class="mt-1">{formatDate(bot.apiKeyRotatedAt)}</dd>
          </div>
        </dl>
      </Panel>

      {#if supportsIncomingWebhooks}
        <Panel
          title={m('settings.bots.webhook_title')}
          subtitle={m('settings.bots.webhook_description')}
          noPadding
        >
          {#snippet actions()}
            <Button
              size="sm"
              disabled={bot.incomingWebhooks.length >= 20}
              onclick={openCreateWebhook}
            >
              <span class="iconify icon-[uil--link-add]" aria-hidden="true"></span>
              {m('settings.bots.webhook_create')}
            </Button>
          {/snippet}
          {#if bot.incomingWebhooks.length > 0}
            <div class="selectable-list" data-testid="bot-incoming-webhooks">
              {#each bot.incomingWebhooks as webhook (webhook.id)}
                <div
                  class="flex flex-col gap-4 selectable-list-item px-5 py-4 sm:flex-row sm:items-center"
                >
                  <div class="min-w-0 flex-1">
                    <div class="font-medium text-text-top">
                      <bdi>{webhook.name || m('settings.bots.webhook_title')}</bdi>
                    </div>
                    <dl class="mt-2 grid gap-x-6 gap-y-2 text-sm sm:grid-cols-3">
                      <div>
                        <dt class="text-muted">{m('settings.bots.webhook_created_at')}</dt>
                        <dd>{formatDate(webhook.createdAt)}</dd>
                      </div>
                      <div>
                        <dt class="text-muted">{m('settings.bots.key_rotated_at')}</dt>
                        <dd>{formatDate(webhook.rotatedAt)}</dd>
                      </div>
                      <div>
                        <dt class="text-muted">{m('settings.bots.webhook_last_used')}</dt>
                        <dd>{formatWebhookLastUsed(webhook)}</dd>
                      </div>
                    </dl>
                  </div>
                  <div class="flex shrink-0 justify-end gap-2">
                    <Button
                      size="sm"
                      variant="warning"
                      onclick={() => openRotateWebhook(webhook.id)}
                    >
                      <span class="iconify icon-[uil--refresh]" aria-hidden="true"></span>
                      {m('settings.bots.webhook_rotate')}
                    </Button>
                    <Button
                      size="sm"
                      variant="danger-secondary"
                      onclick={() => openRevokeWebhook(webhook.id)}
                    >
                      <span class="iconify icon-[uil--times-circle]" aria-hidden="true"></span>
                      {m('settings.bots.webhook_revoke')}
                    </Button>
                  </div>
                </div>
              {/each}
            </div>
          {:else}
            <div class="p-5 text-muted">{m('settings.bots.webhook_empty_description')}</div>
          {/if}
          {#if bot.incomingWebhooks.length >= 20}
            <div class="border-t border-border px-5 py-3 text-muted">
              {m('settings.bots.webhook_limit_reached')}
            </div>
          {/if}
        </Panel>
      {/if}

      <UserPermissionsMatrix
        userId={bot.id}
        subjectKind={m('settings.bots.singular')}
        ownerCapped
        decisionMode="binary"
      />
    </div>
  {/if}
</PaneContent>

<FormDialog
  bind:visible={editVisible}
  title={m('settings.bots.edit_title')}
  submitLabel={m('common.save')}
  loading={editLoading}
  disabled={!normalizedEditLogin || !!editLoginError || !normalizedEditDisplayName || !editDirty}
  error={visibleEditError}
  onsubmit={updateBot}
  onclose={() => (editVisible = false)}
>
  <TextInput
    id="edit-bot-login"
    label={m('settings.bots.username')}
    error={editLoginError}
    maxlength={32}
    required
    bind:value={editLogin}
  />
  <TextInput
    id="edit-bot-display-name"
    label={m('settings.bots.display_name')}
    maxlength={32}
    required
    bind:value={editDisplayName}
  />
</FormDialog>

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

<FormDialog
  bind:visible={createWebhookVisible}
  title={m('settings.bots.webhook_create')}
  submitLabel={m('settings.bots.webhook_create')}
  loading={createWebhookLoading}
  disabled={!normalizedWebhookName}
  onsubmit={createWebhook}
  onclose={() => (createWebhookVisible = false)}
>
  <TextInput
    id="create-bot-webhook-name"
    label={m('settings.bots.webhook_name')}
    maxlength={64}
    required
    bind:value={createWebhookName}
  />
</FormDialog>

<ShowOnceCredentialDialog
  bind:visible={apiKeyVisible}
  bind:value={apiKey}
  title={m('settings.bots.api_key_title')}
  warning={m('settings.bots.api_key_warning')}
  copiedMessage={m('settings.bots.key_copied')}
/>

<ShowOnceCredentialDialog
  bind:visible={webhookURLVisible}
  bind:value={webhookURL}
  title={m('settings.bots.webhook_url_title')}
  warning={m('settings.bots.webhook_url_warning')}
  copiedMessage={m('settings.bots.webhook_url_copied')}
/>

<ConfirmDialog
  bind:visible={rotateVisible}
  title={m('settings.bots.rotate_title')}
  tone="warning"
  actionLabel={m('settings.bots.rotate_key')}
  actionIcon="iconify icon-[uil--refresh]"
  loading={rotateLoading}
  onconfirm={rotateKey}
  onclose={() => (rotateVisible = false)}
>
  {m('settings.bots.rotate_warning')}
</ConfirmDialog>

<ConfirmDialog
  bind:visible={rotateWebhookVisible}
  title={m('settings.bots.webhook_rotate_title')}
  tone="warning"
  actionLabel={m('settings.bots.webhook_rotate')}
  actionIcon="iconify icon-[uil--refresh]"
  loading={rotateWebhookLoading}
  onconfirm={rotateWebhook}
  onclose={() => (rotateWebhookVisible = false)}
>
  {m('settings.bots.webhook_rotate_warning')}
</ConfirmDialog>

<ConfirmDialog
  bind:visible={revokeWebhookVisible}
  title={m('settings.bots.webhook_revoke')}
  actionLabel={m('settings.bots.webhook_revoke')}
  loading={revokeWebhookLoading}
  onconfirm={revokeWebhook}
  onclose={() => (revokeWebhookVisible = false)}
>
  {m('settings.bots.webhook_revoke_warning')}
</ConfirmDialog>

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
