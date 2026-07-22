<!-- SPDX-License-Identifier: Apache-2.0 -->
<!--
@component

Shared full-page bot editor for owners and administrators. The backend remains
the authorization boundary; `scope` controls navigation and credential actions.
-->
<script lang="ts">
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { createBotAPI, type BotAccount } from '$lib/api-client/bots';
  import { Panel } from '$lib/components/admin';
  import { Button, TextArea, TextInput } from '$lib/ui/form';
  import { AccessDenied, EmptyState, Hint, PaneContent, PaneHeader, PageTitle } from '$lib/ui';
  import { toast } from '$lib/ui/toast';
  import { classifyManagementLoadError } from '$lib/utils/managementLoadError';
  import { isCurrentResourceOperation } from '$lib/utils/resourceOperationFence';
  import BotCredentialsDialog from './BotCredentialsDialog.svelte';
  import BotPermissionsMatrix from './BotPermissionsMatrix.svelte';
  import * as m from '$lib/i18n/messages';

  let { botId, scope }: { botId: string; scope: 'owner' | 'admin' } = $props();

  const connection = useConnection();
  let bot = $state<BotAccount | null>(null);
  let loading = $state(true);
  let accessDenied = $state(false);
  let loadFailure = $state<string | null>(null);
  let saving = $state(false);
  let saveError = $state<string | null>(null);
  let login = $state('');
  let displayName = $state('');
  let description = $state('');
  let originalLogin = $state('');
  let originalDisplayName = $state('');
  let originalDescription = $state('');
  let credentialAction = $state<'rotate' | 'revoke' | null>(null);
  let loadGeneration = 0;

  const normalizedLogin = $derived(login.trim());
  const normalizedDisplayName = $derived(displayName.trim());
  const normalizedDescription = $derived(description.trim());
  const loginValid = $derived(normalizedLogin.toLowerCase().endsWith('_bot'));
  const dirty = $derived(
    normalizedLogin !== originalLogin ||
      normalizedDisplayName !== originalDisplayName ||
      normalizedDescription !== originalDescription
  );
  const formValid = $derived(
    loginValid && normalizedDisplayName.length > 0 && normalizedDescription.length > 0 && dirty
  );
  const serverId = $derived(serverIdToSegment(getActiveServer()));
  const backHref = $derived(
    scope === 'owner'
      ? resolve('/chat/[serverId]/settings/bots', { serverId })
      : resolve('/chat/[serverId]/manage/server/bots', { serverId })
  );

  function api() {
    const conn = connection();
    return createBotAPI({ baseUrl: conn.connectBaseUrl, bearerToken: conn.bearerToken });
  }

  function applyBot(nextBot: BotAccount) {
    bot = nextBot;
    login = nextBot.login;
    displayName = nextBot.displayName;
    description = nextBot.description;
    originalLogin = nextBot.login;
    originalDisplayName = nextBot.displayName;
    originalDescription = nextBot.description;
  }

  async function loadBot(targetBotId: string) {
    const generation = ++loadGeneration;
    loading = true;
    accessDenied = false;
    loadFailure = null;
    saveError = null;
    saving = false;
    credentialAction = null;
    bot = null;
    try {
      const nextBot = await api().getBot(targetBotId);
      if (generation !== loadGeneration || targetBotId !== botId) return;
      applyBot(nextBot);
    } catch (error) {
      if (generation !== loadGeneration || targetBotId !== botId) return;
      const classified = classifyManagementLoadError(error);
      if (classified.kind === 'access-denied') accessDenied = true;
      else loadFailure = classified.message;
    } finally {
      if (generation === loadGeneration && targetBotId === botId) loading = false;
    }
  }

  $effect(() => {
    void loadBot(botId);
  });

  async function save(event: SubmitEvent) {
    event.preventDefault();
    if (!bot || !formValid || saving) return;
    const target = { resourceId: botId, generation: loadGeneration };
    saving = true;
    saveError = null;
    try {
      const updated = await api().updateBot({
        botId: target.resourceId,
        login: normalizedLogin === originalLogin ? undefined : normalizedLogin,
        displayName:
          normalizedDisplayName === originalDisplayName ? undefined : normalizedDisplayName,
        description:
          normalizedDescription === originalDescription ? undefined : normalizedDescription
      });
      if (!isCurrentResourceOperation(target, botId, loadGeneration)) return;
      applyBot(updated);
      toast.success(m['bots.toast.updated']());
    } catch (error) {
      if (!isCurrentResourceOperation(target, botId, loadGeneration)) return;
      saveError = error instanceof Error ? error.message : m['bots.error.save_failed']();
    } finally {
      if (isCurrentResourceOperation(target, botId, loadGeneration)) saving = false;
    }
  }

  function updateCredentialBot(updated: BotAccount) {
    // Credential changes must not discard unsaved edits in the general form.
    if (updated.id === botId) bot = updated;
  }
