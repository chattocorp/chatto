<script lang="ts">
  import { goto, invalidateAll } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { onMount } from 'svelte';
  import { getCurrentUser } from '$lib/auth/currentUser.svelte';
  import { clearCachedUser } from '$lib/auth/loadAuth';
  import { graphqlClientManager } from '$lib/state/instance/graphqlClient.svelte';
  import { graphql } from '$lib/gql';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { TextInput, TextArea, Checkbox, FormError, z, validate } from '$lib/ui/form';

  const currentUser = getCurrentUser();

  // Start in "checking" state - only show form after verification
  let checkComplete = $state(false);
  let setupAllowed = $state(false);

  // Form state
  let login = $state('');
  let email = $state('');
  let password = $state('');
  let spaceName = $state('');
  let spaceDescription = $state('');
  let createSpace = $state(true);
  let error = $state('');
  let isLoading = $state(false);

  // Check if setup is needed
  const CheckSetupQuery = graphql(`
    query CheckSetupAllowed {
      instance {
        needsSetup
      }
    }
  `);

  onMount(async () => {
    // If already logged in, redirect to chat immediately
    if (currentUser.user) {
      await goto(resolve('/chat'), { replaceState: true });
      return;
    }

    // Check if setup is actually needed
    const result = await graphqlClientManager.originClient.client.query(CheckSetupQuery, {});
    if (result.data?.instance.needsSetup) {
      setupAllowed = true;
    } else {
      // Instance already set up, redirect to login
      await goto(resolve('/'), { replaceState: true });
      return;
    }

    checkComplete = true;
  });

  // Validation schemas
  const loginSchema = z
    .string()
    .min(2, 'Must be at least 2 characters')
    .max(32, 'Must be at most 32 characters')
    .regex(/^[a-zA-Z0-9._-]+$/, 'Only letters, numbers, dots, dashes, underscores');
  const emailSchema = z.string().email('Please enter a valid email address');
  const passwordSchema = z.string().min(8, 'Must be at least 8 characters');
  const spaceNameSchema = z.string().min(1, 'Space name is required');

  // Field-level errors
  const loginError = $derived(login ? validate(loginSchema, login) : undefined);
  const emailError = $derived(email ? validate(emailSchema, email) : undefined);
  const passwordError = $derived(password ? validate(passwordSchema, password) : undefined);
  const spaceNameError = $derived(
    createSpace && spaceName ? validate(spaceNameSchema, spaceName) : undefined
  );

  const canSubmit = $derived(
    login &&
      email &&
      password &&
      !loginError &&
      !emailError &&
      !passwordError &&
      (!createSpace || (spaceName && !spaceNameError))
  );

  async function handleSubmit(e: Event) {
    e.preventDefault();
    error = '';
    isLoading = true;

    try {
      const response = await fetch('/auth/bootstrap', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          login,
          email,
          password,
          spaceName: createSpace ? spaceName.trim() : undefined,
          spaceDescription: createSpace ? spaceDescription.trim() : undefined
        }),
        credentials: 'include'
      });

      const data = await response.json();

      if (!response.ok) {
        error = data.error || 'Setup failed';
        return;
      }

      // Clear auth cache and invalidate to force load functions to refetch
      clearCachedUser();
      await invalidateAll();
      goto(resolve('/chat'));
    } catch (err) {
      error = err instanceof Error ? err.message : 'Setup failed';
    } finally {
      isLoading = false;
    }
  }
</script>

<PageTitle title="Set Up" />

{#if checkComplete && setupAllowed}
  <div class="flex min-h-full w-full items-center justify-center bg-background p-4">
    <div class="w-full max-w-md">
      <h1 class="mb-2 text-center text-3xl font-bold">
        Set Up <span class="text-primary">Chatto</span>
      </h1>
      <p class="mb-8 text-center text-muted">Welcome! Let's get your instance ready.</p>

      <form onsubmit={handleSubmit} class="flex flex-col gap-6">
        <!-- Admin Account Section -->
        <div class="space-y-4">
          <h2 class="text-lg font-semibold">Create Admin Account</h2>

          <TextInput
            id="login"
            label="Username"
            bind:value={login}
            placeholder="admin"
            disabled={isLoading}
            required
            autocomplete="username"
            error={loginError}
          />

          <TextInput
            id="email"
            label="Email"
            type="email"
            bind:value={email}
            placeholder="admin@example.com"
            disabled={isLoading}
            required
            autocomplete="email"
            error={emailError}
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
        </div>

        <!-- Initial Space Section (Optional) -->
        <div class="space-y-4">
          <Checkbox id="createSpace" bind:checked={createSpace} disabled={isLoading}>
            Create an initial space
          </Checkbox>

          {#if createSpace}
            <TextInput
              id="spaceName"
              label="Space Name"
              bind:value={spaceName}
              placeholder="My Community"
              disabled={isLoading}
              required
              error={spaceNameError}
            />

            <TextArea
              id="spaceDescription"
              label="Description (optional)"
              bind:value={spaceDescription}
              placeholder="What's this space about?"
              disabled={isLoading}
              rows={2}
            />

            <p class="text-sm text-muted">
              Default rooms (#announcements and #general) will be created automatically.
            </p>
          {/if}
        </div>

        <FormError {error} />

        <button
          type="submit"
          disabled={!canSubmit || isLoading}
          class="flex cursor-pointer items-center justify-center gap-2 rounded-lg bg-primary px-4 py-3 font-medium text-white transition-colors hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {#if isLoading}
            <span class="iconify animate-spin text-lg mdi--loading"></span>
            <span>Setting up...</span>
          {:else}
            <span class="iconify uil--check"></span>
            <span>Complete Setup</span>
          {/if}
        </button>
      </form>
    </div>
  </div>
{:else}
  <!-- Loading state - show nothing visible while checking/redirecting -->
  <div class="flex h-full w-full items-center justify-center">
    <span class="iconify animate-spin text-3xl text-muted mdi--loading"></span>
  </div>
{/if}
