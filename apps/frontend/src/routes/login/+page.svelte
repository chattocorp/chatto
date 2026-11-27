<!--
@component
Origin sign-in page. Without SSO providers, it shows the password form. With
SSO providers, it shows the provider choices, plus a link to the password
login page when the server allows direct login.
-->
<script lang="ts">
  import { resolve } from '$app/paths';
  import { returnPathQuery } from '$lib/auth/loginRoutes';
  import OriginProviderButtons from '$lib/components/auth/OriginProviderButtons.svelte';
  import RegistrationLink from '$lib/components/auth/RegistrationLink.svelte';
  import PasswordSignInSection from '$lib/components/auth/PasswordSignInSection.svelte';
  import AuthLayout from '$lib/components/AuthLayout.svelte';
  import { m } from '$lib/i18n/messages';
  import type { PublicAuthProvider } from '@chatto/client/api/server';
  import { Divider, Hint, PageTitle } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import { isIdentityLinkPath } from '$lib/auth/identityLink';

  const { data } = $props();

  let selectedProviderId = $state<string | null>(null);

  // Sign-in windows show a compact page and continue provider sign-in in place.
  const compact = $derived(
    data.redirectUrl.startsWith('/oauth/') || isIdentityLinkPath(data.redirectUrl)
  );

  const authProviders = $derived(data.serverInfo?.authProviders ?? []);
  const directRegistrationEnabled = $derived(data.serverInfo?.directRegistrationEnabled ?? true);
  const directLoginEnabled = $derived(data.serverInfo?.directLoginEnabled ?? true);
  const pageError = $derived(loginErrorMessage(data.loginErrorCode || ''));
  const canCreateAccount = $derived(
    directRegistrationEnabled || authProviders.some((provider) => provider.autoProvision === true)
  );
  const inviteOnlyNotice = $derived(
    data.serverInfo?.accountCreationPolicy === 'invite_only' && canCreateAccount
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
</script>

<PageTitle title={m('auth.login.title')} />

<AuthLayout
  {compact}
  title={authProviders.length
    ? canCreateAccount
      ? m('auth.login.sign_in_or_create')
      : m('auth.login.choose_method')
    : m('auth.login.title')}
>
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
      <PasswordSignInSection redirectUrl={data.redirectUrl} {compact} {pageError} />
    {:else if pageError}
      <Hint tone="danger">{pageError}</Hint>
    {/if}
  {:else}
    <OriginProviderButtons
      providers={authProviders}
      showRestrictions={canCreateAccount}
      bind:selectedProviderId
      {compact}
      href={providerLoginHref}
    />

    {#if pageError}
      <div class="mt-4"><Hint tone="danger">{pageError}</Hint></div>
    {/if}

    {#if directLoginEnabled}
      <Divider label={m('common.or')} />

      <Button
        href={resolve(`/login/password${returnPathQuery(data.redirectUrl)}`)}
        variant="secondary"
        size={compact ? 'md' : 'lg'}
        fullWidth
        disabled={selectedProviderId !== null}
      >
        <span aria-hidden="true" class="iconify icon-[uil--lock] text-lg"></span>
        {m('common.password')}
      </Button>
    {/if}
  {/if}
  <RegistrationLink serverInfo={data.serverInfo} redirectUrl={data.redirectUrl} />
</AuthLayout>
