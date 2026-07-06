<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import EmptyState from './EmptyState.svelte';

  const componentDescription = `
    Use EmptyState when a panel or list has no actionable content yet. Keep the message short,
    specific to the missing content, and avoid wrapping it in an extra card.
  `.trim();

  const { Story } = defineMeta({
    title: 'UI/EmptyState',
    component: EmptyState,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: { component: componentDescription }
      }
    }
  });
</script>

<Story name="Default" asChild>
  <div class="flex h-96 flex-col">
    <EmptyState icon="uil--bell-slash" title="No notifications">You're all caught up!</EmptyState>
  </div>
</Story>

<Story name="Icon only" asChild>
  <div class="flex h-96 flex-col">
    <EmptyState icon="uil--message">Select a room from the sidebar to start chatting.</EmptyState>
  </div>
</Story>

<Story name="Title only" asChild>
  <div class="flex h-96 flex-col">
    <EmptyState title="Nothing here yet">Create your first room to get started.</EmptyState>
  </div>
</Story>
