<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import LoadingDots from './LoadingDots.svelte';

  const { Story } = defineMeta({
    title: 'UI/LoadingDots',
    component: LoadingDots,
    tags: ['autodocs']
  });
</script>

<Story name="Default" asChild>
  <LoadingDots class="py-4" />
</Story>

<Story name="Pending timeline" asChild>
  <div class="flex h-64 w-full max-w-xl flex-col justify-end rounded-lg border border-border">
    <LoadingDots class="py-4" label="Loading messages..." />
  </div>
</Story>
