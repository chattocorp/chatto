<script lang="ts">
  import { goto, invalidateAll } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { clearCachedUser } from '$lib/auth/loadAuth';
  import AuthLayout from '$lib/components/AuthLayout.svelte';
  import { graphql } from '$lib/gql';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { graphqlClientManager } from '$lib/state/server/graphqlClient.svelte';
  import * as m from '$lib/i18n/messages';
  import { Divider, Hint } from '$lib/ui';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { TextInput, FormError, Button, Form } from '$lib/ui/form';
  import AddServerDialog from '$lib/components/AddServerDialog.svelte';

  const { data } = $props();

  type AuthProviderInfo = {
    id: string;
    type: string;
    label: string;
    loginUrl: string;
  };

  let identifier = $state('');
  let password = $state('');
  let error = $state('');
  let isLoading = $state(false);
  let addServerDialogVisible = $state(false);

  const canSubmit = $derived(identifier.trim() && password);

  // Standalone detection: no origin instance means no local backend to log in to.
  // Only applies when there's no redirect param — a redirect means the backend sent
  // us here (e.g. OAuth authorize flow), so the origin probe just hasn't completed yet.
  const isStandalone = $derived(
    !serverRegistry.originServer && serverRegistry.originProbed && data.redirectUrl === '/'
  );

  $effect(() => {
    if (data.user) {
      navigateAfterLogin(data.redirectUrl);
    }
  });

  // Fetch auth providers and registration setting from GraphQL
  const LoginInfoQuery = graphql(`
    query LoginPageInfo {
      server {
        authProviders {
          id
          type
          label
          loginUrl
        }
        directRegistrationEnabled
      }
    }
  `);

  let authProviders = $state.raw<AuthProviderInfo[]>([]);
  let directRegistrationEnabled = $state(true);

  graphqlClientManager.originClient.client
    .query(LoginInfoQuery, {})
    .toPromise()
    .then((result) => {
      if (result.data) {
        authProviders = result.data.server.authProviders;
        directRegistrationEnabled = result.data.server.directRegistrationEnabled;
      }
    });

  /**
   * Same-origin path check; mirrors the validator in +page.ts but applied
   * to runtime values (sessionStorage.returnUrl) since +page.ts only sees
   * the URL search params.
   */
  function isSafeInternalPath(value: string): boolean {
    return (
      typeof value === 'string' &&
      value.startsWith('/') &&
      !value.startsWith('//') &&
      !value.startsWith('/\\')
    );
  }

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
      // eslint-disable-next-line svelte/no-navigation-without-resolve -- target is validated by isSafeInternalPath; backend routes (e.g. /oauth/...) are not SvelteKit routes
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

  function providerLoginHref(provider: AuthProviderInfo): string {
    return `${provider.loginUrl}?redirect=${encodeURIComponent(data.redirectUrl)}`;
  }

  async function handleSubmit(e: Event) {
    e.preventDefault();
    error = '';
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

      serverRegistry.authenticateOrigin(result.token, result.user ?? null);
      clearCachedUser();
      await invalidateAll();

      const returnUrl = sessionStorage.getItem('returnUrl');
      if (returnUrl) {
        sessionStorage.removeItem('returnUrl');
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

{#if !data.user}
  {#if isStandalone}
    <AuthLayout>
      <div class="flex flex-col items-center gap-6 text-center">
        <h1 class="text-2xl font-bold">{m['auth.login.welcome_title']()}</h1>
        <p class="text-muted">
          {m['auth.login.welcome_description']()}
        </p>
        <Button
          variant="accent"
          size="lg"
          fullWidth
          onclick={() => (addServerDialogVisible = true)}
        >
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
            <Button variant="secondary" size="lg" fullWidth href={providerLoginHref(provider)}>
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
          disabled={isLoading}
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
          disabled={isLoading}
          required
          autocomplete="current-password"
        />

        <FormError {error} />

        <Button
          type="submit"
          size="lg"
          disabled={!canSubmit}
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
{/if}

<AddServerDialog
  bind:visible={addServerDialogVisible}
  onclose={() => (addServerDialogVisible = false)}
/>
