<script lang="ts">
  import { goto, invalidateAll } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { clearCachedUser } from '$lib/auth/loadAuth';
  import AuthLayout from '$lib/components/AuthLayout.svelte';
  import { Divider } from '$lib/ui';
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
    .min(2, 'Must be at least 2 characters')
    .max(32, 'Must be at most 32 characters')
    .regex(/^[a-zA-Z0-9._-]+$/, 'Only letters, numbers, dots, dashes, underscores')
    .refine((val) => !val.includes('..'), 'No consecutive periods allowed');
  const passwordSchema = z.string().min(8, 'Must be at least 8 characters');

  // Field-level errors (only show after user has typed something)
  const loginError = $derived(login ? validate(loginSchema, login) : undefined);
  const passwordError = $derived(password ? validate(passwordSchema, password) : undefined);
  const confirmError = $derived(
    confirmPassword && password !== confirmPassword ? 'Passwords do not match' : undefined
  );

  const canSubmit = $derived(
    token && login && password && confirmPassword && !loginError && !passwordError && !confirmError
  );

  async function handleSubmit(e: Event) {
    e.preventDefault();
    if (!token || loginError || passwordError || confirmError) {
      error = loginError || passwordError || confirmError || 'Please fix the errors above';
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
        error = data.error || 'Registration failed';
        return;
      }

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
      error = err instanceof Error ? err.message : 'Registration failed';
    } finally {
      isLoading = false;
    }
  }
</script>

  <PageTitle title="Complete Registration" />

<AuthLayout>
  <h1 class="mb-6 text-center text-2xl font-bold">Complete Registration</h1>

  {#if !token}
    <div
      class="rounded-lg bg-red-100 p-4 text-center text-red-800 dark:bg-red-900/30 dark:text-red-200"
    >
      <p class="mb-2 font-medium">Invalid registration code</p>
      <p class="text-sm">This registration session is invalid or has expired.</p>
    </div>

    <p class="mt-6 text-center">
      <a href={resolve('/register')} class="link">Request a new code</a>
    </p>
  {:else}
    <form onsubmit={handleSubmit} class="flex flex-col gap-4">
      <TextInput
        id="login"
        label="Username"
        bind:value={login}
        placeholder="your_username"
        disabled={isLoading}
        required
        autocomplete="username"
        error={loginError}
      />

      <TextInput
        id="password"
        label="Password"
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

      <Button type="submit" size="lg" disabled={!canSubmit} loading={isLoading} loadingText="Creating account...">
        <span class="iconify uil--user-plus"></span>
        Create Account
      </Button>
    </form>

    <Divider label="or" />

    <a href={resolve('/login')} class="btn-secondary btn-lg block w-full text-center">
      Sign In
    </a>
  {/if}
</AuthLayout>
