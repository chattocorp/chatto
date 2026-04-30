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
  let search = $state('');
  let port = $state('8080');
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

<Story name="Leading icon" asChild>
  <div class="flex max-w-md flex-col gap-4">
    <TextInput
      id="search"
      label="Search"
      bind:value={search}
      placeholder="Search rooms…"
      leadingIcon="uil--search"
    />
    <TextInput
      id="email-icon"
      label="Email"
      bind:value={email}
      type="email"
      placeholder="you@example.com"
      leadingIcon="uil--envelope"
    />
    <TextInput
      id="pw-icon"
      label="Password"
      bind:value={pw}
      type="password"
      leadingIcon="uil--lock"
    />
  </div>
</Story>

<Story name="Trailing unit" asChild>
  <div class="max-w-md">
    <TextInput id="port" label="Port" bind:value={port} trailingText="tcp" />
  </div>
</Story>

<Story name="On a panel surface" asChild>
  <div class="rounded-lg border border-border bg-surface p-6">
    <p class="mb-4 text-sm text-muted">
      How the input looks inside a dialog or panel (<code>bg-surface</code>). Inputs sit on
      <code>--color-input</code>, distinct from any container surface.
    </p>
    <div class="flex flex-col gap-4">
      <TextInput id="panel-name" label="Room name" bind:value placeholder="general" required />
      <TextInput
        id="panel-search"
        label="Search members"
        bind:value={search}
        leadingIcon="uil--search"
        placeholder="Search…"
      />
    </div>
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
