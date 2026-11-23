<!--
@component
The identity-link window: links a sign-in provider to the signed-in account in
the window that account settings opened. The user confirms the link. When the
server requires fresh proof, the page asks for the current password. Then it
sends the window to the provider and receives the provider callback. When
only a new provider sign-in gives fresh proof, the page explains that the user
must first sign in again with a provider that is already linked. After that
re-verification, it starts the link by itself. After a
successful link, it tells account settings and closes the window. Errors offer
a way out: try again when that can help, and close the window.
`$lib/auth/identityLink` defines the URL contract.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import AuthLayout from '$lib/components/AuthLayout.svelte';
  import ReverificationChoice from '$lib/components/auth/ReverificationChoice.svelte';
  import {
    reverificationErrorMessage,
    reverificationProviders,
    reverificationStartUrl
  } from '$lib/auth/freshAuthReverification';
  import { repeatDiscardedRead } from '$lib/api/repeatDiscardedRead';
  import {
    createExternalIdentityAPI,
    type ExternalIdentityProviderInfo
  } from '$lib/api/externalIdentities';
  import {
    announceProviderLinked,
    identityLinkErrorMessage,
    identityLinkPath,
    identityLinkReturnPath,
    linkStartFailure,
    parseIdentityLinkRequest
  } from '$lib/auth/identityLink';
  import { serverConnectionManager, serverRegistry } from '$lib/client';
  import { leavePage } from '$lib/navigation/leavePage';
  import { m } from '$lib/i18n/messages';
  import { Hint, LoadingDots, PageTitle } from '$lib/ui';
  import { Button, TextInput } from '$lib/ui/form';
  import { errorMessage } from '$lib/utils/errorMessage';

  let { data } = $props();

  const request = parseIdentityLinkRequest(page.url);

  let provider = $state.raw<ExternalIdentityProviderInfo | null>(null);
  let view = $state<'loading' | 'confirm' | 'password' | 'reverify' | 'linked' | 'error'>(
    'loading'
  );
  // Linked providers that can confirm a recent sign-in before the link.
  let reverifiers = $state.raw<ExternalIdentityProviderInfo[]>([]);
  let reverifyError = $state('');
  let reverifyingProviderId = $state('');
  let error = $state<{ message: string; retryable: boolean }>({ message: '', retryable: false });
  let password = $state('');
  let passwordError = $state('');
  let submitting = $state(false);

  function externalIdentityAPI() {
    const origin = serverRegistry.originServer;
    if (!origin) throw new Error('origin server is not registered');
    return serverConnectionManager.getClient(origin.id).getAPI(createExternalIdentityAPI);
  }

  /** Show an error. A retryable error offers to start the link again. */
  function fail(message: string, retryable = false) {
    error = { message, retryable };
    view = 'error';
  }

  onMount(async () => {
    if (!request.providerId || !request.userId) {
      fail(m('auth.sso.invalid'));
      return;
    }
    if (request.userId !== data.user.id) {
      fail(m('settings.account.sso.account_mismatch'));
      return;
    }
    let loadError: unknown = null;
    try {
      // This page can read before the origin's first realtime snapshot.
      const list = await repeatDiscardedRead(() => externalIdentityAPI().list());
      provider = list.providers.find((candidate) => candidate.id === request.providerId) ?? null;
      reverifiers = reverificationProviders(list.providers);
    } catch (err) {
      loadError = err;
    }
    // The callback error wins. The provider, when it loaded, only names the page.
    if (request.completed && request.errorCode) {
      fail(identityLinkErrorMessage(request.errorCode), true);
      return;
    }
    if (loadError) {
      fail(errorMessage(loadError, m('settings.account.sso.load_failed')), true);
      return;
    }
    // A re-verification returns here without the link callback marker.
    if (!request.completed && request.errorCode) {
      const message = reverificationErrorMessage(request.errorCode);
      if (reverifiers.length === 0) fail(message);
      else offerReverification(message);
      return;
    }
    if (!provider) {
      fail(m('settings.account.sso.provider_unavailable'));
    } else if (provider.linked) {
      view = 'linked';
      announceProviderLinked(request.userId);
      window.close();
    } else if (request.completed) {
      fail(m('settings.account.sso.link_failed'), true);
    } else if (request.reverified) {
      // The user confirmed the link before the re-verification.
      void startLink(provider);
    } else if (request.reverificationRequired && reverifiers.length > 0) {
      // Account settings already found that the link needs a recent sign-in.
      offerReverification();
    } else {
      view = 'confirm';
    }
  });

  function offerReverification(message = '') {
    reverifyError = message;
    view = 'reverify';
  }

  /** Re-verify the user, then return to this window, which starts the link. */
  function startReverification(target: ExternalIdentityProviderInfo) {
    reverifyingProviderId = target.id;
    // Keep the buttons disabled while the browser leaves the page.
    leavePage(reverificationStartUrl(target, identityLinkPath(request.providerId, request.userId)));
  }

  async function startLink(target: ExternalIdentityProviderInfo, currentPassword?: string) {
    submitting = true;
    try {
      const startUrl = await externalIdentityAPI().startLink({
        providerId: target.id,
        redirectPath: identityLinkReturnPath(target.id, request.userId),
        currentPassword
      });
      // Keep the form disabled while the browser leaves the page.
      leavePage(startUrl);
    } catch (err) {
      submitting = false;
      const failure = linkStartFailure(err, {
        passwordSent: currentPassword !== undefined,
        canUsePasswordProof: data.user.hasPassword && data.directLoginEnabled,
        directLoginEnabled: data.directLoginEnabled,
        // A repeated failure after a re-verification must not loop.
        canReverify: !request.reverified && reverifiers.length > 0
      });
      if (failure.kind === 'password_required') view = 'password';
      else if (failure.kind === 'reverification_required') offerReverification();
      else if (failure.kind === 'password_rejected') passwordError = failure.message;
      else fail(failure.message, failure.retryable);
    }
  }

  function confirmLink(event: SubmitEvent) {
    event.preventDefault();
    if (provider) void startLink(provider);
  }

  async function submitPassword(event: SubmitEvent) {
    event.preventDefault();
    if (!provider || !password) return;
    passwordError = '';
    await startLink(provider, password);
  }

  /** Start again without the callback result. */
  function retry() {
    leavePage(identityLinkPath(request.providerId, request.userId));
  }

  const title = $derived.by(() => {
    if (view === 'reverify') return m('settings.account.sso.reverify_modal.title');
    return provider
      ? m('settings.account.sso.fresh_auth_modal.title', { provider: provider.label })
      : m('settings.account.sso.title');
  });
