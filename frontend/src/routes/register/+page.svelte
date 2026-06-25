<script lang="ts">
  import { goto, invalidateAll } from '$app/navigation';
  import { resolve } from '$app/paths';
  import AuthLayout from '$lib/components/AuthLayout.svelte';
  import * as m from '$lib/i18n/messages';
  import type { AuthenticatedUserSummary } from '$lib/state/server/registry.svelte';
  import Divider from '$lib/ui/Divider.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button, FormError, TextInput, validate, z } from '$lib/ui/form';

  const { data } = $props();

  type Step = 'email' | 'code' | 'details';

  const registrationEnabled = $derived(data.serverInfo?.directRegistrationEnabled ?? true);

  let step = $state<Step>('email');
  let email = $state('');
  let codeDigits = $state(['', '', '', '', '', '']);
  let completionToken = $state('');
  let login = $state('');
  let password = $state('');
  let confirmPassword = $state('');
  let error = $state('');
  let isLoading = $state(false);
  let isResending = $state(false);
  let codeInputs: HTMLInputElement[] = [];

  const emailSchema = z.string().email(m['common.validation.email']());
  const loginSchema = z
    .string()
    .min(2, m['common.validation.username_min']())
    .max(32, m['common.validation.username_max']())
    .regex(/^[a-zA-Z0-9._-]+$/, m['common.validation.username_charset']())
    .refine((val) => !val.includes('..'), m['common.validation.username_no_consecutive_periods']());
  const passwordSchema = z.string().min(8, m['common.validation.password_min']());

  const normalizedEmail = $derived(email.trim().toLowerCase());
  const emailError = $derived(email ? validate(emailSchema, email) : undefined);
  const code = $derived(codeDigits.join(''));
  const codeComplete = $derived(code.length === 6);
  const loginError = $derived(login ? validate(loginSchema, login) : undefined);
  const passwordError = $derived(password ? validate(passwordSchema, password) : undefined);
  const confirmError = $derived(
    confirmPassword && password !== confirmPassword
      ? m['common.validation.passwords_match']()
      : undefined
  );
  const canSubmitEmail = $derived(normalizedEmail && !emailError);
  const canSubmitDetails = $derived(
    completionToken &&
      login &&
      password &&
      confirmPassword &&
      !loginError &&
      !passwordError &&
      !confirmError
  );

  async function requestRegistrationCode(options: { resend?: boolean } = {}) {
    error = '';
    if (emailError || !normalizedEmail) {
      error = emailError || m['common.validation.email']();
      return;
    }

    if (options.resend) {
      isResending = true;
    } else {
      isLoading = true;
    }

    try {
      const response = await fetch('/auth/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: normalizedEmail })
      });
      const body = await response.json();

      if (!response.ok) {
        error = body.error || m['auth.register.failed']();
        return;
      }

      codeDigits = ['', '', '', '', '', ''];
      completionToken = '';
      step = 'code';
      queueMicrotask(() => codeInputs[0]?.focus());
    } catch (err) {
      error = err instanceof Error ? err.message : m['auth.register.failed']();
    } finally {
      isLoading = false;
      isResending = false;
    }
  }

  async function handleEmailSubmit(e: Event) {
    e.preventDefault();
    await requestRegistrationCode();
  }

  function applyCodeFrom(index: number, value: string) {
    const digits = value
      .replace(/\D/g, '')
      .slice(0, 6 - index)
      .split('');
    if (digits.length === 0) {
      codeDigits[index] = '';
      return;
    }
    for (const [offset, digit] of digits.entries()) {
      codeDigits[index + offset] = digit;
    }
    const nextIndex = Math.min(index + digits.length, codeDigits.length - 1);
    codeInputs[nextIndex]?.focus();
  }

  function handleCodeInput(index: number, e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    applyCodeFrom(index, input.value);
  }

  function handleCodePaste(index: number, e: ClipboardEvent) {
    e.preventDefault();
    applyCodeFrom(index, e.clipboardData?.getData('text') ?? '');
  }

  function handleCodeKeydown(index: number, e: KeyboardEvent) {
    if (e.key === 'Backspace' && codeDigits[index] === '' && index > 0) {
      e.preventDefault();
      codeDigits[index - 1] = '';
      codeInputs[index - 1]?.focus();
    }
  }

  async function handleCodeSubmit(e: Event) {
    e.preventDefault();
    if (!codeComplete) {
      error = m['auth.register.code.missing']();
      return;
    }

    error = '';
    isLoading = true;
    try {
      const response = await fetch('/auth/register/verify-code', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: normalizedEmail, code })
      });
      const body = await response.json();

      if (!response.ok) {
        error = body.error || m['auth.register.code.invalid']();
        return;
      }
      completionToken = body.completionToken;
      step = 'details';
    } catch (err) {
      error = err instanceof Error ? err.message : m['auth.register.failed']();
    } finally {
      isLoading = false;
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

  async function handleDetailsSubmit(e: Event) {
    e.preventDefault();
    if (!completionToken || loginError || passwordError || confirmError) {
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
          token: completionToken,
          login,
          password,
          passwordConfirmation: confirmPassword
        }),
        credentials: 'include'
      });
      const body = await response.json();

      if (!response.ok) {
        error = body.error || m['auth.register.failed']();
        return;
      }

      if (typeof body.token !== 'string' || !body.token) {
        error = m['auth.register.missing_token']();
        return;
      }

      await authenticateOrigin(body.token, body.user ?? null);
      await invalidateAll();

      const returnUrl = sessionStorage.getItem('returnUrl');
      if (returnUrl) {
        // Keep the marker until the authenticated chat shell sees it; otherwise
        // the chat landing redirect can win before the return URL settles.
        // eslint-disable-next-line svelte/no-navigation-without-resolve -- dynamic return URL from sessionStorage
        goto(returnUrl);
      } else {
        goto(resolve('/'), { replaceState: true });
      }
    } catch (err) {
      error = err instanceof Error ? err.message : m['auth.register.failed']();
    } finally {
      isLoading = false;
    }
  }
