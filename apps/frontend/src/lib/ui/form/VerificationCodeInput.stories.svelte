<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import VerificationCodeInput from './VerificationCodeInput.svelte';

  const { Story } = defineMeta({
    title: 'Form/VerificationCodeInput',
    component: VerificationCodeInput,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  let value = $state('');
</script>

<Story name="Default" asChild>
  <div class="max-w-md">
    <VerificationCodeInput
      bind:value
      label="Verification code"
      digitLabel={(number) => `Digit ${number}`}
    />
  </div>
</Story>

<Story name="Complete" asChild>
  <div class="max-w-md">
    <VerificationCodeInput
      value="123456"
      label="Verification code"
      digitLabel={(number) => `Digit ${number}`}
    />
  </div>
</Story>

<Story name="Disabled" asChild>
  <div class="max-w-md">
    <VerificationCodeInput
      value="123456"
      disabled
      label="Verification code"
      digitLabel={(number) => `Digit ${number}`}
    />
  </div>
</Story>
