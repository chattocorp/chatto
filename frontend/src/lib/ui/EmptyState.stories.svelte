<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import EmptyState from './EmptyState.svelte';

  const { Story } = defineMeta({
    title: 'UI/EmptyState',
    component: EmptyState,
    tags: ['autodocs']
  });
</script>

<Story name="Default" asChild>
  <div class="flex h-96 flex-col">
    <EmptyState icon="uil--bell-slash" title="No notifications">
      You're all caught up!
    </EmptyState>
  </div>
</Story>

<Story name="Icon only" asChild>
  <div class="flex h-96 flex-col">
    <EmptyState icon="uil--message">
      Select a room from the sidebar to start chatting.
    </EmptyState>
  </div>
</Story>

<Story name="Title only" asChild>
  <div class="flex h-96 flex-col">
    <EmptyState title="Nothing here yet">
      Create your first room to get started.
    </EmptyState>
  </div>
</Story>
