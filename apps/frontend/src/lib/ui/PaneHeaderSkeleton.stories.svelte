<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import PaneHeaderSkeleton from './PaneHeaderSkeleton.svelte';

  const { Story } = defineMeta({
    title: 'UI/PaneHeaderSkeleton',
    component: PaneHeaderSkeleton,
    tags: ['autodocs']
  });
</script>

<Story name="Default" asChild>
  <div class="max-w-xl rounded-lg bg-surface-100 p-4">
    <PaneHeaderSkeleton />
  </div>
</Story>

<Story name="Single action" asChild>
  <div class="max-w-xl rounded-lg bg-surface-100 p-4">
    <PaneHeaderSkeleton buttons={1} />
  </div>
</Story>
