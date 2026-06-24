<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import AuthLayout from '$lib/components/AuthLayout.svelte';
  import * as m from '$lib/i18n/messages';
  import { Hint } from '$lib/ui';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { TextInput, FormError, Button, z, validate } from '$lib/ui/form';

  let { data } = $props();

  const token = $derived(data.token);

  let password = $state('');
  let confirmPassword = $state('');
  let error = $state('');
  let isLoading = $state(false);

  // Validation
  const passwordSchema = z.string().min(8, m['common.validation.password_min']());
  const passwordError = $derived(password ? validate(passwordSchema, password) : undefined);
  const confirmError = $derived(
    confirmPassword && password !== confirmPassword
      ? m['common.validation.passwords_match']()
      : undefined
  );

  const canSubmit = $derived(
    token && password && confirmPassword && !passwordError && !confirmError
  );

  async function handleSubmit(e: Event) {
    e.preventDefault();
    if (!token || passwordError || confirmError) {
      error = passwordError || confirmError || m['common.validation.fix_errors']();
      return;
    }

    error = '';
    isLoading = true;

    try {
      const response = await fetch('/auth/reset-password', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token, password })
      });

      const data = await response.json();

      if (!response.ok) {
        error = data.error || m['common.error.generic']();
        return;
      }

      // Success - redirect to login with message
      const url = resolve('/login') + '?reset=success';
      // eslint-disable-next-line svelte/no-navigation-without-resolve -- url is resolved above
      goto(url);
    } catch (err) {
      error = err instanceof Error ? err.message : m['common.error.network']();
    } finally {
      isLoading = false;
    }
  }
</script>

<PageTitle title={m['auth.reset_password.page_title']()} />

<AuthLayout>
  <h1 class="mb-6 text-center text-2xl font-bold">{m['auth.reset_password.title']()}</h1>

  {#if !token}
    <Hint tone="danger">
      <p class="mb-2 font-medium">{m['auth.reset_password.invalid_title']()}</p>
      <p class="text-sm">{m['auth.reset_password.invalid_text']()}</p>
    </Hint>

    <p class="mt-6 text-center">
      <a href={resolve('/forgot-password')} class="link">
        {m['auth.reset_password.request_new_link']()}
      </a>
    </p>
  {:else}
    <form onsubmit={handleSubmit} class="flex flex-col gap-4">
      <TextInput
        id="password"
        label={m['common.new_password']()}
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
        loadingText={m['auth.reset_password.resetting']()}
      >
        {m['auth.reset_password.submit']()}
      </Button>
    </form>

    <p class="mt-6 text-center">
      {m['auth.forgot_password.remember_password']()}
      <a href={resolve('/login')} class="link">{m['common.sign_in']()}</a>
    </p>
  {/if}
</AuthLayout>
