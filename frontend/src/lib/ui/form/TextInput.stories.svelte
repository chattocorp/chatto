<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import TextInput from './TextInput.svelte';

  const { Story } = defineMeta({
    title: 'Form/TextInput',
    component: TextInput,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  let value = $state('');
  let email = $state('');
  let pw = $state('');
  let withError = $state('not-an-email');
  let withDescription = $state('');
</script>

<Story name="Default" asChild>
  <div class="max-w-md">
    <TextInput id="default" label="Display name" bind:value placeholder="Jane Doe" />
  </div>
</Story>

<Story name="Required" asChild>
  <div class="max-w-md">
    <TextInput id="req" label="Login" bind:value required placeholder="jane" />
  </div>
</Story>

<Story name="With description" asChild>
  <div class="max-w-md">
    <TextInput
      id="desc"
      label="Login"
      bind:value={withDescription}
      description="Lowercase letters, numbers, and dashes only."
    />
  </div>
</Story>

<Story name="With error" asChild>
  <div class="max-w-md">
    <TextInput
      id="err"
      label="Email"
      bind:value={withError}
      type="email"
      error="Please enter a valid email address."
    />
  </div>
</Story>

<Story name="Disabled" asChild>
  <div class="max-w-md">
    <TextInput id="disabled" label="Login" value="jane" disabled />
  </div>
</Story>

<Story name="Email + password" asChild>
  <div class="flex max-w-md flex-col gap-4">
    <TextInput
      id="email"
      label="Email"
      type="email"
      bind:value={email}
      placeholder="you@example.com"
      autocomplete="email"
    />
    <TextInput
      id="pw"
      label="Password"
      type="password"
      bind:value={pw}
      autocomplete="current-password"
    />
  </div>
</Story>
