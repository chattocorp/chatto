<!--
@component

Desktop-only room header affordance for opening or hiding room extras panels.

**Props:**
- `activePanel` - Currently visible room sidebar panel, or `null` when hidden.
- `onToggle` - Called with the panel requested by the user.
-->
<script lang="ts">
  import type { RoomSidebarPanel } from './RoomSidebar.svelte';

  let {
    activePanel,
    onToggle
  }: {
    activePanel: RoomSidebarPanel | null;
    onToggle: (panel: RoomSidebarPanel) => void;
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
</script>

<span class="group/badges hidden items-center gap-1 lg:inline-flex">
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
      <span class={['pane-header-icon-glyph text-base', panel.icon]} aria-hidden="true"></span>
    </button>
  {/each}
</span>
