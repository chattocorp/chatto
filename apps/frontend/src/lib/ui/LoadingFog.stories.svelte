<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import LoadingFog from './LoadingFog.svelte';

  const { Story } = defineMeta({
    title: 'UI/LoadingFog',
    component: LoadingFog,
    tags: ['autodocs']
  });
</script>

<Story name="Content panel" asChild>
  <LoadingFog class="h-64 w-full max-w-xl" />
</Story>

<Story name="Compact content" asChild>
  <LoadingFog class="h-10 w-48" />
</Story>

<Story name="Several panels" asChild>
  <div class="grid w-full max-w-3xl gap-4 sm:grid-cols-2">
    <LoadingFog class="h-40" />
    <LoadingFog class="h-40" />
    <LoadingFog class="h-24 sm:col-span-2" />
  </div>
</Story>
