<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import AuthLayout from '$lib/components/AuthLayout.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { TextInput, FormError, Button, z, validate } from '$lib/ui/form';

  let { data } = $props();

  const token = $derived(data.token);

  let password = $state('');
  let confirmPassword = $state('');
  let error = $state('');
  let isLoading = $state(false);

  // Validation
  const passwordSchema = z.string().min(8, 'Must be at least 8 characters');
  const passwordError = $derived(password ? validate(passwordSchema, password) : undefined);
  const confirmError = $derived(
    confirmPassword && password !== confirmPassword ? 'Passwords do not match' : undefined
  );

  const canSubmit = $derived(
    token && password && confirmPassword && !passwordError && !confirmError
  );

  async function handleSubmit(e: Event) {
    e.preventDefault();
    if (!token || passwordError || confirmError) {
      error = passwordError || confirmError || 'Please fix the errors above';
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
        error = data.error || 'Something went wrong';
        return;
      }

      // Success - redirect to login with message
      const url = resolve('/login') + '?reset=success';
      // eslint-disable-next-line svelte/no-navigation-without-resolve -- url is resolved above
      goto(url);
    } catch (err) {
      error = err instanceof Error ? err.message : 'Network error. Please try again.';
    } finally {
      isLoading = false;
    }
  }
</script>

<PageTitle title="Reset Password" />

<AuthLayout>
  <h1 class="mb-6 text-center text-2xl font-bold">Set New Password</h1>

  {#if !token}
    <div
      class="rounded-lg bg-red-100 p-4 text-center text-red-800 dark:bg-red-900/30 dark:text-red-200"
    >
      <p class="mb-2 font-medium">Invalid reset link</p>
      <p class="text-sm">This link is invalid or has expired.</p>
    </div>

    <p class="mt-6 text-center">
      <a href={resolve('/forgot-password')} class="link">Request a new link</a>
    </p>
  {:else}
    <form onsubmit={handleSubmit} class="flex flex-col gap-4">
      <TextInput
        id="password"
        label="New Password"
        type="password"
        bind:value={password}
        placeholder="At least 8 characters"
        disabled={isLoading}
        required
        minlength={8}
        autocomplete="new-password"
        error={passwordError}
      />

      <TextInput
        id="confirmPassword"
        label="Confirm Password"
        type="password"
        bind:value={confirmPassword}
        placeholder="Enter password again"
        disabled={isLoading}
        required
        autocomplete="new-password"
        error={confirmError}
      />

      <FormError {error} />

      <Button type="submit" size="lg" disabled={!canSubmit} loading={isLoading} loadingText="Resetting...">
        Reset Password
      </Button>
    </form>

    <p class="mt-6 text-center">
      Remember your password?
      <a href={resolve('/login')} class="link">Sign in</a>
    </p>
  {/if}
</AuthLayout>
