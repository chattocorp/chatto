<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import AuthLayout from '$lib/components/AuthLayout.svelte';
  import { Divider } from '$lib/ui';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { TextInput, FormError, Button, z, validate } from '$lib/ui/form';

  const { data } = $props();

  // Redirect if already logged in (use layout data, consistent with /login)
  // svelte-ignore state_referenced_locally
  if (data.user) {
    goto(resolve('/'));
  }

  // Registration enabled check from server store (loaded by root layout)
  const origin = $derived(serverRegistry.originServer);
  const originStore = $derived(origin ? serverRegistry.tryGetStore(origin.id) : undefined);
  const registrationEnabled = $derived(originStore?.serverInfo.directRegistrationEnabled ?? true);

  let email = $state('');
  let error = $state('');
  let isLoading = $state(false);
  let emailSent = $state(false);

  // Validation
  const emailSchema = z.string().email('Please enter a valid email address');
  const emailError = $derived(email ? validate(emailSchema, email) : undefined);
  const canSubmit = $derived(email && !emailError);

  async function handleSubmit(e: Event) {
    e.preventDefault();
    error = '';

    if (emailError) {
      error = emailError;
      return;
    }

    isLoading = true;

    try {
      const response = await fetch('/auth/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email })
      });

      const data = await response.json();

      if (!response.ok) {
        error = data.error || 'Registration failed';
        return;
      }

      emailSent = true;
    } catch (err) {
      error = err instanceof Error ? err.message : 'Registration failed';
    } finally {
      isLoading = false;
    }
  }
</script>

<PageTitle title="Create Account" />

<AuthLayout>
  <h1 class="mb-6 text-center text-2xl font-bold">Create Account</h1>

  {#if !registrationEnabled}
    <p class="text-center text-muted">Registration is not available on this instance.</p>
  {:else if emailSent}
    <div
      class="rounded-lg bg-green-100 p-6 text-center dark:bg-green-900/30"
    >
      <p class="mb-2 font-medium text-green-800 dark:text-green-200">Check your email</p>
      <p class="text-sm text-green-700 dark:text-green-300">
        We sent a registration link to <strong>{email}</strong>. Click the link to complete your
        account.
      </p>
    </div>
  {:else}
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
        Continue
        <span class="iconify uil--arrow-right"></span>
      </Button>
    </form>
  {/if}

  <Divider label="or" />

  <a href={resolve('/login')} class="btn-secondary btn-lg block w-full text-center">
    Sign In
  </a>
</AuthLayout>
