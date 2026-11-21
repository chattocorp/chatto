<!--
@component
Password login page. The origin login page links here when the server offers
SSO providers and direct login. Holds the password form, the password-reset
link, and the Create Account action.
-->
<script lang="ts">
  import { resolve } from '$app/paths';
  import { withReturnPath } from '$lib/auth/loginRoutes';
  import AuthLayout from '$lib/components/AuthLayout.svelte';
  import PasswordSignInSection from '$lib/components/auth/PasswordSignInSection.svelte';
  import { m } from '$lib/i18n/messages';
  import { Hint, PageTitle } from '$lib/ui';

  const { data } = $props();

  const compact = $derived(data.redirectUrl.startsWith('/oauth/'));
  const directRegistrationEnabled = $derived(data.serverInfo?.directRegistrationEnabled ?? true);
  const hasProviders = $derived((data.serverInfo?.authProviders.length ?? 0) > 0);
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

  {#if data.serverInfo?.accountCreationPolicy === 'invite_only' && directRegistrationEnabled}
    <p class="mb-4 text-muted">{m('auth.login.invitation_notice')}</p>
  {/if}

  <PasswordSignInSection redirectUrl={data.redirectUrl} {directRegistrationEnabled} {compact} />

  {#if hasProviders}
    <div class="mt-6 text-center">
      <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- the path is resolved; withReturnPath only adds a query string -->
      <a href={withReturnPath(resolve('/login'), data.redirectUrl)} class="link">
        {m('auth.login.other_options')}
      </a>
    </div>
  {/if}
</AuthLayout>
