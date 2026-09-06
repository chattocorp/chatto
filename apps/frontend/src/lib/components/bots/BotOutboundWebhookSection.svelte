<script lang="ts">
  import { BotWebhookDeliveryStatus } from '@chatto/api-types/api/v1/bots_pb';
  import { createQuery } from '@tanstack/svelte-query';
  import { createBotAPI } from '$lib/api-client/bots';
  import { m } from '$lib/i18n/messages';
  import { queryClient } from '$lib/query/client';
  import { settingsQueryKeys } from '$lib/query/settings';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import { ConfirmDialog, Hint } from '$lib/ui';
  import { Button, Checkbox, TextInput } from '$lib/ui/form';
  import ShowOnceCredentialDialog from './ShowOnceCredentialDialog.svelte';

  let { botId }: { botId: string } = $props();
  const scope = useServerScope();
  const query = createQuery(
    () => ({
      queryKey: [...settingsQueryKeys.bot(scope.serverId, scope.connection, botId), 'outbound'],
      queryFn: ({ signal }) =>
        scope.connection.getAPI(createBotAPI).getOutboundWebhook(botId, signal),
      refetchInterval: 5000
    }),
    () => queryClient
  );
  const webhook = $derived(query.data);
  const latest = $derived(webhook?.latestDelivery);
  let url = $state('');
  let authorization = $state('');
  let enabled = $state(true);
  let pending = $state(false);
  let error = $state(false);
  let secret = $state('');
  let secretVisible = $state(false);
  let removeVisible = $state(false);

  async function save(event: SubmitEvent) {
    event.preventDefault();
    if (pending) return;
    pending = true;
    error = false;
    try {
      const result = await scope.connection
        .getAPI(createBotAPI)
        .replaceOutboundWebhook({ botUserId: botId, url, authorization, enabled });
      secret = result.signingSecret;
      secretVisible = true;
      url = '';
      authorization = '';
      await query.refetch();
    } catch {
      error = true;
    } finally {
      pending = false;
    }
  }

  async function remove() {
    pending = true;
    error = false;
    try {
      await scope.connection.getAPI(createBotAPI).deleteOutboundWebhook(botId);
      removeVisible = false;
      await query.refetch();
    } catch {
      error = true;
    } finally {
      pending = false;
    }
  }
</script>

<!-- @component Configures one outbound bot endpoint and shows its latest durable delivery outcome. -->
<svelte:window
  onbeforeunload={(event) => {
    if (pending || secretVisible) {
      event.preventDefault();
      event.returnValue = '';
    }
  }}
/>
<Panel title={m('settings.bots.outbound.title')}>
  <div class="flex flex-col gap-4" data-testid="bot-outbound-webhook">
    <p class="text-text-dim">{m('settings.bots.outbound.description')}</p>
    {#if query.isError}
      <Hint tone="warning">{m('settings.bots.outbound.load_error')}</Hint>
    {:else if webhook}
      <p>
        {webhook.enabled
          ? m('settings.bots.outbound.active')
          : m('settings.bots.outbound.disabled')}
      </p>
      {#if latest}
        <div class="surface-box p-3" role="status">
          <p>
            {latest.status === BotWebhookDeliveryStatus.DELIVERED
              ? m('settings.bots.outbound.delivered')
              : latest.status === BotWebhookDeliveryStatus.FAILED
                ? m('settings.bots.outbound.failed')
                : m('settings.bots.outbound.skipped')}
          </p>
          <p class="text-text-dim">
            {m('settings.bots.outbound.attempts', { attempts: latest.attempts })}
          </p>
          {#if latest.httpStatus}<p>
              {m('settings.bots.outbound.http_status', { status: latest.httpStatus })}
            </p>{/if}
          {#if latest.reason}<p>
              {m('settings.bots.outbound.reason', { reason: latest.reason })}
            </p>{/if}
        </div>
      {:else}<p class="text-text-dim">{m('settings.bots.outbound.pending')}</p>{/if}
      <p class="text-text-dim">{m('settings.bots.outbound.replace_help')}</p>
    {:else if !query.isPending}
      <p>{m('settings.bots.outbound.empty')}</p>
    {/if}
    {#if error}<Hint tone="warning">{m('settings.bots.outbound.error')}</Hint>{/if}
    <form onsubmit={save} class="flex flex-col gap-4">
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
      <Checkbox
        id="bot-outbound-enabled"
        bind:checked={enabled}
        label={m('settings.bots.outbound.enabled')}
        disabled={pending}
      />
      <div class="flex flex-wrap gap-3">
        <Button
          type="submit"
          disabled={pending || query.isPending || query.isError}
          loading={pending}
          >{webhook
            ? m('settings.bots.outbound.replace')
            : m('settings.bots.outbound.save')}</Button
        >
        {#if webhook}<Button
            variant="danger"
            disabled={pending}
            onclick={() => (removeVisible = true)}>{m('settings.bots.outbound.remove')}</Button
          >{/if}
        <Button variant="secondary" disabled={pending} onclick={() => query.refetch()}
          >{m('settings.bots.outbound.refresh')}</Button
        >
      </div>
    </form>
  </div>
</Panel>
<ConfirmDialog
  bind:visible={removeVisible}
  title={m('settings.bots.outbound.remove_title')}
  onconfirm={remove}
  onclose={() => (removeVisible = false)}
  loading={pending}>{m('settings.bots.outbound.remove_description')}</ConfirmDialog
>
<ShowOnceCredentialDialog
  bind:visible={secretVisible}
  bind:value={secret}
  {pending}
  title={m('settings.bots.outbound.secret_title')}
  warning={m('settings.bots.outbound.secret_warning')}
  copiedMessage={m('settings.bots.outbound.secret_copied')}
/>