</script>

<PageTitle {title} />

<AuthLayout compact {title}>
  {#if view === 'loading'}
    <LoadingDots class="h-32 w-full" label={m('settings.account.sso.loading')} />
  {:else if view === 'error'}
    <div class="flex flex-col gap-5">
      <Hint tone="danger">{error.message}</Hint>
      {#if error.retryable}
        <Button size="lg" fullWidth onclick={retry}>{m('common.retry')}</Button>
      {/if}
      <Button
        variant={error.retryable ? 'secondary' : 'action'}
        size="lg"
        fullWidth
        onclick={() => window.close()}
      >
        {m('settings.account.sso.close_window')}
      </Button>
    </div>
  {:else if view === 'linked' && provider}
    <Hint tone="success" icon="icon-[uil--check-circle]">
      {m('settings.account.sso.linked_close', { provider: provider.label })}
    </Hint>
  {:else if view === 'confirm' && provider}
    <p class="mb-8 text-center leading-relaxed text-balance text-muted">
      {m('settings.account.sso.link_confirm_body', { provider: provider.label })}
    </p>
    <form class="flex flex-col gap-5" onsubmit={confirmLink}>
      <Button type="submit" size="lg" fullWidth loading={submitting}>
        <span aria-hidden="true" class="iconify icon-[uil--link]"></span>
        {m('auth.login.continue_with_provider', { provider: provider.label })}
      </Button>
      <Button variant="secondary" size="lg" fullWidth onclick={() => window.close()}>
        {m('common.cancel')}
      </Button>
    </form>
  {:else if view === 'reverify'}
    <div class="flex flex-col gap-5">
      <ReverificationChoice
        providers={reverifiers}
        body={m('settings.account.sso.reverify_modal.link_body', {
          provider: provider?.label ?? request.providerId
        })}
        error={reverifyError}
        busyProviderId={reverifyingProviderId}
        size="lg"
        onselect={startReverification}
      />
      <Button variant="secondary" size="lg" fullWidth onclick={() => window.close()}>
        {m('common.cancel')}
      </Button>
    </div>
  {:else if view === 'password' && provider}
    <p class="mb-8 text-center leading-relaxed text-balance text-muted">
      {m('settings.account.sso.fresh_auth_modal.body', { provider: provider.label })}
    </p>
    <form class="flex flex-col gap-5" onsubmit={submitPassword}>
      {#if passwordError}
        <Hint tone="danger">{passwordError}</Hint>
      {/if}
      <TextInput
        id="sso-link-current-password"
        label={m('settings.account.sso.fresh_auth_modal.password')}
        type="password"
        autocomplete="current-password"
        autofocus
        required
        bind:value={password}
        disabled={submitting}
      />
      <Button type="submit" size="lg" fullWidth loading={submitting} disabled={!password}>
        <span aria-hidden="true" class="iconify icon-[uil--link]"></span>
        {m('auth.login.continue_with_provider', { provider: provider.label })}
      </Button>
      <Button variant="secondary" size="lg" fullWidth onclick={() => window.close()}>
        {m('common.cancel')}
      </Button>
    </form>
  {/if}
</AuthLayout>