</script>

<PageTitle
  title={bot
    ? `${bot.displayName} | ${m['bots.settings.title']()}`
    : m['bots.settings.page_title']()}
/>

{#if loading}
  <!-- Keep the surrounding settings/admin shell stable while the bot loads. -->
{:else if loadFailure}
  <EmptyState icon="uil--exclamation-triangle" title={m['common.error.generic']()}>
    <div class="flex flex-col items-center gap-4">
      <p>{loadFailure}</p>
      <Button variant="secondary" onclick={() => void loadBot(botId)}>{m['common.retry']()}</Button>
    </div>
  </EmptyState>
{:else if accessDenied || !bot}
  <AccessDenied message={m['ui.access_denied.message']()} {backHref} />
{:else}
  <div class="flex min-h-0 min-w-0 flex-1 flex-col">
    <PaneHeader title={bot.displayName} subtitle={`@${bot.login}`} {backHref} showMobileNav />

    <PaneContent>
      <div class="flex flex-col gap-6">
        <Panel title={m['admin.nav.general']()} icon="iconify uil--setting">
          <form class="flex max-w-2xl flex-col gap-4" onsubmit={save}>
            {#if saveError}<Hint tone="danger">{saveError}</Hint>{/if}
            <TextInput
              id="bot-detail-display-name"
              label={m['bots.field.display_name']()}
              maxlength={100}
              required
              disabled={saving}
              bind:value={displayName}
            />
            <TextInput
              id="bot-detail-username"
              label={m['bots.field.username']()}
              description={m['bots.help.username']()}
              error={login.length > 0 && !loginValid
                ? m['bots.error.username_suffix']()
                : undefined}
              maxlength={64}
              required
              disabled={saving}
              bind:value={login}
            />
            <TextArea
              id="bot-detail-description"
              label={m['bots.field.description']()}
              description={m['bots.help.description']()}
              maxBytes={2000}
              rows={5}
              required
              disabled={saving}
              bind:value={description}
            />
            <div class="flex justify-end">
              <Button type="submit" loading={saving} disabled={!formValid}>
                {m['bots.action.save']()}
              </Button>
            </div>
          </form>
        </Panel>

        <Panel title={m['bots.field.api_key']()} icon="iconify uil--key-skeleton">
          <div class="flex max-w-2xl items-center justify-between gap-4">
            <div class="min-w-0">
              <p class="font-medium">
                {bot.apiKeyCreatedAt
                  ? m['bots.credentials.active_description']()
                  : m['bots.credentials.none_description']()}
              </p>
            </div>
            {#if scope === 'owner'}
              <Button variant="secondary" onclick={() => (credentialAction = 'rotate')}>
                <span class="iconify uil--repeat" aria-hidden="true"></span>
                {m['bots.credentials.rotate']()}
              </Button>
            {:else if bot.apiKeyCreatedAt}
              <Button variant="danger-secondary" onclick={() => (credentialAction = 'revoke')}>
                <span class="iconify uil--key-skeleton-alt" aria-hidden="true"></span>
                {m['bots.credentials.revoke']()}
              </Button>
            {/if}
          </div>
        </Panel>

        <BotPermissionsMatrix {botId} />
      </div>
    </PaneContent>
  </div>
{/if}

{#if bot && credentialAction}
  <BotCredentialsDialog
    {bot}
    action={credentialAction}
    onupdated={updateCredentialBot}
    onclose={() => (credentialAction = null)}
  />
{/if}
