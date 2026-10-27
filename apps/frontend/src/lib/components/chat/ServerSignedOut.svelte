<!-- @component
Shared sign-in for a remote server. Browsers use a server-hosted authorization
window; an optional native host capability enables inline password submission.
-->
<script lang="ts">
  import { onDestroy } from 'svelte';
  import type { RegisteredServer } from '@chatto/client/server/registry';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { hasPasswordLogin, signInWithPassword } from '$lib/desktop/passwordLogin';
  import {
    closeInlineSignIn,
    isRemoteSignInPending,
    startRemoteSignIn
  } from '$lib/auth/remoteSignIn.svelte';
  import { m } from '$lib/i18n/messages';
  import AuthProviderButtons from '$lib/components/auth/AuthProviderButtons.svelte';
  import PasswordSignInForm from '$lib/components/auth/PasswordSignInForm.svelte';
  import AuthPopupNotice from '$lib/components/auth/AuthPopupNotice.svelte';
  import { Button } from '$lib/ui/form';
  import { Divider, PageTitle } from '$lib/ui';
  import AuthLayout from '$lib/components/AuthLayout.svelte';
  import { serverHost } from '$lib/serverUrl';
  import { serverDisplayName } from '@chatto/client/server/state';

  let { registration }: { registration: RegisteredServer } = $props();
  const { store, serverId } = useServerScope();
  const lifetime = new AbortController();
  onDestroy(() => {
    lifetime.abort();
    closeInlineSignIn(serverId);
  });
  let identifier = $state('');
  let password = $state('');
  let submitting = $state(false);
  let failed = $state(false);
  let selectedProviderId = $state<string | null>(null);
  let signingUp = $state(false);
  const inlinePasswordLogin = hasPasswordLogin();
  const busy = $derived(submitting || isRemoteSignInPending(registration.id));
  const providers = $derived(store.serverInfo.authProviders ?? []);
  const showPassword = $derived(
    inlinePasswordLogin && !store.serverInfo.loading && store.serverInfo.directLoginEnabled
  );
  const showRegistration = $derived(
    inlinePasswordLogin && !store.serverInfo.loading && store.serverInfo.directRegistrationEnabled
  );
  const branding = $derived({
    name: serverDisplayName(store.serverInfo, registration.name),
    iconUrl: store.serverInfo.iconUrl ?? registration.iconUrl,
    bannerUrl: store.serverInfo.bannerUrl,
    description: store.serverInfo.description,
    welcomeMessage: store.serverInfo.welcomeMessage
  });

  async function signInOnServer(providerId?: string, signup = false) {
    if (busy) return;
    password = '';
    failed = false;
    selectedProviderId = providerId ?? null;
    signingUp = signup;
    try {
      await startRemoteSignIn(registration, { useOAuth: true, providerId, signup });
    } finally {
      selectedProviderId = null;
      signingUp = false;
    }
  }

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    if (busy || !identifier.trim() || !password) return;
    submitting = true;
    failed = false;
    const submittedPassword = password;
    password = '';
    try {
      await signInWithPassword(registration, identifier.trim(), submittedPassword, lifetime.signal);
      closeInlineSignIn(registration.id);
    } catch {
      if (!lifetime.signal.aborted) failed = true;
    } finally {
      submitting = false;
    }
  }
</script>

<PageTitle title={m('auth.login.title')} />
<div
  class="pane-page"
  data-testid={inlinePasswordLogin ? 'desktop-server-login' : 'server-signed-out'}
>
  <AuthLayout serverInfo={branding} title={m('auth.login.title')} titleOnlyWithSideBranding>
    <div class="w-full text-start">
      {#if !inlinePasswordLogin}
        <p class="mb-6 text-center text-muted">
          {m('auth.remote.popup_notice', { server: serverHost(registration.url) })}
        </p>
        <Button
          fullWidth
          size="lg"
          disabled={busy}
          loading={busy && !signingUp}
          onclick={() => void signInOnServer()}
        >
          <span aria-hidden="true" class="iconify icon-[mdi--open-in-new]"></span>
          {m('auth.remote.continue_on_server')}
        </Button>
      {/if}
      {#if inlinePasswordLogin && providers.length > 0}
        <AuthProviderButtons
          {providers}
          {selectedProviderId}
          disabled={busy}
          onclick={(_event, provider) => void signInOnServer(provider.id)}
        />
        {#if showPassword}<Divider label={m('common.or')} />{/if}
      {/if}
      {#if showPassword}
        <PasswordSignInForm
          bind:identifier
          bind:password
          disabled={busy}
          loading={submitting}
          error={failed ? m('auth.login.failed') : ''}
          onsubmit={submit}
        />
      {/if}
      {#if showRegistration}
        {#if showPassword || providers.length > 0}<Divider label={m('common.or')} />{/if}
        <Button
          variant="secondary"
          size="lg"
          fullWidth
          disabled={busy}
          loading={signingUp}
          onclick={() => void signInOnServer(undefined, true)}
        >
          {m('common.create_account')}
        </Button>
      {/if}
      {#if inlinePasswordLogin}
        <div class="mt-6 text-center">
          <button
            type="button"
            class="cursor-pointer text-sm text-muted hover:text-text hover:underline focus-visible:underline disabled:cursor-default disabled:opacity-50"
            title={m('auth.desktop.server_login_hint')}
            disabled={busy}
            aria-busy={!signingUp &&
              selectedProviderId === null &&
              isRemoteSignInPending(registration.id)}
            onclick={() => void signInOnServer()}
          >
            {m('auth.desktop.server_login')}
          </button>
        </div>
      {/if}
      <AuthPopupNotice pending={isRemoteSignInPending(registration.id)} />
    </div>
    {#if store.realtimeSync.hasDisplayableView}
      <div class="mt-4 text-center">
        <Button variant="secondary" onclick={() => closeInlineSignIn(registration.id)}>
          {m('common.cancel')}
        </Button>
      </div>
    {/if}
  </AuthLayout>
</div>
