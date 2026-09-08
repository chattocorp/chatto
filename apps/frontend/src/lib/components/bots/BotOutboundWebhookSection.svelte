<script lang="ts">
  import {
    BotWebhookDeliveryStatus,
    type BotOutboundWebhook
  } from '@chatto/api-types/api/v1/bots_pb';
  import { createQuery } from '@tanstack/svelte-query';
  import { createBotAPI } from '$lib/api-client/bots';
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import { formatDateTime, timeFormatSettingsFor } from '$lib/utils/formatTime';
  import { queryClient } from '$lib/query/client';
  import { settingsQueryKeys } from '$lib/query/settings';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { ConfirmDialog, Dialog, FormDialog, Hint } from '$lib/ui';
  import { toast } from '$lib/ui/toast';
  import { Button, TextInput } from '$lib/ui/form';
  import BotIntegrationSection from './BotIntegrationSection.svelte';
  import BotIntegrationDetails from './BotIntegrationDetails.svelte';
  import BotWebhookFailureDetails from './BotWebhookFailureDetails.svelte';
  import BotWebhookFailureHistory from './BotWebhookFailureHistory.svelte';
  import ShowOnceCredentialDialog from './ShowOnceCredentialDialog.svelte';

  let { botId }: { botId: string } = $props();
  const scope = useServerScope();
  const query = createQuery(
    () => ({
      queryKey: [...settingsQueryKeys.bot(scope.serverId, scope.connection, botId), 'outbound'],
      queryFn: ({ signal }) =>
        scope.connection.getAPI(createBotAPI).listOutboundWebhooks(botId, signal),
      refetchInterval: 5000
    }),
    () => queryClient
  );
  const webhooks = $derived(query.data ?? []);
  const atLimit = $derived(webhooks.length >= 20);
  const timeSettings = $derived(timeFormatSettingsFor(scope.store.currentUser.user?.settings));
  const activeLocale = $derived(getLocale());

  let historyId = $state('');
  let historyVisible = $state(false);

  let createVisible = $state(false);
  let name = $state('');
  let url = $state('');
  let authorization = $state('');
  let pending = $state(false);
  let createError = $state(false);
  let editId = $state('');
  let editVisible = $state(false);
  let editURL = $state('');
  let editOriginalURL = $state('');
  let editAuthorization = $state('');
  let editHasAuthorization = $state(false);
  let removeAuthorization = $state(false);
  let editError = $state(false);

  function openEdit(webhook: BotOutboundWebhook) {
    editId = webhook.id;
    editURL = editOriginalURL = webhook.url;
    editAuthorization = '';
    editHasAuthorization = webhook.hasAuthorization;
    removeAuthorization = false;
    editError = false;
    editVisible = true;
  }

  function closeEdit() {
    if (pending) return;
    editVisible = false;
    editAuthorization = '';
  }

  async function saveEdit() {
    if (pending) return;
    pending = true;
    editError = false;
    try {
      // Omitted fields preserve concurrent changes to unrelated settings.
      await scope.connection.getAPI(createBotAPI).updateOutboundWebhook(botId, editId, {
        ...(editURL !== editOriginalURL ? { url: editURL } : {}),
        ...(editAuthorization
          ? { authorization: editAuthorization }
          : removeAuthorization
            ? { authorization: '' }
            : {})
      });
      editVisible = false;
      editAuthorization = '';
      await query.refetch();
      toast.success(m('common.saved'));
    } catch {
      editError = true;
    } finally {
      pending = false;
    }
  }
  let revokeId = $state('');
  let revokeVisible = $state(false);

  // The shared dialog clears the newly issued secret when it closes.
  let signingSecret = $state('');
  let secretVisible = $state(false);

  function openCreate() {
    name = '';
    url = '';
    authorization = '';
    createError = false;
    createVisible = true;
  }

  function closeCreate() {
    if (pending) return;
    createVisible = false;
    authorization = '';
  }

  async function create() {
    if (pending) return;
    pending = true;
    createError = false;
    try {
      const result = await scope.connection.getAPI(createBotAPI).createOutboundWebhook({
        botUserId: botId,
        name: name.trim(),
        url,
        authorization,
        enabled: true
      });
      createVisible = false;
      signingSecret = result.signingSecret;
      secretVisible = !!signingSecret;
      authorization = '';
      await query.refetch();
      toast.success(m('settings.bots.outbound.created'));
    } catch {
      createError = true;
    } finally {
      pending = false;
    }
  }

  async function toggle(webhook: BotOutboundWebhook) {
    if (pending) return;
    pending = true;
    try {
      await scope.connection
        .getAPI(createBotAPI)
        .updateOutboundWebhook(botId, webhook.id, { enabled: !webhook.enabled });
      await query.refetch();
      toast.success(
        webhook.enabled ? m('settings.bots.outbound.paused') : m('settings.bots.outbound.resumed')
      );
    } catch {
      toast.error(m('settings.bots.outbound.error'));
    } finally {
      pending = false;
    }
  }

  async function revoke() {
    if (pending) return;
    pending = true;
    try {
      await scope.connection.getAPI(createBotAPI).revokeOutboundWebhook(botId, revokeId);
      revokeVisible = false;
      await query.refetch();
      toast.success(m('settings.bots.outbound.revoked'));
    } catch {
      toast.error(m('settings.bots.outbound.error'));
    } finally {
      pending = false;
    }
  }
