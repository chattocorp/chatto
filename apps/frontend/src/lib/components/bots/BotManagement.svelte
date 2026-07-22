<!--
@component

Shared bot collection and identity editor used by owner settings and Server
Admin. The caller supplies scope-specific headings and creation authority;
all loading, pagination, owner hydration, and mutations stay in this component.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { createBotAPI, type BotAccount } from '$lib/api-client/bots';
  import { createUserAPI } from '$lib/api-client/users';
  import { Panel, DataTable } from '$lib/components/admin';
  import { Button, TextArea, TextInput } from '$lib/ui/form';
  import { Dialog, EmptyState, FormDialog, Hint, Pill } from '$lib/ui';
  import { toast } from '$lib/ui/toast';
  import { BotManagementStore } from './BotManagementStore.svelte';
  import * as m from '$lib/i18n/messages';
  import BotPermissionsMatrix from './BotPermissionsMatrix.svelte';
  import BotCredentialsDialog from './BotCredentialsDialog.svelte';

  let {
    scope,
    canCreate = false,
    scrollContainer
  }: {
    scope: 'owner' | 'admin';
    canCreate?: boolean;
    scrollContainer?: HTMLDivElement;
  } = $props();

  const connection = useConnection();
  const store = new BotManagementStore(
    () => (scope === 'owner' ? 'owned' : 'manageable'),
    () => {
      const conn = connection();
      return createBotAPI({ baseUrl: conn.connectBaseUrl, bearerToken: conn.bearerToken });
    },
    () => {
      const conn = connection();
      return createUserAPI({ baseUrl: conn.connectBaseUrl, bearerToken: conn.bearerToken });
    }
  );

  let dialogVisible = $state(false);
  let editingBot = $state<BotAccount | null>(null);
  let login = $state('');
  let displayName = $state('');
  let botDescription = $state('');
  let saving = $state(false);
  let saveError = $state<string | null>(null);
  let permissionsBot = $state<BotAccount | null>(null);
  let credentialsBot = $state<BotAccount | null>(null);

  const normalizedLogin = $derived(login.trim());
  const normalizedDisplayName = $derived(displayName.trim());
  const normalizedDescription = $derived(botDescription.trim());
  const loginValid = $derived(normalizedLogin.toLowerCase().endsWith('_bot'));
  const dirty = $derived(
    !editingBot ||
      normalizedLogin !== editingBot.login ||
      normalizedDisplayName !== editingBot.displayName ||
      normalizedDescription !== editingBot.description
  );
  const formValid = $derived(
    loginValid && normalizedDisplayName.length > 0 && normalizedDescription.length > 0 && dirty
  );

  onMount(() => void store.load());

  function openCreate() {
    editingBot = null;
    login = '';
    displayName = '';
    botDescription = '';
    saveError = null;
    dialogVisible = true;
  }

  function openEdit(bot: BotAccount) {
    editingBot = bot;
    login = bot.login;
    displayName = bot.displayName;
    botDescription = bot.description;
    saveError = null;
    dialogVisible = true;
  }

  function closeDialog() {
    if (saving) return;
    dialogVisible = false;
    saveError = null;
  }

  function openPermissions(event: MouseEvent, bot: BotAccount) {
    event.stopPropagation();
    permissionsBot = bot;
  }

  function openCredentials(event: MouseEvent, bot: BotAccount) {
    event.stopPropagation();
    credentialsBot = bot;
  }

  function updateCredentialsBot(bot: BotAccount) {
    store.replace(bot);
    credentialsBot = bot;
  }

  async function saveBot() {
    if (!formValid) return;
    saving = true;
    saveError = null;
    try {
      if (editingBot) {
        await store.update({
          botId: editingBot.id,
          login: normalizedLogin === editingBot.login ? undefined : normalizedLogin,
          displayName:
            normalizedDisplayName === editingBot.displayName ? undefined : normalizedDisplayName,
          description:
            normalizedDescription === editingBot.description ? undefined : normalizedDescription
        });
        toast.success(m['bots.toast.updated']());
      } else {
        await store.create({
          login: normalizedLogin,
          displayName: normalizedDisplayName,
          description: normalizedDescription
        });
        toast.success(m['bots.toast.created']());
      }
      dialogVisible = false;
    } catch (error) {
      saveError = error instanceof Error ? error.message : m['bots.error.save_failed']();
    } finally {
      saving = false;
    }
  }

  function ownerLabel(bot: BotAccount): string {
    const owner = store.owner(bot);
    return owner ? `${owner.displayName} (@${owner.login})` : bot.ownerId;
  }
</script>

