<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import NotificationBadge from './NotificationBadge.svelte';

  const { Story } = defineMeta({
    title: 'UI/NotificationBadge',
    component: NotificationBadge,
    tags: ['autodocs']
  });
</script>

<Story name="Counts" asChild>
  <div class="flex items-center gap-4">
    <NotificationBadge count={1} />
    <NotificationBadge count={12} />
    <NotificationBadge count={120} />
  </div>
</Story>

<Story name="Overlay" asChild>
  <div class="relative inline-block rounded-md bg-surface-200 p-3">
    <span class="iconify uil--server text-xl"></span>
    <NotificationBadge count={7} overlay class="absolute -top-1 -right-1" />
  </div>
</Story>
