<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import FormError from './FormError.svelte';

  const { Story } = defineMeta({
    title: 'Form/FormError',
    component: FormError,
    tags: ['autodocs']
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
