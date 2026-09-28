<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import LoadRetry from './LoadRetry.svelte';

  const componentDescription = `
    Use LoadRetry where a lazily loaded part of the interface failed to load. It replaces
    the part in place and lets the user start a new load attempt.
  `.trim();

  const { Story } = defineMeta({
    title: 'UI/LoadRetry',
    component: LoadRetry,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: { component: componentDescription }
      }
    }
  });
</script>

<Story name="Default" asChild>
  <div class="w-80 rounded-md border border-border">
    <LoadRetry onretry={() => {}} />
  </div>
</Story>

<Story name="With another action" asChild>
  <div class="flex h-72 w-96 rounded-md border border-border">
    <LoadRetry
      class="flex-1"
      onretry={() => {}}
      secondaryAction={{ label: 'Close thread', onclick: () => {} }}
    />
  </div>
</Story>
