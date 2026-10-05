<script lang="ts">
  import { errorMessage } from '$lib/utils/errorMessage';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { browserCookieAuthenticationHeaders } from '@chatto/client/auth/authenticationMode';
  import { completeOriginAuthentication } from '$lib/auth/originAuthentication';
  import AuthLayout from '$lib/components/AuthLayout.svelte';
  import { m } from '$lib/i18n/messages';
  import { Divider, Hint, PageTitle } from '$lib/ui';
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
    .min(2, m('common.validation.username_min'))
    .max(32, m('common.validation.username_max'))
    .regex(/^[a-zA-Z0-9._-]+$/, m('common.validation.username_charset'))
    .refine((val) => !val.endsWith('.'), m('common.validation.username_end_alphanumeric'))
    .refine((val) => !val.includes('..'), m('common.validation.username_no_consecutive_periods'));
  const passwordSchema = z.string().min(8, m('common.validation.password_min'));

  // Field-level errors (only show after user has typed something)
  const loginError = $derived(login ? validate(loginSchema, login) : undefined);
  const passwordError = $derived(password ? validate(passwordSchema, password) : undefined);
  const confirmError = $derived(
    confirmPassword && password !== confirmPassword
      ? m('common.validation.passwords_match')
      : undefined
  );

  const canSubmit = $derived(
    token && login && password && confirmPassword && !loginError && !passwordError && !confirmError
  );

  async function handleSubmit(e: Event) {
    e.preventDefault();
    if (!token || loginError || passwordError || confirmError) {
      error = loginError || passwordError || confirmError || m('common.validation.fix_errors');
      return;
    }

    error = '';
    isLoading = true;

    try {
      const response = await fetch('/auth/browser/register/complete', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...browserCookieAuthenticationHeaders
        },
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
        error = data.error || m('auth.register.failed');
        return;
      }

      const resumedReturnNavigation = await completeOriginAuthentication();
      if (!resumedReturnNavigation) {
        // New users have no navigation history, so go directly to root.
        // The root page handles redirecting to last position or Browse Spaces.
        goto(resolve('/'), { replaceState: true });
      }
    } catch (err) {
      error = errorMessage(err, m('auth.register.failed'));
    } finally {
      isLoading = false;
    }
  }
</script>

<PageTitle title={m('auth.register.complete_title')} />

<AuthLayout title={m('auth.register.complete_title')}>
  {#if !token}
    <Hint tone="danger">
      <p class="mb-2 font-medium">{m('auth.register.complete.invalid_title')}</p>
      <p class="text-sm">{m('auth.register.complete.invalid_text')}</p>
    </Hint>

    <p class="mt-6 text-center">
      <a href={resolve('/register')} class="link">
        {m('auth.register.complete.request_new_code')}
      </a>
    </p>
  {:else}
    <form onsubmit={handleSubmit} class="flex flex-col gap-4">
      <TextInput
        id="login"
        label={m('common.username')}
        bind:value={login}
        placeholder={m('common.username_placeholder')}
        disabled={isLoading}
        required
        autocomplete="username"
        error={loginError}
      />

      <TextInput
        id="password"
        label={m('common.password')}
        type="password"
        bind:value={password}
        placeholder={m('common.password_min_placeholder')}
        disabled={isLoading}
        required
        minlength={8}
        autocomplete="new-password"
        error={passwordError}
      />

      <TextInput
        id="confirmPassword"
        label={m('common.confirm_password')}
        type="password"
        bind:value={confirmPassword}
        placeholder={m('common.password_confirm_placeholder')}
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
        loadingText={m('auth.register.creating')}
      >
        <span aria-hidden="true" class="iconify icon-[uil--user-plus]"></span>
        {m('common.create_account')}
      </Button>
    </form>

    <Divider label={m('common.or')} />

    <Button href={resolve('/login')} variant="secondary" size="lg" fullWidth>
      {m('common.sign_in')}
    </Button>
  {/if}
</AuthLayout>
