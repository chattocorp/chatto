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
  import {
    ConfirmDialog,
    Dialog,
    FormDialog,
    Hint,
    PageTitle,
    PaneContent,
    PaneHeader
  } from '$lib/ui';
  import { Button, TextInput, validate, z } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { formatDateTime, timeFormatSettingsFor } from '$lib/utils/formatTime';
  import { onDestroy } from 'svelte';

  const serverScope = useServerScope();
  const botId = $derived(page.params.botId!);
  const supportsBots = $derived(serverScope.store.serverInfo.supportsFeature('botAccounts'));
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

  async function copyAPIKey() {
    await navigator.clipboard.writeText(apiKey);
    toast.success(m('settings.bots.key_copied'));
  }

  function closeAPIKey() {
    apiKeyVisible = false;
    apiKey = '';
  }

  function formatDate(value: Date | null): string {
    return value ? formatDateTime(value, timeSettings, activeLocale) : '—';
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
      <Button onclick={closeAPIKey}>{m('common.got_it')}</Button>
    </div>
  </div>
</Dialog>

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
  bind:visible={deleteVisible}
  title={m('settings.bots.delete_title')}
  actionLabel={m('common.delete')}
  loading={deleteLoading}
  onconfirm={deleteBot}
  onclose={() => (deleteVisible = false)}
>
  {m('settings.bots.delete_warning', { name: bot?.displayName ?? '' })}
</ConfirmDialog>
