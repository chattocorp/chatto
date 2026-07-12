<script lang="ts">
  import { goto, invalidateAll } from '$app/navigation';
  import { resolve } from '$app/paths';
  import AuthLayout from '$lib/components/AuthLayout.svelte';
  import * as m from '$lib/i18n/messages';
  import { isSafeInternalPath } from '$lib/navigation/safeInternalPath';
  import type { AuthenticatedUserSummary } from '$lib/state/server/registry.svelte';
  import type { PublicAuthProvider } from '$lib/api-client/server';
  import Divider from '$lib/ui/Divider.svelte';
  import Hint from '$lib/ui/Hint.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { TextInput, Button, Form } from '$lib/ui/form';

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

  function loadAddServerDialog() {
    addServerDialogModule ??= import('$lib/components/AddServerDialog.svelte');
    return addServerDialogModule;
  }

  const canSubmit = $derived(identifier.trim() && password);
  const authProviders = $derived(data.serverInfo?.authProviders ?? []);
  const directRegistrationEnabled = $derived(data.serverInfo?.directRegistrationEnabled ?? true);
  const isAuthenticating = $derived(isLoading || selectedProviderId !== null);
  const pageError = $derived(
    pageErrorDismissed ? '' : loginErrorMessage(data.loginErrorCode || '')
  );
  const displayedError = $derived(error || pageError);

  // Standalone detection: if public server info failed to load, there is no local
  // backend to log in to. Redirect URLs are backend-driven flows, so keep the
  // login form visible while those complete or fail.
  const isStandalone = $derived(
    !data.serverInfo && data.serverInfoLoaded && data.redirectUrl === '/'
  );

  /**
   * Navigate after a successful login. Uses `window.location.href` for backend
   * routes (e.g. `/oauth/authorize`) that are served by Gin, not SvelteKit.
   * Falls back to `/` for any URL that isn't a same-origin path — this is the
   * last line of defence against an open-redirect via `?redirect=` or
   * sessionStorage tampering.
   */
  function navigateAfterLogin(url: string) {
    const target = isSafeInternalPath(url) ? url : '/';
    if (target.startsWith('/oauth/')) {
      window.location.href = target;
    } else {
      // eslint-disable-next-line svelte/no-navigation-without-resolve -- target is validated by isSafeInternalPath; backend routes are handled above
      goto(target);
    }
  }

  function providerIcon(type: string): string {
    switch (type) {
      case 'github':
        return 'mdi--github';
      case 'gitlab':
        return 'mdi--gitlab';
      case 'google':
        return 'mdi--google';
      case 'discord':
        return 'mdi--discord';
      default:
        return 'mdi--shield-account';
    }
  }

  function providerLoginHref(provider: PublicAuthProvider): string {
    return `${provider.loginUrl}?redirect=${encodeURIComponent(data.redirectUrl)}`;
  }

  function loginErrorMessage(code: string): string {
    switch (code) {
      case 'provider_not_found':
        return m['auth.login.error.provider_not_found']();
      case 'provider_failed':
        return m['auth.login.error.provider_failed']();
      case 'provider_denied':
        return m['auth.login.error.provider_denied']();
      case 'authentication_required':
        return m['auth.login.error.authentication_required']();
      case 'external_identity_unlinked':
        return m['auth.login.error.external_identity_unlinked']();
      case 'external_identity_conflict':
        return m['auth.login.error.external_identity_conflict']();
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

  async function authenticateOrigin(
    token: string,
    user: AuthenticatedUserSummary | null
  ): Promise<void> {
    const [{ serverRegistry }, { clearCachedUser }] = await Promise.all([
      import('$lib/state/server/registry.svelte'),
      import('$lib/auth/loadAuth')
    ]);
    serverRegistry.authenticateOrigin(token, user);
    clearCachedUser();
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
        error = result.error || m['auth.login.failed']();
        return;
      }

      if (typeof result.token !== 'string' || !result.token) {
        error = m['auth.login.missing_token']();
        return;
      }

      await authenticateOrigin(result.token, result.user ?? null);
      await invalidateAll();

      const returnUrl = sessionStorage.getItem('returnUrl');
      if (returnUrl) {
        // Keep the marker until the authenticated chat shell sees it; otherwise
        // the chat landing redirect can win before the return URL settles.
        navigateAfterLogin(returnUrl);
      } else {
        navigateAfterLogin(data.redirectUrl);
      }
    } catch (err) {
      error = err instanceof Error ? err.message : m['auth.login.failed']();
    } finally {
      isLoading = false;
    }
  }
</script>

<PageTitle title={isStandalone ? m['auth.login.welcome_page_title']() : m['auth.login.title']()} />

{#if isStandalone}
  <AuthLayout>
    <div class="flex flex-col items-center gap-6 text-center">
      <h1 class="text-2xl font-bold">{m['auth.login.welcome_title']()}</h1>
      <p class="text-muted">
        {m['auth.login.welcome_description']()}
      </p>
      <Button variant="accent" size="lg" fullWidth onclick={() => (addServerDialogVisible = true)}>
        {m['auth.login.add_server']()}
      </Button>
    </div>
  </AuthLayout>
{:else}
  <AuthLayout>
    <h1 class="mb-6 text-center text-2xl font-bold">{m['auth.login.title']()}</h1>

    {#if data.passwordResetSuccess}
      <div class="mb-4">
        <Hint tone="success">
          {m['auth.login.password_reset_success']()}
        </Hint>
      </div>
    {/if}

    <!-- SSO providers -->
    {#if authProviders.length > 0}
      <div class="flex flex-col gap-3">
        {#each authProviders as provider (provider.id)}
          <Button
            variant="secondary"
            size="lg"
            fullWidth
            href={providerLoginHref(provider)}
            disabled={selectedProviderId !== null && selectedProviderId !== provider.id}
            loading={selectedProviderId === provider.id}
            loadingText={m['auth.login.connecting_provider']({ provider: provider.label })}
            onclick={(e) => handleProviderClick(e, provider)}
          >
            <span class={['iconify text-lg', providerIcon(provider.type)]}></span>
            {m['auth.login.continue_with_provider']({ provider: provider.label })}
          </Button>
        {/each}

        <Divider label={m['common.or']()} />
      </div>
    {/if}

    <Form onsubmit={handleSubmit}>
      <TextInput
        id="identifier"
        label={m['auth.login.identifier_label']()}
        bind:value={identifier}
        placeholder={m['common.email_placeholder']()}
        disabled={isAuthenticating}
        required
        autocomplete="username"
        autofocus
      />

      <TextInput
        id="password"
        label={m['common.password']()}
        type="password"
        bind:value={password}
        placeholder={m['common.password_placeholder']()}
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
        loadingText={m['auth.login.signing_in']()}
      >
        <span class="iconify mdi--login"></span>
        {m['common.sign_in']()}
      </Button>
    </Form>

    <div class="mt-4 text-center">
      <a href={resolve('/forgot-password')} class="link">{m['auth.login.forgot_password']()}</a>
    </div>

    {#if directRegistrationEnabled}
      <Divider label={m['common.or']()} />

      <a href={resolve('/register')} class="btn-secondary block w-full btn-lg text-center">
        {m['common.create_account']()}
      </a>
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