</script>

<PageTitle title={m['auth.register.title']()} />

<AuthLayout>
  <h1 class="mb-6 text-center text-2xl font-bold">
    {step === 'code'
      ? m['auth.register.code.title']()
      : step === 'details'
        ? m['auth.register.complete_title']()
        : m['auth.register.title']()}
  </h1>

  {#if !registrationEnabled}
    <p class="text-center text-muted">{m['auth.register.unavailable']()}</p>
  {:else if step === 'email'}
    <form onsubmit={handleEmailSubmit} class="flex flex-col gap-4">
      <TextInput
        id="email"
        label={m['common.email']()}
        type="email"
        bind:value={email}
        placeholder={m['common.email_placeholder']()}
        disabled={isLoading}
        required
        autofocus
        autocomplete="email"
        error={emailError}
      />

      <FormError {error} />

      <Button
        type="submit"
        size="lg"
        disabled={!canSubmitEmail}
        loading={isLoading}
        loadingText={m['auth.forgot_password.sending']()}
      >
        {m['common.continue']()}
        <span class="iconify uil--arrow-right"></span>
      </Button>
    </form>
  {:else if step === 'code'}
    <form onsubmit={handleCodeSubmit} class="flex flex-col gap-5">
      <div class="text-center">
        <p class="text-muted">{m['auth.register.code.sent_to']()}</p>
        <p class="mt-1 font-semibold break-words">{normalizedEmail}</p>
      </div>

      <div class="grid grid-cols-6 gap-2" aria-label={m['auth.register.code.aria_label']()}>
        {#each codeDigits as digit, index (index)}
          <input
            bind:this={codeInputs[index]}
            value={digit}
            type="text"
            inputmode="numeric"
            pattern="[0-9]*"
            maxlength="6"
            autocomplete={index === 0 ? 'one-time-code' : 'off'}
            aria-label={m['auth.register.code.digit_label']({ number: index + 1 })}
            disabled={isLoading}
            oninput={(e) => handleCodeInput(index, e)}
            onpaste={(e) => handleCodePaste(index, e)}
            onkeydown={(e) => handleCodeKeydown(index, e)}
            class="h-14 rounded-lg border border-text/20 bg-input text-center text-xl font-semibold transition outline-none focus:border-accent focus:ring-2 focus:ring-accent/30 disabled:opacity-60"
          />
        {/each}
      </div>

      <div class="text-center text-sm text-muted">
        {m['auth.register.code.did_not_receive']()}
        <button
          type="button"
          class="cursor-pointer link disabled:cursor-default disabled:opacity-60"
          disabled={isLoading || isResending}
          onclick={() => requestRegistrationCode({ resend: true })}
        >
          {isResending ? m['auth.register.code.resending']() : m['auth.register.code.resend']()}
        </button>
      </div>

      <FormError {error} />

      <Button
        type="submit"
        size="lg"
        disabled={!codeComplete}
        loading={isLoading}
        loadingText={m['auth.register.code.checking']()}
      >
        {m['common.submit']()}
      </Button>
    </form>
  {:else}
    <form onsubmit={handleDetailsSubmit} class="flex flex-col gap-4">
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
        disabled={!canSubmitDetails}
        loading={isLoading}
        loadingText={m['auth.register.creating']()}
      >
        <span class="iconify uil--user-plus"></span>
        {m['common.create_account']()}
      </Button>
    </form>
  {/if}

  <Divider label={m['common.or']()} />

  <a href={resolve('/login')} class="btn-secondary block w-full btn-lg text-center">
    {m['common.sign_in']()}
  </a>
</AuthLayout>
