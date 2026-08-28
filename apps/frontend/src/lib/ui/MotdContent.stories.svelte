<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import MotdContent from './MotdContent.svelte';

  const { Story } = defineMeta({
    title: 'UI/Message of the day content',
    component: MotdContent,
    tags: ['autodocs']
  });
</script>

<Story name="Markdown message" asChild>
  <div class="flex max-w-3xl rounded-lg bg-background p-4">
    <MotdContent motd="**Maintenance window:** Saturday, 10:00–11:00 UTC. [Read details](https://example.com)." />
  </div>
</Story>
