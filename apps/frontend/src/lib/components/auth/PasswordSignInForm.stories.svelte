<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import PasswordSignInForm from './PasswordSignInForm.svelte';

  const { Story } = defineMeta({
    title: 'Auth/Password sign-in form',
    component: PasswordSignInForm,
    args: { onsubmit: (event: SubmitEvent) => event.preventDefault() }
  });
</script>

<Story name="Default" />
<Story name="Compact" args={{ compact: true }} />
<Story name="Submitting" args={{ loading: true, identifier: 'example', password: '' }} />
<Story name="Error" args={{ error: 'Sign-in failed. Please try again.' }} />
