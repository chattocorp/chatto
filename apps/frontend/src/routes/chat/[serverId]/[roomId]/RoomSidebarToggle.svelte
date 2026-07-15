<!--
@component

Room header affordance for opening or hiding room extras panels.

**Props:**
- `activePanel` - Currently visible room sidebar panel, or `null` when hidden.
- `panels` - Panel buttons to show. Defaults to every room sidebar panel.
- `onToggle` - Called with the panel requested by the user.
- `mode` - Responsive visibility for the toggle group.
-->
<script lang="ts">
  import type { RoomSidebarPanel } from './RoomSidebar.svelte';

  let {
    activePanel,
    panels,
    onToggle,
    mode = 'desktop',
    hasActiveCall = false
  }: {
    activePanel: RoomSidebarPanel | null;
    panels?: RoomSidebarPanel[];
    onToggle: (panel: RoomSidebarPanel) => void;
    mode?: 'desktop' | 'mobile' | 'always';
    hasActiveCall?: boolean;
  } = $props();

  const panelDefinitions: {
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
    },
    {
      id: 'call',
      icon: 'uil--phone',
      showLabel: 'Show call',
      hideLabel: 'Hide call'
    }
  ];

  const visiblePanels = $derived(
    panels ? panelDefinitions.filter((panel) => panels.includes(panel.id)) : panelDefinitions
  );

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
  {#each visiblePanels as panel (panel.id)}
    {@const isActive = activePanel === panel.id}
    {@const isActiveCallPanel = panel.id === 'call' && hasActiveCall}
    {@const shouldPulseCallIcon = isActiveCallPanel && !isActive}
    <button
      type="button"
      class={[
        'group/pane-header-icon-button pane-header-icon-button',
        isActive && 'pane-header-icon-button-active',
        isActiveCallPanel && 'text-action'
      ]}
      onclick={() => onToggle(panel.id)}
      title={isActive ? panel.hideLabel : panel.showLabel}
      aria-label={isActive ? panel.hideLabel : panel.showLabel}
      aria-pressed={isActive}
    >
      <span class="relative inline-flex">
        {#if shouldPulseCallIcon}
          <span
            class={['absolute inset-0 pane-header-icon-glyph animate-ping opacity-45', panel.icon]}
            aria-hidden="true"
            data-testid="active-call-pulse-icon"
          ></span>
        {/if}
        <span
          class={[
            'relative pane-header-icon-glyph',
            panel.icon,
            isActiveCallPanel && 'text-action'
          ]}
          aria-hidden="true"
        ></span>
      </span>
    </button>
  {/each}
</span>
