<!--
@component

Room header affordance for opening or hiding room extras panels.

**Props:**
- `activePanel` - Currently visible room sidebar panel, or `null` when hidden.
- `onToggle` - Called with the panel requested by the user.
- `mode` - Responsive visibility for the toggle group.
-->
<script lang="ts">
  import type { RoomSidebarPanel } from './RoomSidebar.svelte';

  let {
    activePanel,
    onToggle,
    mode = 'desktop'
  }: {
    activePanel: RoomSidebarPanel | null;
    onToggle: (panel: RoomSidebarPanel) => void;
    mode?: 'desktop' | 'mobile' | 'always';
  } = $props();

  const panels: {
    id: RoomSidebarPanel;
    icon: string;
    showLabel: string;
    hideLabel: string;
  }[] = [
    {
      id: 'members',
      icon: 'uil--users-alt',
      showLabel: 'Show members',
      hideLabel: 'Hide members'
    },
    {
      id: 'files',
      icon: 'uil--paperclip',
      showLabel: 'Show files',
      hideLabel: 'Hide files'
    }
  ];

  const visibilityClass = $derived.by(() => {
    switch (mode) {
      case 'mobile':
        return 'inline-flex lg:hidden';
      case 'always':
        return 'inline-flex';
      case 'desktop':
        return 'hidden lg:inline-flex';
    }
  });
</script>

<span
  class={['group/badges items-center gap-1', visibilityClass]}
  data-testid="room-sidebar-toggle"
>
  {#each panels as panel (panel.id)}
    {@const isActive = activePanel === panel.id}
    <button
      type="button"
      class={[
        'group/pane-header-icon-button pane-header-icon-button',
        isActive && 'pane-header-icon-button-active'
      ]}
      onclick={() => onToggle(panel.id)}
      title={isActive ? panel.hideLabel : panel.showLabel}
      aria-label={isActive ? panel.hideLabel : panel.showLabel}
      aria-pressed={isActive}
    >
      <span class={['pane-header-icon-glyph', panel.icon]} aria-hidden="true"></span>
    </button>
  {/each}
</span>
