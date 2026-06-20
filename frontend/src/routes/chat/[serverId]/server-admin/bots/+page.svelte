<script lang="ts">
  import { onMount } from 'svelte';
  import { graphql } from '$lib/gql';
  import { BotTokenExpiryPreset } from '$lib/gql/graphql';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { Panel, DataTable } from '$lib/components/admin';
  import { Button, FormError, Select, TextInput } from '$lib/ui/form';
  import { Hint, Pill } from '$lib/ui';
  import FormField from '$lib/ui/form/FormField.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { toast } from '$lib/ui/toast';
  import { getUserSettings } from '$lib/state/userSettings.svelte';
  import { formatDate as formatDateUtil } from '$lib/utils/formatTime';

  type Bot = {
    id: string;
    login: string;
    displayName: string;
    botOwner?: { id: string; login: string; displayName: string } | null;
  };

  type BotToken = {
    id: string;
    name: string;
    createdAt: string;
    expiresAt?: string | null;
    lastUsedAt?: string | null;
    revokedAt?: string | null;
    revokeReason?: string | null;
    createdBy?: { id: string; login: string; displayName: string } | null;
    revokedBy?: { id: string; login: string; displayName: string } | null;
  };

  const connection = useConnection();
  const userSettings = getUserSettings();

  let bots = $state<Bot[]>([]);
  let selectedBotId = $state('');
  let tokens = $state<BotToken[]>([]);
  let loading = $state(true);
  let loadingTokens = $state(false);
  let error = $state<string | null>(null);

  let botLogin = $state('');
  let botDisplayName = $state('');
  let creatingBot = $state(false);
  let botError = $state<string | null>(null);

  let tokenName = $state('');
  let tokenExpiry = $state<BotTokenExpiryPreset>(BotTokenExpiryPreset.Days_90);
  let customExpiresAt = $state('');
  let creatingToken = $state(false);
  let revokingTokenId = $state<string | null>(null);
  let tokenError = $state<string | null>(null);
  let createdSecret = $state<string | null>(null);

  const selectedBot = $derived(bots.find((bot) => bot.id === selectedBotId) ?? null);
  const showCustomExpiry = $derived(tokenExpiry === BotTokenExpiryPreset.Custom);
  const canCreateToken = $derived(
    !!selectedBotId &&
      tokenName.trim().length > 0 &&
      (!showCustomExpiry || customExpiresAt.trim().length > 0)
  );

  const expiryOptions = [
    { value: BotTokenExpiryPreset.Days_30, label: '30 days' },
    { value: BotTokenExpiryPreset.Days_90, label: '90 days' },
    { value: BotTokenExpiryPreset.Days_365, label: '365 days' },
    { value: BotTokenExpiryPreset.Indefinite, label: 'Indefinite' },
    { value: BotTokenExpiryPreset.Custom, label: 'Custom' }
  ];

  async function loadBots() {
    loading = true;
    error = null;
    const resp = await connection().client.query(
      graphql(`
        query BotAccountsAdmin {
          bots {
            id
            login
            displayName
            botOwner {
              id
              login
              displayName
            }
          }
        }
      `),
      {}
    );
    loading = false;

    if (resp.error) {
      error = resp.error.message;
      return;
    }

    bots = resp.data?.bots ?? [];
    if (!selectedBotId && bots.length > 0) {
      selectedBotId = bots[0].id;
      await loadTokens();
    } else if (selectedBotId && !bots.some((bot) => bot.id === selectedBotId)) {
      selectedBotId = bots[0]?.id ?? '';
      await loadTokens();
    }
  }

  async function loadTokens() {
    tokens = [];
    createdSecret = null;
    if (!selectedBotId) return;

    loadingTokens = true;
    tokenError = null;
    const resp = await connection().client.query(
      graphql(`
        query BotTokenList($botUserId: ID!) {
          botTokens(botUserId: $botUserId) {
            id
            name
            createdAt
            expiresAt
            lastUsedAt
            revokedAt
            revokeReason
            createdBy {
              id
              login
              displayName
            }
            revokedBy {
              id
              login
              displayName
            }
          }
        }
      `),
      { botUserId: selectedBotId }
    );
    loadingTokens = false;

    if (resp.error) {
      tokenError = resp.error.message;
      return;
    }
    tokens = resp.data?.botTokens ?? [];
  }

  async function createBot(event: Event) {
    event.preventDefault();
    botError = null;
    creatingBot = true;

    const resp = await connection().client.mutation(
      graphql(`
        mutation CreateBotAccount($input: CreateBotInput!) {
          createBot(input: $input) {
            id
            login
            displayName
            botOwner {
              id
              login
              displayName
            }
          }
        }
      `),
      { input: { login: botLogin.trim(), displayName: botDisplayName.trim() } }
    );
    creatingBot = false;

    if (resp.error) {
      botError = resp.error.message;
      return;
    }
    const bot = resp.data?.createBot;
    if (bot) {
      bots = [...bots, bot];
      selectedBotId = bot.id;
      botLogin = '';
      botDisplayName = '';
      tokens = [];
      toast.success('Bot account created');
    }
  }

  async function createToken(event: Event) {
    event.preventDefault();
    if (!canCreateToken) return;

    tokenError = null;
    createdSecret = null;
    creatingToken = true;

    const input: {
      botUserId: string;
      name: string;
      expiry: BotTokenExpiryPreset;
      customExpiresAt?: string;
    } = {
      botUserId: selectedBotId,
      name: tokenName.trim(),
      expiry: tokenExpiry
    };
    if (tokenExpiry === BotTokenExpiryPreset.Custom) {
      const customDate = new Date(customExpiresAt);
      if (Number.isNaN(customDate.getTime())) {
        tokenError = 'Choose a valid custom expiration date.';
        creatingToken = false;
        return;
      }
      input.customExpiresAt = customDate.toISOString();
    }

    const resp = await connection().client.mutation(
      graphql(`
        mutation CreateBotApiToken($input: CreateBotTokenInput!) {
          createBotToken(input: $input) {
            secret
            token {
              id
              name
              createdAt
              expiresAt
              lastUsedAt
              revokedAt
              revokeReason
              createdBy {
                id
                login
                displayName
              }
              revokedBy {
                id
                login
                displayName
              }
            }
          }
        }
      `),
      { input }
    );
    creatingToken = false;

    if (resp.error) {
      tokenError = resp.error.message;
      return;
    }

    const created = resp.data?.createBotToken;
    if (created) {
      tokens = [created.token, ...tokens];
      createdSecret = created.secret;
      tokenName = '';
      tokenExpiry = BotTokenExpiryPreset.Days_90;
      customExpiresAt = '';
      toast.success('Bot token created');
    }
  }

  async function revokeToken(token: BotToken) {
    if (!selectedBotId || revokingTokenId) return;
    revokingTokenId = token.id;
    tokenError = null;

    const resp = await connection().client.mutation(
      graphql(`
        mutation RevokeBotApiToken($input: RevokeBotTokenInput!) {
          revokeBotToken(input: $input)
        }
      `),
      { input: { botUserId: selectedBotId, tokenId: token.id } }
    );
    revokingTokenId = null;

    if (resp.error) {
      tokenError = resp.error.message;
      return;
    }
    await loadTokens();
    toast.success('Bot token revoked');
  }

  async function copySecret() {
    if (!createdSecret) return;
    await navigator.clipboard.writeText(createdSecret);
    toast.success('Token copied');
  }

  function formatDate(dateStr: string | null | undefined): string {
    if (!dateStr) return '—';
    return formatDateUtil(dateStr, userSettings);
  }

  function formatExpiry(token: BotToken): string {
    if (token.revokedAt) return 'Revoked';
    return token.expiresAt ? formatDate(token.expiresAt) : 'Indefinite';
  }

  onMount(() => {
    void loadBots();
  });
