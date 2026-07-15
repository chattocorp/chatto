<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import CollapsibleGroup from './CollapsibleGroup.svelte';

  const { Story } = defineMeta({
    title: 'UI/CollapsibleGroup',
    component: CollapsibleGroup,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  const rooms = [
    { id: 'general', label: 'general' },
    { id: 'announcements', label: 'announcements' },
    { id: 'support', label: 'support' }
  ];
</script>

{#snippet room(room: { id: string; label: string })}
  <button type="button" class="sidebar-item w-full text-left">
    <span class="sidebar-icon text-muted">#</span>
    <span>{room.label}</span>
  </button>
{/snippet}

<Story name="Expanded" asChild>
  <div class="w-64 rounded-lg bg-background p-3">
    <CollapsibleGroup
      label="Rooms"
      items={rooms}
      item={room}
      persistKey="storybook:collapsible-group:expanded"
    />
  </div>
</Story>

<Story name="Initially collapsed" asChild>
  <div class="w-64 rounded-lg bg-background p-3">
    <CollapsibleGroup
      label="Rooms"
      items={rooms}
      item={room}
      persistKey="storybook:collapsible-group:collapsed"
      defaultCollapsed
    />
  </div>
</Story>
