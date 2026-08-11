<script lang="ts">
  import { resolve } from '$app/paths';
  import { onMount } from 'svelte';
  import { completeOriginAuthentication } from '$lib/auth/originAuthentication';
  import { startRemoteReauthentication } from '$lib/auth/reauth';
  import { findAuthlingServerProvider } from '$lib/authling/serverProvider';
  import { getClientConfiguration } from '$lib/clientConfig';
  import { navigateAfterAuthentication } from '$lib/auth/returnNavigation';
  import AuthLayout from '$lib/components/AuthLayout.svelte';
  import { m } from '$lib/i18n/messages';
  import type { PublicAuthProvider } from '$lib/api-client/server';
  import Divider from '$lib/ui/Divider.svelte';
  import Hint from '$lib/ui/Hint.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { TextInput, Button, Form } from '$lib/ui/form';
  import { serverRegistry, type RegisteredServer } from '$lib/state/server/registry.svelte';

  type AccountDataSyncAPI = typeof import('$lib/accountData/sync.svelte').accountDataSync;

  const { data } = $props();

  let identifier = $state('');
  let password = $state('');
  let error = $state('');
  let isLoading = $state(false);
  let selectedProviderId = $state<string | null>(null);
  let pageErrorDismissed = $state(false);
  let addServerDialogVisible = $state(false);
  let addServerDialogModule: Promise<
    typeof import('$lib/components/AddServerDialog.svelte')
  > | null = null;
  let authlingAvailable = $state(false);
  let authlingSync = $state<AccountDataSyncAPI | null>(null);
  let authlingError = $state('');
  let authlingProviderId = $state<string | null>(null);
  let connectingServerId = $state<string | null>(null);

  function loadAddServerDialog() {
    addServerDialogModule ??= import('$lib/components/AddServerDialog.svelte');
    return addServerDialogModule;
  }

  const canSubmit = $derived(identifier.trim() && password);
  const authProviders = $derived(data.serverInfo?.authProviders ?? []);
  const visibleAuthProviders = $derived(
    authProviders.filter((provider) => provider.id !== authlingProviderId)
  );
  const directRegistrationEnabled = $derived(data.serverInfo?.directRegistrationEnabled ?? true);
  const isAuthenticating = $derived(isLoading || selectedProviderId !== null);
  const pageError = $derived(
    pageErrorDismissed ? '' : loginErrorMessage(data.loginErrorCode || '')
  );
  const displayedError = $derived(error || pageError);
  const authlingStatus = $derived(authlingSync?.status ?? 'disconnected');
  const authlingAccountId = $derived(authlingSync?.accountId ?? null);
  const syncedServers = $derived(serverRegistry.servers.filter((server) => !server.token));

  // Standalone detection: if public server info failed to load, there is no local
  // backend to log in to. Redirect URLs are backend-driven flows, so keep the
  // login form visible while those complete or fail.
  const isStandalone = $derived(
    !data.serverInfo && data.serverInfoLoaded && data.redirectUrl === '/'
  );

  onMount(() => {
    void getClientConfiguration()
      .then(async (configuration) => {
        if (!configuration.authling) return;
        authlingAvailable = true;
        authlingProviderId = (await findAuthlingServerProvider(authProviders))?.id ?? null;
        const { accountDataSync } = await import('$lib/accountData/sync.svelte');
        authlingSync = accountDataSync;
        await authlingSync.initialize();
      })
      .catch((cause) => {
        console.error('[authling] invalid client configuration', cause);
      });
  });

  function providerIcon(type: string): string {
    switch (type) {
      case 'github':
        return 'icon-[mdi--github]';
      case 'gitlab':
        return 'icon-[mdi--gitlab]';
      case 'google':
        return 'icon-[mdi--google]';
      case 'discord':
        return 'icon-[mdi--discord]';
      default:
        return 'icon-[mdi--shield-account]';
    }
  }

  function providerLoginHref(provider: PublicAuthProvider): string {
    return `${provider.loginUrl}?redirect=${encodeURIComponent(data.redirectUrl)}`;
  }

  function loginErrorMessage(code: string): string {
    switch (code) {
      case 'provider_not_found':
        return m('auth.login.error.provider_not_found');
      case 'provider_failed':
        return m('auth.login.error.provider_failed');
      case 'provider_denied':
        return m('auth.login.error.provider_denied');
      case 'authentication_required':
        return m('auth.login.error.authentication_required');
      case 'external_identity_unlinked':
        return m('auth.login.error.external_identity_unlinked');
      case 'external_identity_conflict':
        return m('auth.login.error.external_identity_conflict');
      case 'invalid_invitation':
        return m('auth.login.error.invalid_invitation');
      default:
        return '';
    }
  }

  function handleProviderClick(e: MouseEvent, provider: PublicAuthProvider) {
    e.preventDefault();
    error = '';
    pageErrorDismissed = true;
    selectedProviderId = provider.id;
    window.setTimeout(() => {
      window.location.href = providerLoginHref(provider);
    }, 250);
  }

  async function handleAuthlingSignIn() {
    if (!authlingSync || authlingStatus === 'connecting') return;
    authlingError = '';
    await authlingSync.connect();
    if (authlingSync.status !== 'connected') {
      authlingError = authlingSync.error || m('chat.server_gutter.account_data_error');
      return;
    }

    if (!isStandalone) {
      const provider = await findAuthlingServerProvider(authProviders);
      if (!provider) {
        authlingError = m('auth.login.error.provider_not_found');
        return;
      }
      selectedProviderId = provider.id;
      window.location.href = providerLoginHref(provider);
    }
  }

  async function handleSyncedServerSignIn(server: RegisteredServer) {
    authlingError = '';
    connectingServerId = server.id;
    try {
      await startRemoteReauthentication(server);
    } catch (cause) {
      authlingError = cause instanceof Error ? cause.message : m('add_server.start_failed');
      connectingServerId = null;
    }
  }

  async function handleSubmit(e: Event) {
    e.preventDefault();
    error = '';
    pageErrorDismissed = true;
    isLoading = true;

    try {
      const response = await fetch('/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ identifier, password }),
        credentials: 'include'
      });

      const result = await response.json();

      if (!response.ok) {
        error = result.error || m('auth.login.failed');
        return;
      }

      if (typeof result.token !== 'string' || !result.token) {
        error = m('auth.login.missing_token');
        return;
      }

      const resumedReturnNavigation = await completeOriginAuthentication(
        result.token,
        result.user ?? null
      );
      if (!resumedReturnNavigation) {
        await navigateAfterAuthentication(data.redirectUrl);
      }
    } catch (err) {
      error = err instanceof Error ? err.message : m('auth.login.failed');
    } finally {
      isLoading = false;
    }
  }