</script>

<PageTitle title="Bots | Server Admin" />

<PaneHeader title="Bots" subtitle="Create bot accounts and manage their API tokens" showMobileNav />

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  {#if error}
    <Hint tone="danger">{error}</Hint>
  {/if}

  <Panel title="Create Bot" icon="iconify mdi--robot">
    <form class="grid gap-4 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]" onsubmit={createBot}>
      <TextInput
        label="Login"
        bind:value={botLogin}
        placeholder="deploy-bot"
        required
        disabled={creatingBot}
      />
      <TextInput
        label="Display Name"
        bind:value={botDisplayName}
        placeholder="Deploy Bot"
        required
        disabled={creatingBot}
      />
      <div class="flex items-end">
        <Button
          type="submit"
          disabled={!botLogin.trim() || !botDisplayName.trim() || creatingBot}
          loading={creatingBot}
          loadingText="Creating..."
        >
          <span class="iconify uil--plus"></span>
          Create
        </Button>
      </div>
    </form>
    {#if botError}
      <div class="mt-4">
        <FormError error={botError} />
      </div>
    {/if}
  </Panel>

  <div class="grid min-h-0 gap-6 xl:grid-cols-[minmax(280px,360px)_minmax(0,1fr)]">
    <Panel title="Bot Accounts" icon="iconify uil--users-alt" noPadding>
      {#if loading}
        <div class="p-4 text-muted">Loading bots...</div>
      {:else}
        <DataTable items={bots} columns={2} emptyMessage="No bot accounts yet" onRowClick={(bot) => {
          selectedBotId = bot.id;
          loadTokens();
        }}>
          {#snippet header()}
            <th class="px-4 py-3 font-medium">Bot</th>
            <th class="px-4 py-3 font-medium">Owner</th>
          {/snippet}
          {#snippet row(bot)}
            <td class="px-4 py-3">
              <div class="flex items-center gap-2">
                <span class="iconify mdi--robot text-muted"></span>
                <div class="min-w-0">
                  <div class="truncate font-medium">{bot.displayName}</div>
                  <div class="truncate text-sm text-muted">@{bot.login}</div>
                </div>
                {#if bot.id === selectedBotId}
                  <Pill>Selected</Pill>
                {/if}
              </div>
            </td>
            <td class="px-4 py-3 text-sm text-muted">
              {#if bot.botOwner}
                @{bot.botOwner.login}
              {:else}
                —
              {/if}
            </td>
          {/snippet}
        </DataTable>
      {/if}
    </Panel>

    <div class="flex min-w-0 flex-col gap-6">
      <Panel title="Create Token" icon="iconify uil--key-skeleton">
        {#if !selectedBot}
          <div class="text-muted">Select a bot account first.</div>
        {:else}
          <form class="flex flex-col gap-4" onsubmit={createToken}>
            {#if tokenError}
              <FormError error={tokenError} />
            {/if}
            <div class="grid gap-4 md:grid-cols-[minmax(0,1fr)_220px]">
              <TextInput
                label="Token Name"
                bind:value={tokenName}
                placeholder="Production deploy"
                required
                disabled={creatingToken}
              />
              <Select
                id="bot-token-expiry"
                label="Expiry"
                bind:value={tokenExpiry}
                options={expiryOptions}
                disabled={creatingToken}
              />
            </div>
            {#if showCustomExpiry}
              <FormField
                id="bot-token-custom-expiry"
                label="Custom Expiration"
                required
                description="Choose a future date and time."
              >
                <input
                  id="bot-token-custom-expiry"
                  type="datetime-local"
                  bind:value={customExpiresAt}
                  required
                  disabled={creatingToken}
                  class="input max-w-xs"
                />
              </FormField>
            {/if}
            <div>
              <Button
                type="submit"
                disabled={!canCreateToken || creatingToken}
                loading={creatingToken}
                loadingText="Creating..."
              >
                <span class="iconify uil--plus"></span>
                Create Token
              </Button>
            </div>
          </form>
          {#if createdSecret}
            <div class="mt-4 rounded-lg border border-accent bg-accent/10 p-4">
              <div class="mb-2 text-sm font-medium">Token secret</div>
              <div class="flex min-w-0 items-center gap-2">
                <code class="min-w-0 flex-1 overflow-x-auto rounded bg-surface px-3 py-2 text-xs">
                  {createdSecret}
                </code>
                <Button type="button" variant="secondary" onclick={copySecret}>
                  <span class="iconify uil--copy"></span>
                  Copy
                </Button>
              </div>
            </div>
          {/if}
        {/if}
      </Panel>

      <Panel title={selectedBot ? `Tokens for ${selectedBot.displayName}` : 'Tokens'} icon="iconify uil--key-skeleton" noPadding>
        {#if !selectedBot}
          <div class="p-4 text-muted">Select a bot account first.</div>
        {:else if loadingTokens}
          <div class="p-4 text-muted">Loading tokens...</div>
        {:else}
          <DataTable items={tokens} columns={5} emptyMessage="No tokens for this bot">
            {#snippet header()}
              <th class="px-4 py-3 font-medium">Name</th>
              <th class="px-4 py-3 font-medium">Created</th>
              <th class="px-4 py-3 font-medium">Last Used</th>
              <th class="px-4 py-3 font-medium">Expiry</th>
              <th class="px-4 py-3 font-medium">Actions</th>
            {/snippet}
            {#snippet row(token)}
              <td class="px-4 py-3">
                <div class="flex items-center gap-2">
                  <span class="font-medium">{token.name}</span>
                  {#if token.revokedAt}
                    <Pill>Revoked</Pill>
                  {/if}
                </div>
              </td>
              <td class="px-4 py-3 text-sm text-muted">{formatDate(token.createdAt)}</td>
              <td class="px-4 py-3 text-sm text-muted">{formatDate(token.lastUsedAt)}</td>
              <td class="px-4 py-3 text-sm text-muted">{formatExpiry(token)}</td>
              <td class="px-4 py-3">
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  disabled={!!token.revokedAt || revokingTokenId === token.id}
                  loading={revokingTokenId === token.id}
                  loadingText="Revoking..."
                  onclick={() => revokeToken(token)}
                >
                  Revoke
                </Button>
              </td>
            {/snippet}
          </DataTable>
        {/if}
      </Panel>
    </div>
  </div>
</div>
