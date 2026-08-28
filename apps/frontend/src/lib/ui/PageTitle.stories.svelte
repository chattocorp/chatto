<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import PageTitle from './PageTitle.svelte';

  const { Story } = defineMeta({
    title: 'UI/Page title',
    component: PageTitle,
    tags: ['autodocs']
  });
</script>

<Story name="Sets document title" asChild>
  <PageTitle title="Server settings" />
  <div class="max-w-xl rounded-lg bg-background p-6">
    <h1 class="text-xl font-semibold">Server settings</h1>
    <p class="mt-2 text-muted">This mountable helper updates the browser document title.</p>
  </div>
</Story>
