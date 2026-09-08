<script lang="ts">
  import {
    BotWebhookDeliveryStatus,
    type BotOutboundWebhook
  } from '@chatto/api-types/api/v1/bots_pb';
  import { createQuery } from '@tanstack/svelte-query';
  import { createBotAPI } from '$lib/api-client/bots';
  import { m } from '$lib/i18n/messages';
  import { queryClient } from '$lib/query/client';
  import { settingsQueryKeys } from '$lib/query/settings';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { ConfirmDialog, Dialog, FormDialog, Hint } from '$lib/ui';
  import { toast } from '$lib/ui/toast';
  import { Button, TextInput } from '$lib/ui/form';
  import BotIntegrationSection from './BotIntegrationSection.svelte';
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

  let historyId = $state('');
  let historyVisible = $state(false);

  let createVisible = $state(false);
  let name = $state('');
  let url = $state('');
  let authorization = $state('');
  let pending = $state(false);
  let createError = $state(false);
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
        .updateOutboundWebhook(botId, webhook.id, !webhook.enabled);
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

<!-- @component Manages named outbound endpoints with immutable credentials, independent pause/resume, and revocation. -->
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
    <div class="font-medium text-text-top">
      <bdi>{webhook.name || m('settings.bots.outbound.unnamed')}</bdi>
    </div>
    <p class="mt-1 break-all text-muted"><bdi>{webhook.url}</bdi></p>
    <p class="mt-2 text-muted">
      {webhook.enabled ? m('settings.bots.outbound.active') : m('settings.bots.outbound.disabled')}
    </p>
    {#if webhook.latestDelivery?.status === BotWebhookDeliveryStatus.FAILED}
      {@const latest = webhook.latestDelivery}
      <div class="mt-3" role="status">
        <p class="text-warning">{m('settings.bots.outbound.failed')}</p>
        <BotWebhookFailureDetails failure={latest} />
      </div>
    {/if}
    <div class="mt-3">
      <Button
        size="sm"
        variant="secondary"
        onclick={() => {
          historyId = webhook.id;
          historyVisible = true;
        }}
      >
        {m('settings.bots.outbound.history')}
      </Button>
    </div>
  {/snippet}
  {#snippet itemActions(webhook)}
    <Button size="sm" variant="secondary" disabled={pending} onclick={() => toggle(webhook)}>
      {webhook.enabled ? m('settings.bots.outbound.pause') : m('settings.bots.outbound.resume')}
    </Button>
    <Button
      size="sm"
      variant="danger-secondary"
      disabled={pending}
      onclick={() => {
        revokeId = webhook.id;
        revokeVisible = true;
      }}
    >
      <span class="iconify icon-[uil--times-circle]" aria-hidden="true"></span>
      {m('settings.bots.outbound.revoke')}
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
