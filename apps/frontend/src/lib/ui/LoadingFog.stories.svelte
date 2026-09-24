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
