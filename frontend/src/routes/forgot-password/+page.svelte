<script lang="ts">
  import { resolve } from '$app/paths';
  import AuthLayout from '$lib/components/AuthLayout.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { TextInput, FormError, Button, z, validate } from '$lib/ui/form';

  let email = $state('');
  let error = $state('');
  let isLoading = $state(false);
  let submitted = $state(false);

  // Validation
  const emailSchema = z.string().email('Please enter a valid email address');
  const emailError = $derived(email ? validate(emailSchema, email) : undefined);
  const canSubmit = $derived(email && !emailError);

  async function handleSubmit(e: Event) {
    e.preventDefault();
    if (emailError) {
      error = emailError;
      return;
    }

    error = '';
    isLoading = true;

    try {
      const response = await fetch('/auth/forgot-password', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email })
      });

      const data = await response.json();

      if (!response.ok) {
        error = data.error || 'Something went wrong';
        return;
      }

      submitted = true;
    } catch (err) {
      error = err instanceof Error ? err.message : 'Network error. Please try again.';
    } finally {
      isLoading = false;
    }
  }
</script>

<PageTitle title="Forgot Password" />

<AuthLayout>
  <h1 class="mb-6 text-center text-2xl font-bold">Forgot Password</h1>

  {#if submitted}
    <div
      class="rounded-lg bg-green-100 p-4 text-center text-green-800 dark:bg-green-900/30 dark:text-green-200"
    >
      <p class="mb-2 font-medium">Check your email</p>
      <p class="text-sm">
        If that email is registered, you'll receive a password reset link shortly.
      </p>
      <p class="mt-2 text-sm text-muted">Check your spam folder if you don't see it.</p>
    </div>

    <p class="mt-6 text-center">
      <a href={resolve('/login')} class="link">← Back to login</a>
    </p>
  {:else}
    <p class="mb-6 text-sm text-muted">
      Enter your email address and we'll send you a link to reset your password.
    </p>

    <form onsubmit={handleSubmit} class="flex flex-col gap-4">
      <TextInput
        id="email"
        label="Email"
        type="email"
        bind:value={email}
        placeholder="you@example.com"
        disabled={isLoading}
        required
        autocomplete="email"
        error={emailError}
      />

      <FormError {error} />

      <Button type="submit" size="lg" disabled={!canSubmit} loading={isLoading} loadingText="Sending...">
        <span class="iconify uil--envelope-send"></span>
        Send Reset Link
      </Button>
    </form>

    <p class="mt-6 text-center">
      Remember your password?
      <a href={resolve('/login')} class="link">Sign in</a>
    </p>
  {/if}
</AuthLayout>
