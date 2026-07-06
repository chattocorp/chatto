<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import TextInput from './TextInput.svelte';

  const componentDescription = `
    Use TextInput for short text, search, email, password, and numeric string entry. Labels stay
    visible, helper text explains constraints, and validation errors are rendered by the field so
    forms keep one consistent error treatment.
  `.trim();

  const { Story } = defineMeta({
    title: 'Form/TextInput',
    component: TextInput,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: { component: componentDescription }
      }
    }
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

<Story
  name="Default"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'The default field includes a persistent label and inherits the shared input surface.'
      }
    }
  }}
>
  <div class="max-w-md">
    <TextInput id="default" label="Display name" bind:value placeholder="Jane Doe" />
  </div>
</Story>

<Story
  name="Required"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Required fields use the same layout and rely on form-level validation for blocking submission.'
      }
    }
  }}
>
  <div class="max-w-md">
    <TextInput id="req" label="Login" bind:value required placeholder="jane" />
  </div>
</Story>

<Story
  name="With description"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Use descriptions for stable constraints or consequences, not transient validation messages.'
      }
    }
  }}
>
  <div class="max-w-md">
    <TextInput
      id="desc"
      label="Login"
      bind:value={withDescription}
      description="Lowercase letters, numbers, and dashes only."
    />
  </div>
</Story>

<Story
  name="With error"
  asChild
  parameters={{
    docs: {
      description: {
        story: 'Validation errors appear below the field with the shared error color and spacing.'
      }
    }
  }}
>
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

<Story
  name="Leading icon"
  asChild
  parameters={{
    docs: {
      description: {
        story: 'Leading icons are for recognizable input types such as search, email, and password.'
      }
    }
  }}
>
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

<Story
  name="Trailing unit"
  asChild
  parameters={{
    docs: {
      description: {
        story: 'Trailing text is for units or protocol hints that belong visually inside the field.'
      }
    }
  }}
>
  <div class="max-w-md">
    <TextInput id="port" label="Port" bind:value={port} trailingText="tcp" />
  </div>
</Story>

<Story
  name="On a panel surface"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Inputs remain distinct from panel backgrounds, including dialogs and raised surfaces.'
      }
    }
  }}
>
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
