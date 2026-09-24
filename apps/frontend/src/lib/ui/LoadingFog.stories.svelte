<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import LoadingFog from './LoadingFog.svelte';

  const { Story } = defineMeta({
    title: 'UI/LoadingFog',
    component: LoadingFog,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  let showFog = $state(true);
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

<Story name="Enter and resolve" asChild>
  <div class="flex w-full max-w-xl flex-col gap-4">
    <button class="self-start rounded-md border px-3 py-1" onclick={() => (showFog = !showFog)}>
      Toggle loading
    </button>
    {#if showFog}
      <LoadingFog class="h-40 w-full" />
    {/if}
  </div>
</Story>
