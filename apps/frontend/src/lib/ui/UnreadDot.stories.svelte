<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import UnreadDot from './UnreadDot.svelte';

  const { Story } = defineMeta({
    title: 'UI/UnreadDot',
    component: UnreadDot,
    tags: ['autodocs']
  });
</script>

<Story name="Colors" asChild>
  <div class="flex items-center gap-6">
    <div class="flex items-center gap-2">
      <UnreadDot color="warning" />
      <span class="text-sm">warning</span>
    </div>
    <div class="flex items-center gap-2">
      <UnreadDot color="action" />
      <span class="text-sm">action</span>
    </div>
    <div class="flex items-center gap-2">
      <UnreadDot color="muted" />
      <span class="text-sm">muted</span>
    </div>
  </div>
</Story>

<Story name="Overlay" asChild>
  <div class="flex gap-6">
    <div class="relative inline-block rounded-md bg-surface-emphasized p-3">
      <span class="iconify text-xl uil--bell"></span>
      <UnreadDot overlay color="warning" class="absolute -top-1 -right-1" />
    </div>
    <div class="relative inline-block rounded-md bg-surface-emphasized p-3">
      <span class="iconify text-xl uil--message"></span>
      <UnreadDot overlay color="action" class="absolute -top-1 -right-1" />
    </div>
  </div>
</Story>
