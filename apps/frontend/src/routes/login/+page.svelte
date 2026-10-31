<script lang="ts">
  import { errorMessage } from '$lib/utils/errorMessage';
  import { resolve } from '$app/paths';
  import { onDestroy } from 'svelte';
  import { openProviderSignIn, verifyProviderSignIn } from '$lib/auth/providerSignIn';
  import type { OAuthPopup } from '$lib/oauth/popup';
  import { browserCookieAuthenticationHeaders } from '@chatto/client/auth/authenticationMode';
  import { navigateAfterAuthentication } from '$lib/auth/returnNavigation';
  import AuthProviderButtons from '$lib/components/auth/AuthProviderButtons.svelte';
  import AuthPopupNotice from '$lib/components/auth/AuthPopupNotice.svelte';
  import { popupErrorMessage } from '$lib/oauth/popupError';
  import { isProviderContinuation } from '$lib/auth/providerContinuation';
  import AuthLayout from '$lib/components/AuthLayout.svelte';
  import { m } from '$lib/i18n/messages';
  import type { PublicAuthProvider } from '@chatto/client/api/server';
  import { Divider, Hint, PageTitle } from '$lib/ui';
  import PasswordSignInForm from '$lib/components/auth/PasswordSignInForm.svelte';
  import { Button } from '$lib/ui/form';

  const { data } = $props();

  let identifier = $state('');
  let password = $state('');
  let error = $state('');
  let isLoading = $state(false);
  let selectedProviderId = $state<string | null>(null);
  let pageErrorDismissed = $state(false);
  let providerPopup: OAuthPopup | null = null;
  let active = true;
  onDestroy(() => {
    active = false;
    providerPopup?.close();
  });

  const compact = $derived(data.redirectUrl.startsWith('/oauth/'));

  const authProviders = $derived(data.serverInfo?.authProviders ?? []);
  const directRegistrationEnabled = $derived(data.serverInfo?.directRegistrationEnabled ?? true);
  const registrationAvailable = $derived(
    directRegistrationEnabled ||
      (data.serverInfo?.authProviders ?? []).some((provider) => provider.autoProvision !== false)
  );
  const directLoginEnabled = $derived(data.serverInfo?.directLoginEnabled ?? true);
  const isAuthenticating = $derived(isLoading || selectedProviderId !== null);
  const pageError = $derived(
    pageErrorDismissed ? '' : loginErrorMessage(data.loginErrorCode || '')
  );
  const displayedError = $derived(error || pageError);

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
      case 'provider_access_denied':
        return m('auth.login.error.provider_access_denied');
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

  async function handleProviderClick(e: MouseEvent, provider: PublicAuthProvider) {
    e.preventDefault();
    error = '';
    pageErrorDismissed = true;
    selectedProviderId = provider.id;
    // Remote authorization and existing-account linking already own this window.
    // Keep their continuation here instead of opening another provider popup.
    if (compact || isProviderContinuation(data.redirectUrl)) {
      window.location.href = providerLoginHref(provider);
      return;
    }
    try {
      providerPopup = openProviderSignIn(provider.loginUrl);
      await verifyProviderSignIn(providerPopup);
      if (!active) return;
      const { completeOriginAuthentication } = await import('$lib/auth/originAuthentication');
      const resumed = await completeOriginAuthentication();
      if (!resumed) await navigateAfterAuthentication(data.redirectUrl);
    } catch (err) {
      if (active) error = popupErrorMessage(err, m('auth.login.failed'));
    } finally {
      providerPopup = null;
      selectedProviderId = null;
    }
  }

  async function handleSubmit(e: Event) {
    e.preventDefault();
    error = '';
    pageErrorDismissed = true;
    isLoading = true;

    try {
      const response = await fetch('/auth/browser/login', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...browserCookieAuthenticationHeaders
        },
        body: JSON.stringify({ identifier, password }),
        credentials: 'include'
      });

      const result = await response.json();

      if (!response.ok) {
        error = result.error || m('auth.login.failed');
        return;
      }

      // Session completion is only needed after a successful password login.
      const { completeOriginAuthentication } = await import('$lib/auth/originAuthentication');
      const resumedReturnNavigation = await completeOriginAuthentication({
        resumeReturnNavigation: !isProviderContinuation(data.redirectUrl)
      });
      if (!resumedReturnNavigation) {
        await navigateAfterAuthentication(data.redirectUrl);
      }
    } catch (err) {
      error = errorMessage(err, m('auth.login.failed'));
    } finally {
      isLoading = false;
    }
  }
</script>

<PageTitle title={m('auth.login.title')} />

<AuthLayout {compact} title={m('auth.login.title')}>
  {#if data.passwordResetSuccess}
    <div class="mb-4">
      <Hint tone="success">
        {m('auth.login.password_reset_success')}
      </Hint>
    </div>
  {/if}

  <!-- SSO providers -->
  {#if authProviders.length > 0}
    <AuthProviderButtons
      providers={authProviders}
      {selectedProviderId}
      {compact}
      href={providerLoginHref}
      onclick={handleProviderClick}
    />
    <AuthPopupNotice pending={!compact && selectedProviderId !== null} />
    {#if directLoginEnabled}<Divider label={m('common.or')} />{/if}
  {/if}

  {#if !directLoginEnabled && displayedError}
    <Hint tone="danger">{displayedError}</Hint>
  {/if}

  {#if directLoginEnabled}
    <PasswordSignInForm
      bind:identifier
      bind:password
      disabled={isAuthenticating}
      loading={isLoading}
      error={displayedError}
      {compact}
      autofocus
      onsubmit={handleSubmit}
    />

    <div class="mt-4 text-center">
      <a href={resolve('/forgot-password')} class="link">{m('auth.login.forgot_password')}</a>
    </div>
  {/if}

  {#if registrationAvailable}
    <Divider label={m('common.or')} />

    <Button
      href={resolve('/register') +
        (data.redirectUrl === '/' ? '' : `?${new URLSearchParams({ redirect: data.redirectUrl })}`)}
      variant="secondary"
      size={compact ? 'md' : 'lg'}
      fullWidth
    >
      {m('common.create_account')}
    </Button>
  {/if}
</AuthLayout>
