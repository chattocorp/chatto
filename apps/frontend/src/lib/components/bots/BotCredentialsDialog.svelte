<!-- SPDX-License-Identifier: Apache-2.0 -->
<!--
@component

Shared bot API-key management dialog. Owners may issue the show-once secret;
administrators may only revoke an existing credential.
-->
<script lang="ts">
  import { createBotAPI, type BotAccount } from '$lib/api-client/bots';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { ConfirmDialog, Dialog, Hint, Pill } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import * as m from '$lib/i18n/messages';

  let {
    bot,
    canRotate,
    onupdated,
    onclose
  }: {
    bot: BotAccount;
    canRotate: boolean;
    onupdated: (bot: BotAccount) => void;
    onclose: () => void;
  } = $props();

  const connection = useConnection();
  let secret = $state<string | null>(null);
  let confirmAction = $state<'rotate' | 'revoke' | null>(null);
  let loading = $state(false);
  let error = $state<string | null>(null);

  function api() {
    const conn = connection();
    return createBotAPI({ baseUrl: conn.connectBaseUrl, bearerToken: conn.bearerToken });
  }

  function close() {
    secret = null;
    confirmAction = null;
    error = null;
    onclose();
  }

  async function rotate() {
    loading = true;
    error = null;
    try {
      const result = await api().rotateAPIKey(bot.id);
      secret = result.apiKey;
      onupdated(result.bot);
      confirmAction = null;
      toast.success(m['bots.credentials.toast.rotated']());
    } catch (cause) {
      error = cause instanceof Error ? cause.message : String(cause);
    } finally {
      loading = false;
    }
  }

  async function revoke() {
    loading = true;
    error = null;
    try {
      const updated = await api().revokeAPIKey(bot.id);
      secret = null;
      onupdated(updated);
      confirmAction = null;
      toast.success(m['bots.credentials.toast.revoked']());
    } catch (cause) {
      error = cause instanceof Error ? cause.message : String(cause);
    } finally {
      loading = false;
    }
  }

  async function copySecret() {
    if (!secret) return;
    try {
      await navigator.clipboard.writeText(secret);
      toast.success(m['common.copied_to_clipboard']());
    } catch {
      toast.error(m['bots.credentials.copy_failed']());
    }
  }
</script>

<Dialog visible title={m['bots.credentials.title']({ bot: bot.displayName })} size="md" onclose={close}>
  <div class="flex flex-col gap-5">
    {#if error}
      <Hint tone="danger">{error}</Hint>
    {/if}

    <div class="flex items-center justify-between gap-4 rounded-lg border border-border p-4">
      <div>
        <div class="font-medium">{m['bots.field.api_key']()}</div>
        <div class="text-sm text-muted">
          {bot.apiKeyCreatedAt
            ? m['bots.credentials.active_description']()
            : m['bots.credentials.none_description']()}
        </div>
      </div>
      <Pill tone={bot.apiKeyCreatedAt ? 'success' : 'neutral'}>
        {bot.apiKeyCreatedAt ? m['bots.api_key.active']() : m['bots.api_key.none']()}
      </Pill>
    </div>

    {#if secret}
      <Hint tone="warning">{m['bots.credentials.show_once']()}</Hint>
      <div class="flex items-start gap-2 rounded-lg bg-surface-emphasized p-3">
        <code class="min-w-0 flex-1 break-all text-sm select-all" data-testid="bot-api-key-secret"
          >{secret}</code
        >
        <Button variant="secondary" size="sm" onclick={copySecret}>
          <span class="iconify uil--copy" aria-hidden="true"></span>
          {m['bots.credentials.copy']()}
        </Button>
      </div>
    {/if}

    <div class="flex flex-wrap gap-2">
      {#if canRotate}
        <Button variant="action" onclick={() => (confirmAction = 'rotate')}>
          <span class="iconify uil--key-skeleton" aria-hidden="true"></span>
          {bot.apiKeyCreatedAt
            ? m['bots.credentials.rotate']()
            : m['bots.credentials.generate']()}
        </Button>
      {/if}
      {#if bot.apiKeyCreatedAt}
        <Button variant="danger-secondary" onclick={() => (confirmAction = 'revoke')}>
          <span class="iconify uil--key-skeleton-alt" aria-hidden="true"></span>
          {m['bots.credentials.revoke']()}
        </Button>
      {/if}
    </div>
  </div>
</Dialog>

{#if confirmAction === 'rotate'}
  <ConfirmDialog
    visible
    title={bot.apiKeyCreatedAt
      ? m['bots.credentials.rotate_confirm_title']()
      : m['bots.credentials.generate_confirm_title']()}
    tone="warning"
    actionLabel={bot.apiKeyCreatedAt
      ? m['bots.credentials.rotate']()
      : m['bots.credentials.generate']()}
    loading={loading}
    onconfirm={rotate}
    onclose={() => (confirmAction = null)}
  >
    {bot.apiKeyCreatedAt
      ? m['bots.credentials.rotate_confirm_body']()
      : m['bots.credentials.generate_confirm_body']()}
  </ConfirmDialog>
{:else if confirmAction === 'revoke'}
  <ConfirmDialog
    visible
    title={m['bots.credentials.revoke_confirm_title']()}
    actionLabel={m['bots.credentials.revoke']()}
    loading={loading}
    onconfirm={revoke}
    onclose={() => (confirmAction = null)}
  >
    {m['bots.credentials.revoke_confirm_body']()}
  </ConfirmDialog>
{/if}