<div class="flex min-h-0 flex-1 flex-col">
  {#if store.error}
    <Hint tone="danger">{store.error}</Hint>
  {/if}

  {#if store.loading && store.bots.length === 0}
    <div class="text-muted">{m['bots.loading']()}</div>
  {:else if store.bots.length === 0}
    <Panel>
      <EmptyState icon="uil--robot" title={m['bots.empty.title']()}>
        <div class="flex flex-col items-center gap-4">
          <p>{scope === 'owner' ? m['bots.empty.owner']() : m['bots.empty.admin']()}</p>
          {#if canCreate}
            <Button onclick={openCreate}>
              <span class="iconify uil--plus" aria-hidden="true"></span>
              {m['bots.action.create']()}
            </Button>
          {/if}
        </div>
      </EmptyState>
    </Panel>
  {:else}
    <Panel title={m['bots.list.title']()} noPadding>
      {#snippet actions()}
        {#if canCreate}
          <Button onclick={openCreate}>
            <span class="iconify uil--plus" aria-hidden="true"></span>
            {m['bots.action.create']()}
          </Button>
        {/if}
      {/snippet}
      <DataTable
        items={store.bots}
        columns={scope === 'admin' ? 5 : 4}
        getKey={(bot) => bot.id}
        hasMore={store.hasMore && !store.error}
        loadingMore={store.loadingMore}
        onLoadMore={() => store.loadMore()}
        loadMoreRoot={scrollContainer}
        loadingMoreMessage={m['bots.loading_more']()}
        onRowClick={openEdit}
      >
        {#snippet header()}
          <th class="table-header-cell">{m['bots.field.bot']()}</th>
          {#if scope === 'admin'}
            <th class="table-header-cell">{m['bots.field.owner']()}</th>
          {/if}
          <th class="table-header-cell">{m['bots.field.description']()}</th>
          <th class="table-header-cell">{m['bots.field.api_key']()}</th>
          <th class="table-header-cell">{m['admin.permissions.title']()}</th>
        {/snippet}
        {#snippet row(bot)}
          <td class="px-4 py-3">
            <div class="flex min-w-0 items-center gap-3">
              <div
                class="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-surface-emphasized text-neutral-action"
                aria-hidden="true"
              >
                <span class="iconify text-xl uil--robot"></span>
              </div>
              <div class="min-w-0">
                <div class="flex items-center gap-2">
                  <span class="truncate font-medium text-text-top">{bot.displayName}</span>
                  <Pill tone="neutral">{m['bots.badge.bot']()}</Pill>
                </div>
                <p class="truncate text-sm text-muted">@{bot.login}</p>
              </div>
            </div>
          </td>
          {#if scope === 'admin'}
            <td class="px-4 py-3 text-muted">{ownerLabel(bot)}</td>
          {/if}
          <td class="max-w-md px-4 py-3 text-muted">
            <p class="line-clamp-2">{bot.description}</p>
          </td>
          <td class="px-4 py-3">
            <Button
              variant="ghost"
              size="sm"
              disabled={scope === 'admin' && !bot.apiKeyCreatedAt}
              onclick={(event) => openCredentials(event, bot)}
            >
              <Pill tone={bot.apiKeyCreatedAt ? 'success' : 'neutral'}>
                {bot.apiKeyCreatedAt ? m['bots.api_key.active']() : m['bots.api_key.none']()}
              </Pill>
            </Button>
          </td>
          <td class="px-4 py-3">
            <Button variant="secondary" onclick={(event) => openPermissions(event, bot)}>
              <span class="iconify uil--shield-check" aria-hidden="true"></span>
              {m['admin.permissions.title']()}
            </Button>
          </td>
        {/snippet}
      </DataTable>
    </Panel>
  {/if}
</div>

<FormDialog
  bind:visible={dialogVisible}
  title={editingBot ? m['bots.dialog.edit_title']() : m['bots.dialog.create_title']()}
  submitLabel={editingBot ? m['bots.action.save']() : m['bots.action.create']()}
  submitLoadingText={editingBot ? m['bots.action.saving']() : m['bots.action.creating']()}
  submitIcon={editingBot ? 'iconify uil--check' : 'iconify uil--plus'}
  loading={saving}
  disabled={!formValid}
  error={saveError}
  onsubmit={saveBot}
  onclose={closeDialog}
>
  {#snippet description()}
    {m['bots.dialog.description']()}
  {/snippet}
  <TextInput
    id="bot-display-name"
    label={m['bots.field.display_name']()}
    placeholder={m['bots.placeholder.display_name']()}
    maxlength={100}
    required
    bind:value={displayName}
  />
  <TextInput
    id="bot-username"
    label={m['bots.field.username']()}
    placeholder={m['bots.placeholder.username']()}
    description={m['bots.help.username']()}
    error={login.length > 0 && !loginValid ? m['bots.error.username_suffix']() : undefined}
    maxlength={64}
    required
    bind:value={login}
  />
  <TextArea
    id="bot-description"
    label={m['bots.field.description']()}
    placeholder={m['bots.placeholder.description']()}
    description={m['bots.help.description']()}
    maxBytes={2000}
    rows={5}
    required
    bind:value={botDescription}
  />
</FormDialog>

<Dialog
  visible={permissionsBot !== null}
  title={permissionsBot
    ? `${permissionsBot.displayName} — ${m['admin.permissions.title']()}`
    : m['admin.permissions.title']()}
  size="lg"
  onclose={() => (permissionsBot = null)}
>
  {#if permissionsBot}
    <BotPermissionsMatrix botId={permissionsBot.id} />
  {/if}
</Dialog>

{#if credentialsBot}
  <BotCredentialsDialog
    bot={credentialsBot}
    canRotate={scope === 'owner'}
    onupdated={updateCredentialsBot}
    onclose={() => (credentialsBot = null)}
  />
{/if}
