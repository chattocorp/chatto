<!-- @component
Desktop sign-in for the selected server. Provider buttons start the server's
OAuth flow with the selected provider; passwords use the native host request.
-->
<script lang="ts">
  import { onDestroy } from 'svelte';
  import type { RegisteredServer } from '@chatto/client/server/registry';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { signInWithPassword } from '$lib/desktop/passwordLogin';
  import {
    closeInlineSignIn,
    isRemoteSignInPending,
    startRemoteSignIn
  } from '$lib/auth/remoteSignIn.svelte';
  import { m } from '$lib/i18n/messages';
  import { Button, Form, TextInput } from '$lib/ui/form';
  import { Divider, Hint, PageTitle } from '$lib/ui';
  import AuthLayout from '$lib/components/AuthLayout.svelte';
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
  const busy = $derived(submitting || isRemoteSignInPending(registration.id));
  const providers = $derived(store.serverInfo.authProviders ?? []);
  const showPassword = $derived(!store.serverInfo.loading && store.serverInfo.directLoginEnabled);
  const showRegistration = $derived(
    !store.serverInfo.loading && store.serverInfo.directRegistrationEnabled
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
<div class="pane-page" data-testid="desktop-server-login">
  <AuthLayout serverInfo={branding} title={m('auth.login.title')} titleOnlyWithSideBranding>
    <div class="w-full text-start">
      {#if providers.length > 0}
        <div class="flex flex-col gap-3">
          {#each providers as provider (provider.id)}
            <Button
              fullWidth
              size="lg"
              variant="secondary"
              disabled={busy}
              loading={selectedProviderId === provider.id && isRemoteSignInPending(registration.id)}
              loadingText={m('auth.login.connecting_provider', { provider: provider.label })}
              onclick={() => void signInOnServer(provider.id)}
            >
              <span aria-hidden="true" class="iconify icon-[mdi--shield-account] text-lg"></span>
              {m('auth.login.continue_with_provider', { provider: provider.label })}
            </Button>
          {/each}
        </div>
        {#if showPassword}<Divider label={m('common.or')} />{/if}
      {/if}
      {#if showPassword}
        <Form onsubmit={submit}>
          <TextInput
            id="desktop-identifier"
            label={m('auth.login.identifier_label')}
            bind:value={identifier}
            autocomplete="username"
            placeholder={m('common.email_placeholder')}
            required
            disabled={busy}
          />
          <TextInput
            id="desktop-password"
            label={m('common.password')}
            type="password"
            bind:value={password}
            autocomplete="current-password"
            placeholder={m('common.password_placeholder')}
            required
            disabled={busy}
          />
          {#if failed}<Hint tone="danger">{m('auth.login.failed')}</Hint>{/if}
          <Button
            type="submit"
            size="lg"
            fullWidth
            loading={submitting}
            loadingText={m('auth.login.signing_in')}
            disabled={busy || !identifier.trim() || !password}
          >
            <span aria-hidden="true" class="iconify icon-[mdi--login]"></span>
            {m('common.sign_in')}
          </Button>
        </Form>
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
