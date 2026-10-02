<!--
@component

Room header affordance for opening or hiding room extras panels. Each toggle
has a stable accessible name and expresses its state with `aria-pressed`. When
the pane header is at least 80 rem wide, the toggles also show their labels.

**Props:**
- `activePanel` - Currently visible room sidebar panel, or `null` when hidden.
- `panels` - Panel buttons to show. Defaults to every room sidebar panel.
- `onToggle` - Called with the panel requested by the user.
- `mode` - Responsive visibility for the toggle group.
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { UnreadDot } from '$lib/ui';
  import type { RoomSidebarPanel } from './RoomSidebar.svelte';

  let {
    activePanel,
    panels,
    onToggle,
    mode = 'desktop',
    hasActiveCall = false,
    hasUnseenPins = false
  }: {
    activePanel: RoomSidebarPanel | null;
    panels?: RoomSidebarPanel[];
    onToggle: (panel: RoomSidebarPanel) => void;
    mode?: 'desktop' | 'mobile' | 'always';
    hasActiveCall?: boolean;
    hasUnseenPins?: boolean;
  } = $props();

  const panelDefinitions = $derived<
    {
      id: RoomSidebarPanel;
      icon: string;
      /** Visible label, shown when the pane header has room. */
      label: string;
      /** Accessible name when it differs from the label. It contains the label. */
      name?: string;
    }[]
  >([
    { id: 'pins', icon: 'icon-[mdi--pin-outline]', label: m('room.sidebar.pins') },
    { id: 'members', icon: 'icon-[uil--users-alt]', label: m('room.sidebar.members') },
    {
      id: 'search',
      icon: 'icon-[uil--search]',
      label: m('room.sidebar.search'),
      name: m('search.in_room')
    },
    { id: 'files', icon: 'icon-[uil--paperclip]', label: m('room.sidebar.files') },
    { id: 'call', icon: 'icon-[uil--phone]', label: m('room.sidebar.call') }
  ]);

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
    {@const showUnseenPin = panel.id === 'pins' && hasUnseenPins && !isActive}
    {@const name = panel.name ?? panel.label}
    <button
      type="button"
      class={[
        'pane-header-label-button',
        isActive && 'pane-header-icon-button-active',
        isActiveCallPanel && 'text-action'
      ]}
      onclick={() => onToggle(panel.id)}
      title={name}
      aria-label={showUnseenPin ? `${name}. ${m('room.pins.unseen')}` : name}
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
        {#if showUnseenPin}
          <UnreadDot class="absolute -end-1 -top-1 ring-2 ring-surface" testid="unseen-pin-dot" />
        {/if}
      </span>
      <span class="hidden @min-[80rem]/pane-header:inline" aria-hidden="true">{panel.label}</span>
    </button>
  {/each}
</span>
