<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import AppHeader from './AppHeader.svelte';

  const { Story } = defineMeta({
    title: 'UI/App header',
    component: AppHeader,
    tags: ['autodocs']
  });
</script>

<Story name="Unauthenticated shell" asChild>
  <div class="max-w-4xl rounded-lg bg-background p-2 ring-1 ring-border">
    <AppHeader />
  </div>
</Story>
