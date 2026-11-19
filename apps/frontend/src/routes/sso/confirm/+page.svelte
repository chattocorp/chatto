<script lang="ts">
  import { MAX_LOGIN_LENGTH } from '$lib/validation/login';
  import { errorMessage } from '$lib/utils/errorMessage';
  import { UsernameChoiceAvailability } from '@chatto/client/api/actionAvailability';
  import { usernameChoiceUnavailableMessage } from '$lib/actionAvailability';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { completeOriginAuthentication } from '$lib/auth/originAuthentication';
  import { isProviderCallback } from '$lib/auth/providerContinuation';
  import { providerIcon } from '$lib/auth/providerIcon';
  import { getAuthServerInfo } from '$lib/components/authServerInfo';
  import AuthLayout from '$lib/components/AuthLayout.svelte';
  import {
    createExternalIdentityFlowAPI,
    createExternalIdentityAPI,
    ExternalIdentityFlowKind
  } from '$lib/api/externalIdentities';
  import { m } from '$lib/i18n/messages';
  import { MAX_DISPLAY_NAME_LENGTH, validateDisplayName } from '$lib/validation/displayName';
  import { Hint, PageTitle } from '$lib/ui';
  import { TextInput, FormError, Button, z, validate } from '$lib/ui/form';

  const { data } = $props();
  const getServerInfo = getAuthServerInfo();
  const flowAPI = createExternalIdentityFlowAPI();

  const pending = $derived(data.pending);
  let actionError = $state('');
  let submitting = $state(false);
  let login = $derived(pending?.loginHint ?? '');
  const usernameChoice = $derived(pending?.chooseUsername ?? UsernameChoiceAvailability.ALLOWED);
  const usernameManaged = $derived(usernameChoice === UsernameChoiceAvailability.PROVIDER_MANAGED);
  let displayName = $derived(
    pending?.displayNameHint ||
      (usernameManaged ? '' : pending?.loginHint.slice(0, MAX_DISPLAY_NAME_LENGTH)) ||
      ''
  );

  const loginSchema = z
    .string()
    .min(2, m('common.validation.username_min'))
    .max(MAX_LOGIN_LENGTH, m('common.validation.username_max', { max: MAX_LOGIN_LENGTH }))
    .regex(/^[a-zA-Z0-9._-]+$/, m('common.validation.username_charset'))
    .refine((val) => !val.endsWith('.'), m('common.validation.username_end_alphanumeric'))
    .refine((val) => !val.includes('..'), m('common.validation.username_no_consecutive_periods'));

  // Managed names are assigned and validated by the server; this field is a preview.
  const loginError = $derived(!usernameManaged && login ? validate(loginSchema, login) : undefined);
  const displayNameError = $derived.by(() => {
    if (!displayName) return undefined;
    return validateDisplayName(displayName).valid
      ? undefined
      : m('settings.profile.display_name.invalid');
  });
  const isCreate = $derived(
    pending?.kind === ExternalIdentityFlowKind.CREATE_ACCOUNT && !data.existingAccount
  );
  const isLink = $derived(
    pending?.kind === ExternalIdentityFlowKind.LINK_ACCOUNT || data.existingAccount
  );
  const serverName = $derived(getServerInfo()?.name || 'Chatto');
  const title = $derived(
    isCreate
      ? m('auth.sso.join_title', { server: serverName })
      : isLink
        ? m('auth.sso.link_button')
        : m('auth.sso.title')
  );
  const canSubmit = $derived(
    pending &&
      !submitting &&
      ((isCreate &&
        (usernameManaged || login.trim()) &&
        (usernameManaged || displayName.trim()) &&
        !loginError &&
        !displayNameError) ||
        isLink)
  );

  async function handleCreate(e: Event) {
    e.preventDefault();
    if (!pending || !data.token || loginError || displayNameError) {
      actionError = loginError || displayNameError || m('common.validation.fix_errors');
      return;
    }
    const redirectPath = pending.redirectPath || '/';
    submitting = true;
    actionError = '';
    try {
      await flowAPI.createAccount({
        token: data.token,
        login: usernameManaged ? '' : login,
        displayName
      });
      const returnURL = new URL(redirectPath, window.location.origin);
      if (isProviderCallback(returnURL)) {
        // The opening tab owns its saved return navigation. Complete this
        // popup before any copied sessionStorage return path can take over.
        await goto(resolve(redirectPath as '/'), { replaceState: true });
        return;
      }
      const resumedReturnNavigation = await completeOriginAuthentication();
      if (!resumedReturnNavigation) {
        goto(resolve(redirectPath as '/'), { replaceState: true });
      }
    } catch (err) {
      actionError = errorMessage(err, m('auth.sso.create_failed'));
    } finally {
      submitting = false;
    }
  }

  async function handleLink() {
    if (!pending || !data.token) return;
    const redirectPath = pending.redirectPath || '/';
    submitting = true;
    actionError = '';
    try {
      if (data.existingAccount) {
        // A create token cannot authorize a link. Start a fresh provider flow
        // bound by the server to the account that just authenticated.
        const startUrl = await createExternalIdentityAPI({
          baseUrl: '/api/connect',
          bearerToken: null
        }).startLink({
          providerId: pending.providerId,
          redirectPath
        });
        try {
          await flowAPI.cancel(data.token);
        } catch {
          // The accepted link-start is independent; the old create token expires.
        }
        window.location.href = startUrl;
        return;
      }
      await flowAPI.confirmLink(data.token);
      goto(resolve(redirectPath as '/'), { replaceState: true });
    } catch (err) {
      actionError = errorMessage(err, m('auth.sso.link_failed'));
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
    const returnURL = new URL(pending?.redirectPath || '/login', window.location.origin);
    if (isProviderCallback(returnURL)) returnURL.searchParams.set('error', 'access_denied');
    goto(resolve((returnURL.pathname + returnURL.search + returnURL.hash) as '/login'), {
      replaceState: true
    });
  }
</script>

<!-- Who signed in: the provider and, when it supplies one, the verified email. -->
{#snippet providerIdentity(info: NonNullable<typeof pending>)}
  <div class="flex items-center gap-3 surface-box p-3">
    <span
      aria-hidden="true"
      class={['iconify shrink-0 text-2xl text-action', providerIcon(info.providerType)]}
    ></span>
    <div class="min-w-0">
      {#if info.verifiedEmail}
        <p class="font-semibold break-words"><bdi>{info.verifiedEmail}</bdi></p>
      {/if}
      <p class={info.verifiedEmail ? 'text-sm text-muted' : 'font-semibold'}>
        {m('auth.sso.signed_in', { provider: info.providerLabel })}
      </p>
    </div>
  </div>
{/snippet}

{#snippet cancelButton()}
  <Button variant="ghost" onclick={handleCancel} disabled={submitting}>
    {m('common.cancel')}
  </Button>
{/snippet}

<PageTitle {title} />

<!-- The title names the server, so the page shows only its banner and description. -->
<AuthLayout compact {title}>
  {#if !data.token}
    <Hint tone="danger">{m('auth.sso.invalid')}</Hint>
    <p class="mt-6 text-center">
      <a href={resolve('/login')} class="link">{m('common.sign_in')}</a>
    </p>
  {:else if data.loadError}
    <Hint tone="danger">
      {data.loadError === 'provider_username'
        ? m('settings.account.sso.username_unavailable')
        : data.loadError === 'invalid'
          ? m('auth.sso.invalid')
          : m('auth.sso.load_failed')}
    </Hint>
    <p class="mt-6 text-center">
      <a href={resolve('/login')} class="link">{m('common.sign_in')}</a>
    </p>
  {:else if pending && isCreate}
    <div class="mb-6">
      {@render providerIdentity(pending)}
    </div>

    <form onsubmit={handleCreate} class="flex flex-col gap-4">
      <!-- A provider-managed username shows as a disabled field. The server
           assigns it and ignores any username that the client sends. -->
      <TextInput
        id="sso-login"
        label={m('common.username')}
        description={usernameChoice === UsernameChoiceAvailability.ALLOWED
          ? m('auth.sso.username_help')
          : usernameChoiceUnavailableMessage(usernameChoice)}
        bind:value={login}
        placeholder={m('common.username_placeholder')}
        disabled={submitting || usernameChoice !== UsernameChoiceAvailability.ALLOWED}
        required={!usernameManaged}
        autocomplete="username"
        error={loginError}
      />

      <!-- An optional display name falls back to the username, which the placeholder shows. -->
      <TextInput
        id="sso-display-name"
        label={m('settings.profile.display_name.label')}
        description={m('auth.sso.display_help')}
        bind:value={displayName}
        placeholder={usernameManaged ? login : m('settings.profile.display_name.placeholder')}
        disabled={submitting}
        required={!usernameManaged}
        maxlength={32}
        autocomplete="name"
        error={displayNameError}
      />

      <FormError error={actionError} />

      <Button
        type="submit"
        size="lg"
        fullWidth
        disabled={!canSubmit}
        loading={submitting}
        loadingText={m('auth.sso.creating')}
      >
        {m('auth.sso.create_action')}
      </Button>
    </form>

    <div class="mt-6 flex flex-col items-center gap-2 text-center">
      <p class="text-muted">
        {m('auth.sso.existing_prompt')}
        <a
          href={resolve(
            `/sso/confirm?token=${encodeURIComponent(data.token)}&existing=1` as '/sso/confirm'
          )}
          class="link"
          aria-disabled={submitting || undefined}
          onclick={(event) => submitting && event.preventDefault()}
          >{m('auth.sso.sign_in_existing')}</a
        >
      </p>
      {@render cancelButton()}
    </div>
  {:else if pending && isLink}
    <div class="mb-6 flex flex-col gap-3">
      {@render providerIdentity(pending)}
      <p class="text-muted">
        {m('auth.sso.link_intro', { provider: pending.providerLabel })}
      </p>
      {#if data.user && (data.existingAccount || data.user.id === pending.boundUserId)}
        <div class="flex items-center gap-3 surface-box p-3">
          <span aria-hidden="true" class="iconify icon-[uil--user] shrink-0 text-2xl text-muted"
          ></span>
          <div class="min-w-0">
            <p class="font-semibold break-words"><bdi>{data.user.displayName}</bdi></p>
            <p class="break-words text-muted"><bdi>@{data.user.login}</bdi></p>
          </div>
        </div>
      {/if}
    </div>

    <FormError error={actionError} />

    <div class="flex items-center gap-2">
      <div class="min-w-0 flex-1">
        <Button
          fullWidth
          size="lg"
          disabled={!canSubmit}
          loading={submitting}
          loadingText={m('auth.sso.linking')}
          onclick={handleLink}
        >
          <span aria-hidden="true" class="iconify icon-[uil--link]"></span>
          {m('auth.sso.link_button')}
        </Button>
      </div>
      {@render cancelButton()}
    </div>
  {:else}
    <Hint tone="danger">{m('auth.sso.invalid')}</Hint>
  {/if}
</AuthLayout>
