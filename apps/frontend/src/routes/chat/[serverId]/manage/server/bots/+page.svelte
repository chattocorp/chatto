<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { createBotAPI } from '$lib/api-client/bots';
  import { viewerResponseToState } from '$lib/api-client/viewer';
  import { Panel } from '$lib/components/admin';
  import { UserPermissionsMatrix } from '$lib/components/rbac';
  import { m } from '$lib/i18n/messages';
  import { queryClient } from '$lib/query/client';
  import { settingsQueryKeys } from '$lib/query/settings';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import {
    ConfirmDialog,
    Dialog,
    EmptyState,
    FormDialog,
    Hint,
    PageTitle,
    PaneContent,
    PaneHeader
  } from '$lib/ui';
  import { Button, TextInput, validate, z } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';

  const serverScope = useServerScope();
  const supportsBots = $derived(serverScope.store.serverInfo.supportsFeature('botAccounts'));
  const canCreateBots = $derived.by(() => {
    const viewer = serverScope.store.projection.viewer;
    return viewer ? (viewerResponseToState(viewer).viewerPermissions['bot.create'] ?? false) : false;
  });
  const botsQuery = createQuery(
    () => {
      const serverId = serverScope.serverId;
      const connection = serverScope.connection;
      return {
        queryKey: settingsQueryKeys.bots(serverId, connection),
        queryFn: ({ signal }) => connection.getAPI(createBotAPI).listBots({ signal }),
        enabled: supportsBots
      };
    },
    () => queryClient
  );

  const bots = $derived(botsQuery.data ?? []);
  let selectedBotId = $state<string | null>(null);
  const selectedBot = $derived(bots.find((bot) => bot.id === selectedBotId) ?? bots[0] ?? null);
  let createVisible = $state(false);
  let createLogin = $state('');
  let createDisplayName = $state('');
  let createLoading = $state(false);
  let createError = $state<string | null>(null);
  let editVisible = $state(false);
  let editLogin = $state('');
  let editDisplayName = $state('');
  let editLoading = $state(false);
  let editError = $state<string | null>(null);
  let apiKeyVisible = $state(false);
  let apiKey = $state('');
  let rotateVisible = $state(false);
  let rotateLoading = $state(false);
  let deleteVisible = $state(false);
  let deleteLoading = $state(false);

  const botLoginSchema = z
    .string()
    .min(2, m('common.validation.username_min'))
    .max(32, m('common.validation.username_max'))
    .regex(/^[a-zA-Z0-9][a-zA-Z0-9._-]*$/, m('common.validation.username_charset'))
    .refine((value) => !value.endsWith('.'), m('common.validation.username_end_alphanumeric'))
    .refine(
      (value) => value.toLowerCase().endsWith('_bot'),
      m('settings.bots.username_hint')
    );
  const normalizedCreateLogin = $derived(createLogin.trim());
  const createLoginError = $derived(
    normalizedCreateLogin ? validate(botLoginSchema, normalizedCreateLogin) : undefined
  );
  const normalizedEditLogin = $derived(editLogin.trim());
  const editLoginError = $derived(
    normalizedEditLogin ? validate(botLoginSchema, normalizedEditLogin) : undefined
  );

  function botAPI() {
    return serverScope.connection.getAPI(createBotAPI);
  }

  async function refreshBots() {
    await queryClient.invalidateQueries({
      queryKey: settingsQueryKeys.bots(serverScope.serverId, serverScope.connection),
      exact: true
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
    createLoading = true;
    createError = null;
    try {
      const created = await botAPI().createBot({
        login: normalizedCreateLogin,
        displayName: createDisplayName.trim()
      });
      selectedBotId = created.bot.id;
      createVisible = false;
      apiKey = created.apiKey;
      apiKeyVisible = true;
      toast.success(m('settings.bots.created'));
      void refreshBots().catch(() => {});
    } catch (error) {
      createError = error instanceof Error ? error.message : m('settings.bots.create_failed');
    } finally {
      createLoading = false;
    }
  }

  function openEdit() {
    if (!selectedBot) return;
    editLogin = selectedBot.login;
    editDisplayName = selectedBot.displayName;
    editError = null;
    editVisible = true;
  }

  async function updateBot() {
    if (!selectedBot || !normalizedEditLogin || editLoginError) return;
    editLoading = true;
    editError = null;
    try {
      await botAPI().updateBot({
        botUserId: selectedBot.id,
        login: normalizedEditLogin,
        displayName: editDisplayName.trim()
      });
      await refreshBots();
      editVisible = false;
      toast.success(m('settings.bots.updated'));
    } catch (error) {
      editError = error instanceof Error ? error.message : m('settings.bots.update_failed');
    } finally {
      editLoading = false;
    }
  }

  async function rotateKey() {
    if (!selectedBot) return;
    rotateLoading = true;
    try {
      const rotated = await botAPI().rotateBotAPIKey(selectedBot.id);
      rotateVisible = false;
      apiKey = rotated.apiKey;
      apiKeyVisible = true;
      toast.success(m('settings.bots.key_rotated'));
      void refreshBots().catch(() => {});
    } catch (error) {
      toast.error(error instanceof Error ? error.message : m('settings.bots.rotate_failed'));
    } finally {
      rotateLoading = false;
    }
  }

  async function deleteBot() {
    if (!selectedBot) return;
    deleteLoading = true;
    try {
      await botAPI().deleteBot(selectedBot.id);
      selectedBotId = null;
      await refreshBots();
      deleteVisible = false;
      toast.success(m('settings.bots.deleted'));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : m('settings.bots.delete_failed'));
    } finally {
      deleteLoading = false;
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
    return value
      ? new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(
          value
        )
      : '—';
  }
</script>

<PageTitle
  title={m('admin.common.server_admin_page_title', { title: m('settings.bots.title') })}
/>
<PaneHeader title={m('settings.bots.title')} subtitle={m('settings.bots.subtitle')} showMobileNav />

<PaneContent>
  {#if !supportsBots}
    <Hint tone="warning">{m('settings.bots.unsupported')}</Hint>
  {:else}
    <div class="flex flex-col gap-6">
      {#if !canCreateBots}
        <Hint>{m('settings.bots.create_permission_required')}</Hint>
      {/if}
      <Panel title={m('settings.bots.list_title')} count={bots.length} noPadding>
        {#snippet actions()}
          {#if canCreateBots}
            <Button size="sm" onclick={openCreate}>
              <span class="iconify icon-[uil--plus]" aria-hidden="true"></span>
              {m('settings.bots.create')}
            </Button>
          {/if}
        {/snippet}
        {#if botsQuery.isPending}
          <div class="p-5 text-muted">{m('settings.bots.loading')}</div>
        {:else if botsQuery.error}
          <div class="p-5"><Hint tone="danger">{botsQuery.error.message}</Hint></div>
        {:else if bots.length === 0}
          <EmptyState icon="icon-[uil--robot]" title={m('settings.bots.empty_title')}>
            {m('settings.bots.empty_body')}
          </EmptyState>
        {:else}
          <div class="selectable-list" aria-label={m('settings.bots.list_title')}>
            {#each bots as bot (bot.id)}
              <button
                type="button"
                class={[
                  'flex w-full items-center gap-3 selectable-list-item px-4 py-3 text-left',
                  selectedBot?.id === bot.id ? 'bg-surface-selected' : ''
                ]}
                aria-pressed={selectedBot?.id === bot.id}
                onclick={() => (selectedBotId = bot.id)}
              >
                <span
                  class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-surface-emphasized text-neutral-action"
                  aria-hidden="true"
                >
                  <span class="iconify icon-[uil--robot] text-xl"></span>
                </span>
                <span class="min-w-0 flex-1">
                  <span class="block truncate font-medium text-text-top">{bot.displayName}</span>
                  <span class="block truncate text-sm text-muted">@{bot.login}</span>
                </span>
                <span class="iconify icon-[uil--angle-right] text-muted" aria-hidden="true"></span>
              </button>
            {/each}
          </div>
        {/if}
      </Panel>

      {#if selectedBot}
        <Panel title={selectedBot.displayName} subtitle={`@${selectedBot.login}`}>
          {#snippet actions()}
            <Button size="sm" variant="secondary" onclick={openEdit}>
              <span class="iconify icon-[uil--edit]" aria-hidden="true"></span>
              {m('settings.bots.edit')}
            </Button>
            <Button size="sm" variant="warning" onclick={() => (rotateVisible = true)}>
              <span class="iconify icon-[uil--refresh]" aria-hidden="true"></span>
              {m('settings.bots.rotate_key')}
            </Button>
            <Button size="sm" variant="danger-secondary" onclick={() => (deleteVisible = true)}>
              <span class="iconify icon-[uil--trash]" aria-hidden="true"></span>
              {m('common.delete')}
            </Button>
          {/snippet}
          <dl class="grid gap-4 text-sm sm:grid-cols-3">
            <div>
              <dt class="text-muted">{m('admin.members.user_id')}</dt>
              <dd class="mt-1 font-mono">{selectedBot.id}</dd>
            </div>
            <div>
              <dt class="text-muted">{m('settings.bots.key_created')}</dt>
              <dd class="mt-1">{formatDate(selectedBot.apiKeyCreatedAt)}</dd>
            </div>
            <div>
              <dt class="text-muted">{m('settings.bots.key_rotated_at')}</dt>
              <dd class="mt-1">{formatDate(selectedBot.apiKeyRotatedAt)}</dd>
            </div>
          </dl>
        </Panel>

        <UserPermissionsMatrix
          userId={selectedBot.id}
          subjectKind={m('settings.bots.singular')}
          ownerCapped
        />
      {/if}
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

<FormDialog
  bind:visible={editVisible}
  title={m('settings.bots.edit_title')}
  submitLabel={m('common.save')}
  loading={editLoading}
  disabled={!normalizedEditLogin || !!editLoginError || !editDisplayName.trim()}
  error={editError}
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
  {m('settings.bots.delete_warning', { name: selectedBot?.displayName ?? '' })}
</ConfirmDialog>
