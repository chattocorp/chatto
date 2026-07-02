<script lang="ts">
  import { goto, invalidateAll } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { Code, ConnectError } from '@connectrpc/connect';
  import AuthLayout from '$lib/components/AuthLayout.svelte';
  import {
    createExternalIdentityFlowAPI,
    ExternalIdentityFlowKind,
    type PendingExternalIdentityInfo
  } from '$lib/api-client/externalIdentities';
  import * as m from '$lib/i18n/messages';
  import type { AuthenticatedUserSummary } from '$lib/state/server/registry.svelte';
  import Hint from '$lib/ui/Hint.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { TextInput, FormError, Button, z, validate } from '$lib/ui/form';

  const { data } = $props();
  const flowAPI = createExternalIdentityFlowAPI();

  let pending = $state<PendingExternalIdentityInfo | null>(null);
  let loadError = $state('');
  let actionError = $state('');
  let loading = $state(true);
  let submitting = $state(false);
  let login = $state('');
  let loadedToken = '';

  const loginSchema = z
    .string()
    .min(2, m['common.validation.username_min']())
    .max(32, m['common.validation.username_max']())
    .regex(/^[a-zA-Z0-9._-]+$/, m['common.validation.username_charset']())
    .refine((val) => !val.includes('..'), m['common.validation.username_no_consecutive_periods']());

  const loginError = $derived(login ? validate(loginSchema, login) : undefined);
  const isCreate = $derived(pending?.kind === ExternalIdentityFlowKind.CREATE_ACCOUNT);
  const isLink = $derived(pending?.kind === ExternalIdentityFlowKind.LINK_ACCOUNT);
  const canSubmit = $derived(
    pending && !submitting && ((isCreate && login.trim() && !loginError) || isLink)
  );

  $effect(() => {
    const token = data.token;
    if (!token || token === loadedToken) return;
    loadedToken = token;
    void loadPending(token);
  });

  async function loadPending(token: string) {
    loading = true;
    loadError = '';
    actionError = '';
    pending = null;
    try {
      const result = await flowAPI.getPending(token);
      if (!result) {
        loadError = m['auth.sso.invalid']();
        return;
      }
      pending = result;
      login = result.loginHint;
    } catch (err) {
      if (err instanceof ConnectError && err.code === Code.NotFound) {
        loadError = m['auth.sso.invalid']();
      } else {
        loadError = err instanceof Error ? err.message : m['auth.sso.load_failed']();
      }
    } finally {
      loading = false;
    }
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

  async function handleCreate(e: Event) {
    e.preventDefault();
    if (!pending || !data.token || loginError) {
      actionError = loginError || m['common.validation.fix_errors']();
      return;
    }
    submitting = true;
    actionError = '';
    try {
      const result = await flowAPI.createAccount({ token: data.token, login });
      await authenticateOrigin(result.token, {
        id: result.userId,
        login: result.login
      });
      await invalidateAll();
      goto(resolve((pending.redirectPath || '/') as '/'), { replaceState: true });
    } catch (err) {
      actionError = err instanceof Error ? err.message : m['auth.sso.create_failed']();
    } finally {
      submitting = false;
    }
  }

  async function handleLink() {
    if (!pending || !data.token) return;
    submitting = true;
    actionError = '';
    try {
      await flowAPI.confirmLink(data.token);
      goto(resolve((pending.redirectPath || '/') as '/'), { replaceState: true });
    } catch (err) {
      actionError = err instanceof Error ? err.message : m['auth.sso.link_failed']();
    } finally {
      submitting = false;
    }
  }

  async function handleCancel() {
    if (data.token) {
      try {
        await flowAPI.cancel(data.token);
      } catch {
        // Cancelling is best-effort; leaving the page is enough for the user.
      }
    }
    goto(resolve((pending?.redirectPath || '/login') as '/login'), { replaceState: true });
  }
</script>

<PageTitle title={m['auth.sso.title']()} />

<AuthLayout>
  <h1 class="mb-6 text-center text-2xl font-bold">{m['auth.sso.title']()}</h1>

  {#if !data.token}
    <Hint tone="danger">{m['auth.sso.invalid']()}</Hint>
    <p class="mt-6 text-center">
      <a href={resolve('/login')} class="link">{m['common.sign_in']()}</a>
    </p>
  {:else if loading}
    <div class="text-center text-sm text-muted">{m['auth.sso.loading']()}</div>
  {:else if loadError}
    <Hint tone="danger">{loadError}</Hint>
    <p class="mt-6 text-center">
      <a href={resolve('/login')} class="link">{m['common.sign_in']()}</a>
    </p>
  {:else if pending && isCreate}
    <div class="mb-5 flex flex-col gap-2 text-center">
      <p class="text-sm text-muted">
        {m['auth.sso.create_intro']({ provider: pending.providerLabel })}
      </p>
      {#if pending.verifiedEmail}
        <p class="text-sm">{pending.verifiedEmail}</p>
      {/if}
    </div>

    <form onsubmit={handleCreate} class="flex flex-col gap-4">
      <TextInput
        id="sso-login"
        label={m['common.username']()}
        bind:value={login}
        placeholder={m['common.username_placeholder']()}
        disabled={submitting}
        required
        autocomplete="username"
        error={loginError}
      />

      <FormError error={actionError} />

      <Button
        type="submit"
        size="lg"
        disabled={!canSubmit}
        loading={submitting}
        loadingText={m['auth.sso.creating']()}
      >
        <span class="iconify uil--user-plus"></span>
        {m['common.create_account']()}
      </Button>
    </form>

    <div class="mt-3">
      <Button variant="secondary" fullWidth href={resolve('/login')} disabled={submitting}>
        <span class="iconify mdi--login"></span>
        {m['auth.sso.sign_in_existing']()}
      </Button>
    </div>

    <div class="mt-3">
      <Button variant="ghost" fullWidth onclick={handleCancel} disabled={submitting}>
        {m['common.cancel']()}
      </Button>
    </div>
  {:else if pending && isLink}
    <div class="mb-5 flex flex-col gap-2 text-center">
      <p class="text-sm text-muted">
        {m['auth.sso.link_intro']({ provider: pending.providerLabel })}
      </p>
      {#if pending.verifiedEmail}
        <p class="text-sm">{pending.verifiedEmail}</p>
      {/if}
    </div>

    <FormError error={actionError} />

    <div class="flex flex-col gap-3">
      <Button
        size="lg"
        disabled={!canSubmit}
        loading={submitting}
        loadingText={m['auth.sso.linking']()}
        onclick={handleLink}
      >
        <span class="iconify uil--link"></span>
        {m['auth.sso.link_button']()}
      </Button>
      <Button variant="secondary" fullWidth onclick={handleCancel} disabled={submitting}>
        {m['common.cancel']()}
      </Button>
    </div>
  {:else}
    <Hint tone="danger">{m['auth.sso.invalid']()}</Hint>
  {/if}
</AuthLayout>
