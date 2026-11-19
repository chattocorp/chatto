<script lang="ts">
  import { confirmPendingAtprotoHandle } from '$lib/atproto/rememberedHandle';
  import { errorMessage } from '$lib/utils/errorMessage';
  import { resolve } from '$app/paths';
  import {
    confirmProviderAccount,
    ProviderSignInError,
    type ProviderSignInResult
  } from '$lib/auth/providerSignIn';
  import { signInWithPassword } from '$lib/auth/passwordSignIn';
  import { navigateAfterAuthentication } from '$lib/auth/returnNavigation';
  import OriginProviderButtons from '$lib/components/auth/OriginProviderButtons.svelte';
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

  const compact = $derived(data.redirectUrl.startsWith('/oauth/'));

  const authProviders = $derived(data.serverInfo?.authProviders ?? []);
  const directRegistrationEnabled = $derived(data.serverInfo?.directRegistrationEnabled ?? true);
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

  async function completeProviderSignIn(result: ProviderSignInResult) {
    confirmPendingAtprotoHandle();
    if (result.kind === 'confirm-account') {
      await confirmProviderAccount(result.token, data.redirectUrl);
      return;
    }
    const { completeOriginAuthentication } = await import('$lib/auth/originAuthentication');
    const resumed = await completeOriginAuthentication();
    if (!resumed) await navigateAfterAuthentication(data.redirectUrl);
  }

  async function handleSubmit(e: Event) {
    e.preventDefault();
    error = '';
    pageErrorDismissed = true;
    isLoading = true;

    try {
      await signInWithPassword(identifier, password, m('auth.login.failed'));

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

  {#if data.serverInfo?.accountCreationPolicy === 'invite_only' && (directRegistrationEnabled || authProviders.some((provider) => provider.autoProvision === true))}
    <p class="mb-4 text-muted">{m('auth.login.invitation_notice')}</p>
  {/if}

  <!-- SSO providers -->
  {#if authProviders.length > 0}
    <OriginProviderButtons
      providers={authProviders}
      bind:selectedProviderId
      {compact}
      href={providerLoginHref}
      popupUrl={compact || isProviderContinuation(data.redirectUrl)
        ? undefined
        : (provider) => provider.loginUrl}
      onstart={() => {
        error = '';
        pageErrorDismissed = true;
      }}
      onresult={completeProviderSignIn}
      onerror={(err) => {
        error =
          (err instanceof ProviderSignInError && loginErrorMessage(err.code)) ||
          popupErrorMessage(err, m('auth.login.failed'));
      }}
    />
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

  {#if directRegistrationEnabled}
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
