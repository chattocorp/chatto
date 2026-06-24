<script lang="ts">
  import { resolve } from '$app/paths';
  import AuthLayout from '$lib/components/AuthLayout.svelte';
  import * as m from '$lib/i18n/messages';
  import { Hint } from '$lib/ui';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { TextInput, FormError, Button, Form, z, validate } from '$lib/ui/form';

  let email = $state('');
  let error = $state('');
  let isLoading = $state(false);
  let submitted = $state(false);

  // Validation
  const emailSchema = z.string().email(m['common.validation.email']());
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
        error = data.error || m['common.error.generic']();
        return;
      }

      submitted = true;
    } catch (err) {
      error = err instanceof Error ? err.message : m['common.error.network']();
    } finally {
      isLoading = false;
    }
  }
</script>

<PageTitle title={m['auth.forgot_password.title']()} />

<AuthLayout>
  <h1 class="mb-6 text-center text-2xl font-bold">{m['auth.forgot_password.title']()}</h1>

  {#if submitted}
    <Hint tone="success">
      <p class="mb-2 font-medium">{m['auth.forgot_password.submitted_title']()}</p>
      <p class="text-sm">
        {m['auth.forgot_password.submitted_text']()}
      </p>
      <p class="mt-2 text-sm text-muted">{m['auth.forgot_password.spam_hint']()}</p>
    </Hint>

    <p class="mt-6 text-center">
      <a href={resolve('/login')} class="link">{m['auth.forgot_password.back_to_login']()}</a>
    </p>
  {:else}
    <p class="mb-6 text-sm text-muted">
      {m['auth.forgot_password.instructions']()}
    </p>

    <Form onsubmit={handleSubmit}>
      <TextInput
        id="email"
        label={m['common.email']()}
        type="email"
        bind:value={email}
        placeholder={m['common.email_placeholder']()}
        disabled={isLoading}
        required
        autocomplete="email"
        error={emailError}
      />

      <FormError {error} />

      <Button
        type="submit"
        size="lg"
        disabled={!canSubmit}
        loading={isLoading}
        loadingText={m['auth.forgot_password.sending']()}
      >
        <span class="iconify uil--envelope-send"></span>
        {m['auth.forgot_password.send_button']()}
      </Button>
    </Form>

    <p class="mt-6 text-center">
      {m['auth.forgot_password.remember_password']()}
      <a href={resolve('/login')} class="link">{m['common.sign_in']()}</a>
    </p>
  {/if}
</AuthLayout>
