<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import LoadingPage from './LoadingPage.svelte';

  const { Story } = defineMeta({
    title: 'UI/LoadingPage',
    component: LoadingPage,
    tags: ['autodocs']
  });
</script>

<Story name="Default" asChild>
  <div class="h-72">
    <LoadingPage />
  </div>
</Story>

<Story name="Specific operation" asChild>
  <div class="h-72">
    <LoadingPage message="Loading room history…" />
  </div>
</Story>
