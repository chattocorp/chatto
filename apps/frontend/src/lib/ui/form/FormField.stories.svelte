<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import FormField from './FormField.svelte';

  const { Story } = defineMeta({
    title: 'Form/FormField',
    component: FormField,
    tags: ['autodocs']
  });
</script>

<Story name="Description" asChild>
  <div class="max-w-md">
    <FormField
      id="storybook-username"
      label="Username"
      description="Lowercase letters, numbers, dots, and dashes."
      required
    >
      <input id="storybook-username" class="input" value="ada" />
    </FormField>
  </div>
</Story>

<Story name="Error" asChild>
  <div class="max-w-md">
    <FormField id="storybook-domain" label="Domain" error="Enter a valid hostname.">
      <input id="storybook-domain" class="input" value="not a hostname" aria-invalid="true" />
    </FormField>
  </div>
</Story>
