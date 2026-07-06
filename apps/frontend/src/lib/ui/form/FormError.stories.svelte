<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import FormError from './FormError.svelte';

  const componentDescription = `
    Use FormError for form-level or field-adjacent validation messages that should use the shared
    error color, spacing, and conditional rendering behavior.
  `.trim();

  const { Story } = defineMeta({
    title: 'Form/FormError',
    component: FormError,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: { component: componentDescription }
      }
    }
  });
</script>

<Story name="Default" asChild>
  <div class="max-w-md">
    <FormError error="Something went wrong. Please try again." />
  </div>
</Story>

<Story name="Empty (renders nothing)" asChild>
  <div class="max-w-md">
    <p class="text-sm text-muted">No error → component renders nothing:</p>
    <FormError error={null} />
  </div>
</Story>