</script>

<!-- @component Manages named outbound endpoints with editable destinations, independent pause/resume, and revocation. -->
<svelte:window
  onbeforeunload={(event) => {
    if (pending || secretVisible) {
      event.preventDefault();
      event.returnValue = '';
    }
  }}
/>
<BotIntegrationSection
  title={m('settings.bots.outbound.title')}
  description={m('settings.bots.outbound.description')}
  testId="bot-outbound-webhooks"
  items={webhooks}
  empty={query.isPending ? m('common.loading') : m('settings.bots.outbound.empty')}
>
  {#snippet actions()}
    <Button
      size="sm"
      disabled={pending || atLimit || query.isPending || query.isError}
      onclick={openCreate}
    >
      <span class="iconify icon-[uil--plus]" aria-hidden="true"></span>
      {m('settings.bots.outbound.add')}
    </Button>
  {/snippet}
  {#snippet details(webhook)}
    <BotIntegrationDetails name={webhook.name || m('settings.bots.outbound.unnamed')}>
      {#snippet status()}
        <span class="text-sm text-muted">
          {webhook.enabled
            ? m('settings.bots.outbound.active')
            : m('settings.bots.outbound.disabled')}
        </span>
      {/snippet}
      <div class="min-w-0">
        <dt class="text-muted">{m('settings.bots.webhook_created_at')}</dt>
        <dd>
          {webhook.createdAt
            ? formatDateTime(webhook.createdAt.toDate(), timeSettings, activeLocale)
            : '—'}
        </dd>
      </div>
      <div class="min-w-0">
        <dt class="text-muted">{m('settings.bots.outbound.url')}</dt>
        <dd class="truncate" title={webhook.url}><bdi>{webhook.url}</bdi></dd>
      </div>
    </BotIntegrationDetails>
    {#if webhook.latestDelivery?.status === BotWebhookDeliveryStatus.FAILED}
      {@const latest = webhook.latestDelivery}
      <div class="mt-3" role="status">
        <p class="text-warning">{m('settings.bots.outbound.failed')}</p>
        <BotWebhookFailureDetails failure={latest} />
      </div>
    {/if}
  {/snippet}
  {#snippet itemActions(webhook)}
    <Button
      size="sm"
      variant="secondary"
      label={m('settings.bots.outbound.edit')}
      title={m('settings.bots.outbound.edit')}
      disabled={pending}
      onclick={() => openEdit(webhook)}
    >
      <span class="iconify icon-[uil--edit-alt]" aria-hidden="true"></span>
    </Button>
    <Button
      size="sm"
      variant="secondary"
      label={m('settings.bots.outbound.history')}
      title={m('settings.bots.outbound.history')}
      onclick={() => {
        historyId = webhook.id;
        historyVisible = true;
      }}
    >
      <span class="iconify icon-[uil--history]" aria-hidden="true"></span>
    </Button>
    <Button
      size="sm"
      variant="secondary"
      disabled={pending}
      onclick={() => toggle(webhook)}
      label={webhook.enabled
        ? m('settings.bots.outbound.pause')
        : m('settings.bots.outbound.resume')}
      title={webhook.enabled
        ? m('settings.bots.outbound.pause')
        : m('settings.bots.outbound.resume')}
    >
      <span
        class={webhook.enabled ? 'iconify icon-[uil--pause]' : 'iconify icon-[uil--play]'}
        aria-hidden="true"
      ></span>
    </Button>
    <Button
      size="sm"
      variant="danger-secondary"
      label={m('settings.bots.outbound.revoke')}
      title={m('settings.bots.outbound.revoke')}
      disabled={pending}
      onclick={() => {
        revokeId = webhook.id;
        revokeVisible = true;
      }}
    >
      <span class="iconify icon-[uil--times-circle]" aria-hidden="true"></span>
    </Button>
  {/snippet}
  {#snippet footer()}
    {#if query.isError}<div class="p-5">
        <Hint tone="warning">{m('settings.bots.outbound.load_error')}</Hint>
      </div>{/if}
    {#if atLimit}<div class="border-t border-border px-5 py-3 text-muted">
        {m('settings.bots.outbound.limit')}
      </div>{/if}
  {/snippet}
</BotIntegrationSection>

<FormDialog
  bind:visible={createVisible}
  title={m('settings.bots.outbound.add')}
  submitLabel={m('settings.bots.outbound.add')}
  loading={pending}
  disabled={!name.trim() || !url.trim() || atLimit}
  error={createError ? m('settings.bots.outbound.error') : undefined}
  onsubmit={create}
  onclose={closeCreate}
>
  <TextInput
    id="bot-outbound-name"
    label={m('settings.bots.outbound.name')}
    bind:value={name}
    required
    maxlength={64}
    disabled={pending}
  />
  <TextInput
    id="bot-outbound-url"
    label={m('settings.bots.outbound.url')}
    type="url"
    bind:value={url}
    required
    maxlength={4096}
    disabled={pending}
    autocomplete="off"
  />
  <TextInput
    id="bot-outbound-authorization"
    label={m('settings.bots.outbound.authorization')}
    type="password"
    bind:value={authorization}
    maxlength={4096}
    disabled={pending}
    autocomplete="new-password"
  />
</FormDialog>
<FormDialog
  bind:visible={editVisible}
  title={m('settings.bots.outbound.edit')}
  submitLabel={m('common.save')}
  loading={pending}
  disabled={!editURL.trim()}
  error={editError ? m('settings.bots.outbound.error') : undefined}
  onsubmit={saveEdit}
  onclose={closeEdit}
>
  <TextInput
    id="edit-outbound-url"
    label={m('settings.bots.outbound.url')}
    type="url"
    bind:value={editURL}
    required
    maxlength={4096}
    disabled={pending}
    autocomplete="off"
  />
  <div>
    <div class="flex items-end gap-2">
      <div class="min-w-0 flex-1">
        <TextInput
          id="edit-outbound-authorization"
          label={m('settings.bots.outbound.authorization')}
          type="password"
          bind:value={editAuthorization}
          placeholder={editHasAuthorization && !removeAuthorization ? '••••••••' : ''}
          maxlength={4096}
          disabled={pending}
          autocomplete="new-password"
        />
      </div>
      {#if editHasAuthorization}
        <Button
          variant="ghost"
          disabled={pending}
          label={m(
            removeAuthorization
              ? 'settings.bots.outbound.auth_keep'
              : 'settings.bots.outbound.auth_remove'
          )}
          title={m(
            removeAuthorization
              ? 'settings.bots.outbound.auth_keep'
              : 'settings.bots.outbound.auth_remove'
          )}
          onclick={() => {
            removeAuthorization = !removeAuthorization;
            editAuthorization = '';
          }}
        >
          <span
            class={removeAuthorization ? 'iconify icon-[uil--redo]' : 'iconify icon-[uil--times]'}
            aria-hidden="true"
          ></span>
        </Button>
      {/if}
    </div>
    {#if editHasAuthorization && !editAuthorization}
      <p class="mt-1 text-sm text-muted" role="status">
        {m(
          removeAuthorization
            ? 'settings.bots.outbound.auth_removed'
            : 'settings.bots.outbound.auth_unchanged'
        )}
      </p>
    {/if}
  </div>
</FormDialog>

<ConfirmDialog
  bind:visible={revokeVisible}
  title={m('settings.bots.outbound.revoke_title')}
  actionLabel={m('settings.bots.outbound.revoke')}
  onconfirm={revoke}
  onclose={() => (revokeVisible = false)}
  loading={pending}>{m('settings.bots.outbound.revoke_description')}</ConfirmDialog
>
<ShowOnceCredentialDialog
  bind:visible={secretVisible}
  bind:value={signingSecret}
  {pending}
  title={m('settings.bots.outbound.secret_title')}
  warning={m('settings.bots.outbound.secret_warning')}
  copiedMessage={m('settings.bots.outbound.secret_copied')}
/>

<Dialog bind:visible={historyVisible} title={m('settings.bots.outbound.history')}>
  {#if historyVisible}
    {#key historyId}<BotWebhookFailureHistory {botId} webhookId={historyId} />{/key}
  {/if}
</Dialog>
