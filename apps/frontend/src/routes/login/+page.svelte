<!--
@component
Origin sign-in page. Without SSO providers, it shows the password form. With
SSO providers, it shows only the provider choices, plus a link to the password
login page when the server allows direct login.
-->
<script lang="ts">
  import { confirmPendingAtprotoHandle } from '$lib/atproto/rememberedHandle';
  import { resolve } from '$app/paths';
  import {
    confirmProviderAccount,
    ProviderSignInError,
    type ProviderSignInResult
  } from '$lib/auth/providerSignIn';
  import { navigateAfterAuthentication } from '$lib/auth/returnNavigation';
  import { withReturnPath } from '$lib/auth/loginRoutes';
  import OriginProviderButtons from '$lib/components/auth/OriginProviderButtons.svelte';
  import PasswordSignInSection from '$lib/components/auth/PasswordSignInSection.svelte';
  import { popupErrorMessage } from '$lib/oauth/popupError';
  import { isProviderContinuation } from '$lib/auth/providerContinuation';
  import AuthLayout from '$lib/components/AuthLayout.svelte';
  import { m } from '$lib/i18n/messages';
  import type { PublicAuthProvider } from '@chatto/client/api/server';
  import { Divider, Hint, PageTitle } from '$lib/ui';
  import { Button } from '$lib/ui/form';

  const { data } = $props();

  let error = $state('');
  let selectedProviderId = $state<string | null>(null);
  let pageErrorDismissed = $state(false);

  const compact = $derived(data.redirectUrl.startsWith('/oauth/'));

  const authProviders = $derived(data.serverInfo?.authProviders ?? []);
  const directRegistrationEnabled = $derived(data.serverInfo?.directRegistrationEnabled ?? true);
  const directLoginEnabled = $derived(data.serverInfo?.directLoginEnabled ?? true);
  const pageError = $derived(
    pageErrorDismissed ? '' : loginErrorMessage(data.loginErrorCode || '')
  );
  const displayedError = $derived(error || pageError);
  /** This page offers account creation through direct registration or provider auto-provisioning. */
  const inviteOnlyNotice = $derived(
    data.serverInfo?.accountCreationPolicy === 'invite_only' &&
      (authProviders.length === 0
        ? directRegistrationEnabled && directLoginEnabled
        : authProviders.some((provider) => provider.autoProvision === true))
  );

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

  {#if inviteOnlyNotice}
    <p class="mb-4 text-muted">{m('auth.login.invitation_notice')}</p>
  {/if}

  {#if authProviders.length === 0}
    {#if directLoginEnabled}
      <PasswordSignInSection
        redirectUrl={data.redirectUrl}
        {directRegistrationEnabled}
        {compact}
        {pageError}
      />
    {:else if pageError}
      <Hint tone="danger">{pageError}</Hint>
    {/if}
  {:else}
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

    {#if displayedError}
      <div class="mt-4"><Hint tone="danger">{displayedError}</Hint></div>
    {/if}

    {#if directLoginEnabled}
      <Divider label={m('common.or')} />

      <Button
        href={withReturnPath(resolve('/login/password'), data.redirectUrl)}
        variant="secondary"
        size={compact ? 'md' : 'lg'}
        fullWidth
        disabled={selectedProviderId !== null}
      >
        <span aria-hidden="true" class="iconify icon-[uil--lock] text-lg"></span>
        {m('auth.login.continue_with_password')}
      </Button>
    {/if}
  {/if}
</AuthLayout>