</script>

{#snippet authlingSignIn()}
  {#if authlingAvailable}
    <div class="flex flex-col gap-3">
      <Button
        variant="action"
        size="lg"
        fullWidth
        onclick={handleAuthlingSignIn}
        disabled={authlingStatus === 'connecting' ||
          (authlingStatus === 'connected' && isStandalone)}
        loading={authlingStatus === 'connecting'}
        loadingText={m('auth.login.connecting_provider', { provider: 'Authling' })}
      >
        <span class="iconify icon-[mdi--shield-account] text-lg"></span>
        {#if authlingStatus === 'connected' && isStandalone}
          {m('chat.server_gutter.account_data_connected', { provider: 'Authling' })}
        {:else}
          {m('auth.login.continue_with_provider', { provider: 'Authling' })}
        {/if}
      </Button>
      {#if authlingAccountId}
        <p class="truncate text-center font-mono text-xs text-muted">
          Authling · {authlingAccountId}
        </p>
      {/if}
      {#if authlingError}
        <Hint tone="danger">{authlingError}</Hint>
      {/if}
    </div>
  {/if}
{/snippet}

<PageTitle title={isStandalone ? m('auth.login.welcome_page_title') : m('auth.login.title')} />

{#if isStandalone}
  <AuthLayout showBranding={false} centerContent>
    <div class="flex flex-col items-center text-center">
      <div
        class="mb-8 flex h-20 w-20 items-center justify-center rounded-xl border border-dashed border-action/50 bg-action/10 text-action"
        aria-hidden="true"
      >
        <span class="iconify icon-[mdi--server-plus] text-4xl"></span>
      </div>

      <div class="flex flex-col gap-3">
        <h1 class="text-3xl font-bold tracking-tight text-balance text-text-top">
          {m('auth.login.welcome_title')}
        </h1>
        <p class="text-pretty text-muted">
          {m('auth.login.welcome_description')}
        </p>
      </div>

      <div class="mt-8 w-full">
        {@render authlingSignIn()}

        {#if authlingAvailable}
          <div class="my-4">
            <Divider label={m('common.or')} />
          </div>
        {/if}

        <Button
          variant="action"
          size="lg"
          fullWidth
          onclick={() => (addServerDialogVisible = true)}
        >
          <span class="iconify icon-[mdi--plus] text-lg"></span>
          {m('auth.login.add_server')}
        </Button>
      </div>

      {#if authlingStatus === 'connected' && syncedServers.length > 0}
        <div class="mt-8 flex w-full flex-col gap-3 text-left">
          {#each syncedServers as server (server.id)}
            <div class="flex items-center gap-3 rounded-lg border border-border bg-surface p-3">
              <div class="min-w-0 flex-1">
                <div class="truncate font-semibold">{server.name}</div>
                <div class="truncate text-xs text-muted">{new URL(server.url).host}</div>
              </div>
              <Button
                variant="secondary"
                size="sm"
                onclick={() => handleSyncedServerSignIn(server)}
                loading={connectingServerId === server.id}
                disabled={connectingServerId !== null && connectingServerId !== server.id}
              >
                {m('add_server.sign_in')}
              </Button>
            </div>
          {/each}
        </div>
      {/if}

      <p class="mt-4 flex items-start gap-2 text-left text-sm text-muted">
        <span class="iconify mt-0.5 icon-[mdi--open-in-new] shrink-0 text-base" aria-hidden="true"
        ></span>
        <span>{m('auth.login.welcome_sign_in_hint')}</span>
      </p>
    </div>
  </AuthLayout>
{:else}
  <AuthLayout>
    <h1 class="mb-6 text-center text-2xl font-bold">{m('auth.login.title')}</h1>

    {#if data.passwordResetSuccess}
      <div class="mb-4">
        <Hint tone="success">
          {m('auth.login.password_reset_success')}
        </Hint>
      </div>
    {/if}

    {@render authlingSignIn()}

    {#if authlingAvailable}
      <Divider label={m('common.or')} />
    {/if}

    <!-- SSO providers -->
    {#if visibleAuthProviders.length > 0}
      <div class="flex flex-col gap-3">
        {#each visibleAuthProviders as provider (provider.id)}
          <Button
            variant="secondary"
            size="lg"
            fullWidth
            href={providerLoginHref(provider)}
            disabled={selectedProviderId !== null && selectedProviderId !== provider.id}
            loading={selectedProviderId === provider.id}
            loadingText={m('auth.login.connecting_provider', { provider: provider.label })}
            onclick={(e) => handleProviderClick(e, provider)}
          >
            <span class={['iconify text-lg', providerIcon(provider.type)]}></span>
            {m('auth.login.continue_with_provider', { provider: provider.label })}
          </Button>
        {/each}

        <Divider label={m('common.or')} />
      </div>
    {/if}

    <Form onsubmit={handleSubmit}>
      <TextInput
        id="identifier"
        label={m('auth.login.identifier_label')}
        bind:value={identifier}
        placeholder={m('common.email_placeholder')}
        disabled={isAuthenticating}
        required
        autocomplete="username"
        autofocus
      />

      <TextInput
        id="password"
        label={m('common.password')}
        type="password"
        bind:value={password}
        placeholder={m('common.password_placeholder')}
        disabled={isAuthenticating}
        required
        autocomplete="current-password"
      />

      {#if displayedError}
        <Hint tone="danger">{displayedError}</Hint>
      {/if}

      <Button
        type="submit"
        size="lg"
        disabled={!canSubmit || isAuthenticating}
        loading={isLoading}
        loadingText={m('auth.login.signing_in')}
      >
        <span class="iconify icon-[mdi--login]"></span>
        {m('common.sign_in')}
      </Button>
    </Form>

    <div class="mt-4 text-center">
      <a href={resolve('/forgot-password')} class="link">{m('auth.login.forgot_password')}</a>
    </div>

    {#if directRegistrationEnabled}
      <Divider label={m('common.or')} />

      <Button href={resolve('/register')} variant="secondary" size="lg" fullWidth>
        {m('common.create_account')}
      </Button>
    {/if}
  </AuthLayout>
{/if}

{#if addServerDialogVisible}
  {#await loadAddServerDialog() then { default: AddServerDialog }}
    <AddServerDialog
      bind:visible={addServerDialogVisible}
      onclose={() => (addServerDialogVisible = false)}
    />
  {/await}
{/if}
