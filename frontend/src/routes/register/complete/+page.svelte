<script lang="ts">
  import { goto, invalidateAll } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { clearCachedUser } from '$lib/auth/loadAuth';
  import AuthLayout from '$lib/components/AuthLayout.svelte';
  import * as m from '$lib/i18n/messages';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { Divider, Hint } from '$lib/ui';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { TextInput, FormError, Button, z, validate } from '$lib/ui/form';

  let { data } = $props();

  const token = $derived(data.token);

  let login = $state('');
  let password = $state('');
  let confirmPassword = $state('');
  let error = $state('');
  let isLoading = $state(false);

  // Validation schemas
  const loginSchema = z
    .string()
    .min(2, m['common.validation.username_min']())
    .max(32, m['common.validation.username_max']())
    .regex(/^[a-zA-Z0-9._-]+$/, m['common.validation.username_charset']())
    .refine((val) => !val.includes('..'), m['common.validation.username_no_consecutive_periods']());
  const passwordSchema = z.string().min(8, m['common.validation.password_min']());

  // Field-level errors (only show after user has typed something)
  const loginError = $derived(login ? validate(loginSchema, login) : undefined);
  const passwordError = $derived(password ? validate(passwordSchema, password) : undefined);
  const confirmError = $derived(
    confirmPassword && password !== confirmPassword
      ? m['common.validation.passwords_match']()
      : undefined
  );

  const canSubmit = $derived(
    token && login && password && confirmPassword && !loginError && !passwordError && !confirmError
  );

  async function handleSubmit(e: Event) {
    e.preventDefault();
    if (!token || loginError || passwordError || confirmError) {
      error = loginError || passwordError || confirmError || m['common.validation.fix_errors']();
      return;
    }

    error = '';
    isLoading = true;

    try {
      const response = await fetch('/auth/register/complete', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          token,
          login,
          password,
          passwordConfirmation: confirmPassword
        }),
        credentials: 'include'
      });

      const data = await response.json();

      if (!response.ok) {
        error = data.error || m['auth.register.failed']();
        return;
      }

      if (typeof data.token !== 'string' || !data.token) {
        error = m['auth.register.missing_token']();
        return;
      }

      serverRegistry.authenticateOrigin(data.token, data.user ?? null);

      // Clear auth cache and invalidate to force load functions to refetch
      clearCachedUser();
      await invalidateAll();

      // Check for a return URL (saved when redirected from a protected route)
      const returnUrl = sessionStorage.getItem('returnUrl');
      if (returnUrl) {
        sessionStorage.removeItem('returnUrl');
        // eslint-disable-next-line svelte/no-navigation-without-resolve -- dynamic return URL from sessionStorage
        goto(returnUrl);
      } else {
        // New users have no navigation history, so go directly to root.
        // The root page handles redirecting to last position or Browse Spaces.
        goto(resolve('/'), { replaceState: true });
      }
    } catch (err) {
      error = err instanceof Error ? err.message : m['auth.register.failed']();
    } finally {
      isLoading = false;
    }
  }
</script>

<PageTitle title={m['auth.register.complete_title']()} />

<AuthLayout>
  <h1 class="mb-6 text-center text-2xl font-bold">{m['auth.register.complete_title']()}</h1>

  {#if !token}
    <Hint tone="danger">
      <p class="mb-2 font-medium">{m['auth.register.complete.invalid_title']()}</p>
      <p class="text-sm">{m['auth.register.complete.invalid_text']()}</p>
    </Hint>

    <p class="mt-6 text-center">
      <a href={resolve('/register')} class="link">
        {m['auth.register.complete.request_new_code']()}
      </a>
    </p>
  {:else}
    <form onsubmit={handleSubmit} class="flex flex-col gap-4">
      <TextInput
        id="login"
        label={m['common.username']()}
        bind:value={login}
        placeholder={m['common.username_placeholder']()}
        disabled={isLoading}
        required
        autocomplete="username"
        error={loginError}
      />

      <TextInput
        id="password"
        label={m['common.password']()}
        type="password"
        bind:value={password}
        placeholder={m['common.password_min_placeholder']()}
        disabled={isLoading}
        required
        minlength={8}
        autocomplete="new-password"
        error={passwordError}
      />

      <TextInput
        id="confirmPassword"
        label={m['common.confirm_password']()}
        type="password"
        bind:value={confirmPassword}
        placeholder={m['common.password_confirm_placeholder']()}
        disabled={isLoading}
        required
        autocomplete="new-password"
        error={confirmError}
      />

      <FormError {error} />

      <Button
        type="submit"
        size="lg"
        disabled={!canSubmit}
        loading={isLoading}
        loadingText={m['auth.register.creating']()}
      >
        <span class="iconify uil--user-plus"></span>
        {m['common.create_account']()}
      </Button>
    </form>

    <Divider label={m['common.or']()} />

    <a href={resolve('/login')} class="btn-secondary block w-full btn-lg text-center">
      {m['common.sign_in']()}
    </a>
  {/if}
</AuthLayout>
