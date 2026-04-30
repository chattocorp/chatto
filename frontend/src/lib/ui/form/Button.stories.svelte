<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Button from './Button.svelte';

  const { Story } = defineMeta({
    title: 'Form/Button',
    component: Button,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  const variants = ['primary', 'secondary', 'ghost', 'danger'] as const;
  const sizes = ['sm', 'default', 'lg'] as const;
</script>

<Story name="Variants" asChild>
  <div class="flex flex-wrap items-center gap-3">
    {#each variants as variant (variant)}
      <Button {variant}>{variant}</Button>
    {/each}
  </div>
</Story>

<Story name="Sizes" asChild>
  <div class="flex flex-wrap items-center gap-3">
    {#each sizes as size (size)}
      <Button {size}>{size}</Button>
    {/each}
  </div>
</Story>

<Story name="Loading" asChild>
  <div class="flex flex-wrap items-center gap-3">
    <Button loading>Saving...</Button>
    <Button loading loadingText="Sending...">Send</Button>
    <Button variant="danger" loading>Deleting...</Button>
  </div>
</Story>

<Story name="Disabled" asChild>
  <div class="flex flex-wrap items-center gap-3">
    {#each variants as variant (variant)}
      <Button {variant} disabled>{variant}</Button>
    {/each}
  </div>
</Story>

<Story name="As link" asChild>
  <Button href="https://www.chatto.run" variant="secondary">Visit chatto.run</Button>
</Story>

<Story name="Full width" asChild>
  <div class="max-w-md">
    <Button fullWidth>Continue</Button>
  </div>
</Story>
