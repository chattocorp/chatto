<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import UnreadDot from './UnreadDot.svelte';
  import NotificationBadge from './NotificationBadge.svelte';

  const { Story } = defineMeta({
    title: 'UI/Navigation states',
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'Shared sidebar navigation states. Blue marks the current route; neutral text and dots mark unread content.'
        }
      }
    }
  });
</script>

<Story name="Route and attention" asChild>
  <nav class="w-64 rounded-lg bg-background p-2 ring-1 ring-border" aria-label="Example rooms">
    <a href="#current" class="sidebar-item" aria-current="page">
      <span class="sidebar-icon text-muted">#</span>
      <span class="flex-1 truncate">General</span>
    </a>
    <a href="#unread" class="sidebar-item sidebar-item-attention">
      <span class="sidebar-icon text-text-top">#</span>
      <span class="flex-1 truncate">Design review</span>
      <UnreadDot color="neutral" />
    </a>
    <a href="#notification" class="sidebar-item">
      <span class="sidebar-icon text-muted">#</span>
      <span class="flex-1 truncate">Release planning</span>
      <NotificationBadge count={3} color="warning" />
    </a>
  </nav>
</Story>
