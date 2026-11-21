<!--
@component
Direct sign-in with a username or email address and password, with the
password-reset link and, when the server allows direct registration, the
Create Account action. Owns the credentials and the submission. The login page
and the password login page both render it.
-->
<script lang="ts">
  import { resolve } from '$app/paths';
  import { signInWithPassword } from '$lib/auth/passwordSignIn';
  import { navigateAfterAuthentication } from '$lib/auth/returnNavigation';
  import { isProviderContinuation } from '$lib/auth/providerContinuation';
  import { withReturnPath } from '$lib/auth/loginRoutes';
  import { errorMessage } from '$lib/utils/errorMessage';
  import { m } from '$lib/i18n/messages';
  import { Divider } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import PasswordSignInForm from './PasswordSignInForm.svelte';

  let {
    redirectUrl,
    directRegistrationEnabled,
    compact = false,
    pageError = ''
  }: {
    /** Same-origin path to open after sign-in. */
    redirectUrl: string;
    /** Shows the Create Account action. */
    directRegistrationEnabled: boolean;
    compact?: boolean;
    /** Error from the page URL. Shown until the first submission. */
    pageError?: string;
  } = $props();

  let identifier = $state('');
  let password = $state('');
  let error = $state('');
  let submitted = $state(false);
  let isLoading = $state(false);

  async function handleSubmit(e: Event) {
    e.preventDefault();
    error = '';
    submitted = true;
    isLoading = true;

    try {
      await signInWithPassword(identifier, password, m('auth.login.failed'));

      // Session completion is only needed after a successful password login.
      const { completeOriginAuthentication } = await import('$lib/auth/originAuthentication');
      const resumedReturnNavigation = await completeOriginAuthentication({
        resumeReturnNavigation: !isProviderContinuation(redirectUrl)
      });
      if (!resumedReturnNavigation) {
        await navigateAfterAuthentication(redirectUrl);
      }
    } catch (err) {
      error = errorMessage(err, m('auth.login.failed'));
    } finally {
      isLoading = false;
    }
  }
</script>

<PasswordSignInForm
  bind:identifier
  bind:password
  loading={isLoading}
  error={error || (submitted ? '' : pageError)}
  {compact}
  autofocus
  onsubmit={handleSubmit}
/>

<div class="mt-4 text-center">
  <a href={resolve('/forgot-password')} class="link">{m('auth.login.forgot_password')}</a>
</div>

{#if directRegistrationEnabled}
  <Divider label={m('common.or')} />

  <Button
    href={withReturnPath(resolve('/register'), redirectUrl)}
    variant="secondary"
    size={compact ? 'md' : 'lg'}
    fullWidth
  >
    {m('common.create_account')}
  </Button>
{/if}
