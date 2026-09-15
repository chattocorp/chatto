<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import RoomGroupSection from './RoomGroupSection.svelte';

  const { Story } = defineMeta({
    title: 'Chat/Room group section',
    component: RoomGroupSection,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'Collapsible room-group navigation section. Highlighted rooms remain visible while the section is collapsed.'
        }
      }
    }
  });

  const rooms = [
    { id: 'announcements', label: 'announcements', universal: true, highlighted: false },
    { id: 'random-chat', label: 'random-chat', universal: false, highlighted: true },
    { id: 'development', label: 'chatto-development', universal: false, highlighted: false }
  ];
</script>

<script lang="ts">
  let discoveryExpanded = $state(false);
</script>

{#snippet room(room: (typeof rooms)[number])}
  <button type="button" class={['sidebar-item text-start', room.highlighted && 'bg-surface']}>
    {#if room.universal}
      <span
        class="iconify sidebar-icon icon-[uil--globe] text-muted"
        role="img"
        aria-label="Universal room"
      ></span>
    {:else}
      <span class="sidebar-icon text-muted">#</span>
    {/if}
    <span class="min-w-0 flex-1 truncate">{room.label}</span>
  </button>
{/snippet}

<Story name="Adjacent sections" asChild>
  <div class="w-72 bg-background">
    <RoomGroupSection
      label="Chatto"
      items={rooms}
      item={room}
      persistKey="storybook:room-group-section:first"
      keepVisibleWhenCollapsed={(entry) => entry.highlighted}
    />
    <RoomGroupSection
      label="Chatto Cloud"
      items={rooms.slice(0, 2)}
      item={room}
      persistKey="storybook:room-group-section:second"
      keepVisibleWhenCollapsed={(entry) => entry.highlighted}
      separated
    />
  </div>
</Story>

<Story name="Collapsed with active room" asChild>
  <div class="w-72 bg-background">
    <RoomGroupSection
      label="Chatto"
      items={rooms}
      item={room}
      persistKey="storybook:room-group-section:collapsed"
      defaultCollapsed
      keepVisibleWhenCollapsed={(entry) => entry.highlighted}
    />
  </div>
</Story>

<Story name="Empty manageable group" asChild>
  <div class="w-72 bg-background">
    <RoomGroupSection
      label="Private projects"
      items={[]}
      item={room}
      persistKey="storybook:room-group-section:empty"
    />
  </div>
</Story>

<Story name="Room discovery footer" asChild>
  <div class="w-72 bg-background">
    <RoomGroupSection
      label="Projects"
      items={discoveryExpanded ? rooms : rooms.slice(0, 1)}
      item={room}
      persistKey="storybook:room-group-section:discovery"
    >
      {#snippet footer()}
        <button
          type="button"
          class="mini-icon-action w-full items-center gap-2 px-1 py-1 text-start text-xs"
          aria-expanded={discoveryExpanded}
          aria-label={discoveryExpanded ? 'Hide unjoined rooms' : 'Show 2 unjoined rooms'}
          onclick={() => {
            discoveryExpanded = !discoveryExpanded;
          }}
        >
          <span class="sidebar-icon" aria-hidden="true">{discoveryExpanded ? '−' : '+'}</span>
          <span>{discoveryExpanded ? 'Show less' : '2 more'}</span>
        </button>
      {/snippet}
    </RoomGroupSection>
  </div>
</Story>

<Story name="Collapsible content" asChild>
  <div class="w-72 bg-background">
    <RoomGroupSection
      label="What it can do"
      items={[{ id: 'permissions' }]}
      persistKey="storybook:room-group-section:content"
      separated
    >
      {#snippet item()}
        <div class="space-y-3 px-2 py-2">
          <p class="font-medium">Rooms it has joined</p>
          <ul class="list-disc space-y-1 ps-5">
            <li>Read all messages</li>
            <li>Reply in threads</li>
          </ul>
        </div>
      {/snippet}
    </RoomGroupSection>
  </div>
</Story>

<Story name="Profile content" asChild>
  <div class="w-64 bg-background">
    <RoomGroupSection label="Bio" items={[]} persistKey="storybook:profile-bio" separated>
      {#snippet content()}
        <p class="px-1 py-2">I build chat software and help with development.</p>
      {/snippet}
    </RoomGroupSection>
  </div>
</Story>
