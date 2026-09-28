<!--
@component

Integrations section of a bot: outbound webhooks, incoming webhooks, and API
keys. Only the owner and bot managers can open it.
-->
<script lang="ts">
  import { toastError } from '$lib/utils/errorMessage';
  import { createBotAPI, type Bot } from '$lib/api-client/bots';
  import { RoomKind } from '$lib/api-client/roomDirectory';
  import BotCredentialSection, {
    type BotCredentialSectionItem
  } from '$lib/components/bots/BotCredentialSection.svelte';
  import BotOutboundWebhookSection from '$lib/components/bots/BotOutboundWebhookSection.svelte';
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { Hint, LoadingFog } from '$lib/ui';
  import { Select } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { formatDateTime, timeFormatSettingsFor } from '$lib/utils/formatTime';
  import { useBotDetail } from '../botDetailContext';

  const serverScope = useServerScope();
  const detail = useBotDetail();
  const bot = $derived(detail.bot);
  let webhookRoomId = $state('');
  const webhookRoomOptions = $derived([
    { value: '', label: m('settings.bots.webhook_room_none') },
    ...serverScope.store.navigation.rooms
      .filter((room) => room.type === RoomKind.CHANNEL)
      .map((room) => ({ value: room.id, label: room.name }))
      .sort((a, b) => a.label.localeCompare(b.label))
  ]);

  const timeSettings = $derived(
    timeFormatSettingsFor(serverScope.store.currentUser.user?.settings)
  );
  const activeLocale = $derived(getLocale());

  function botAPI() {
    return serverScope.connection.getAPI(createBotAPI);
  }

  async function createAPIKey(name: string): Promise<string | null> {
    if (!bot) return null;
    const mutationTarget = detail.botId;
    try {
      const created = await botAPI().createBotAPIKey(bot.id, name);
      if (!detail.isCurrentTarget(mutationTarget)) return null;
      detail.refreshBot();
      toast.success(m('settings.bots.key_created_toast'));
      return created.apiKey;
    } catch (error) {
      if (detail.isCurrentTarget(mutationTarget)) {
        toastError(error, m('settings.bots.key_create_failed'));
      }
      return null;
    }
  }

  async function revokeAPIKey(keyId: string): Promise<boolean> {
    if (!bot) return false;
    const mutationTarget = detail.botId;
    try {
      const updated = await botAPI().revokeBotAPIKey(bot.id, keyId);
      if (!detail.isCurrentTarget(mutationTarget)) return false;
      detail.cacheBot(updated);
      toast.success(m('settings.bots.key_revoked'));
      return true;
    } catch (error) {
      if (detail.isCurrentTarget(mutationTarget)) {
        toastError(error, m('settings.bots.key_revoke_failed'));
      }
      return false;
    }
  }

  async function createWebhook(name: string): Promise<string | null> {
    if (!bot) return null;
    const mutationTarget = detail.botId;
    const roomId = webhookRoomId;
    try {
      const created = await botAPI().createBotIncomingWebhook(bot.id, name);
      if (!detail.isCurrentTarget(mutationTarget)) return null;
      detail.refreshBot();
      toast.success(m('settings.bots.webhook_created'));
      if (!roomId) return created.webhookUrl;
      // The URL selects the destination; it does not restrict the credential's permissions.
      const url = new URL(created.webhookUrl);
      url.searchParams.set('room_id', roomId);
      return url.toString();
    } catch (error) {
      if (detail.isCurrentTarget(mutationTarget)) {
        toastError(error, m('settings.bots.webhook_create_failed'));
      }
      return null;
    }
  }

  async function revokeWebhook(webhookId: string): Promise<boolean> {
    if (!bot) return false;
    const mutationTarget = detail.botId;
    try {
      const updated = await botAPI().revokeBotIncomingWebhook(bot.id, webhookId);
      if (!detail.isCurrentTarget(mutationTarget)) return false;
      detail.cacheBot(updated);
      toast.success(m('settings.bots.webhook_revoked'));
      return true;
    } catch (error) {
      if (detail.isCurrentTarget(mutationTarget)) {
        toastError(error, m('settings.bots.webhook_revoke_failed'));
      }
      return false;
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

{#if detail.canOperateBot}
  {#key detail.botId}
    <BotOutboundWebhookSection botId={detail.botId} />
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
      oncreateopen={() => (webhookRoomId = '')}
      onrevoke={revokeWebhook}
    >
      {#snippet createFields(pending)}
        <Select
          id="create-bot-webhook-room"
          label={m('settings.bots.webhook_room')}
          description={m('settings.bots.webhook_room_description')}
          options={webhookRoomOptions}
          bind:value={webhookRoomId}
          disabled={pending}
        />
      {/snippet}
    </BotCredentialSection>

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
  {/key}
{:else if detail.isPending}
  <LoadingFog class="h-40 w-full" />
{:else}
  <Hint tone="danger">{m('ui.access_denied.message')}</Hint>
{/if}
