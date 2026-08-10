<script lang="ts">
  import type { ComponentProps } from 'svelte';
  import type RoomSidebar from './RoomSidebar.svelte';
  import { fly } from 'svelte/transition';
  import { m } from '$lib/i18n/messages';
  import { fromInlineEndOffset } from '$lib/i18n/direction';
  import { expoOutTransition } from '$lib/ui/motion';

  let roomSidebarModule: Promise<typeof import('./RoomSidebar.svelte')> | null = null;
  let roomSidebarLoadAttempt = $state(0);

  function loadRoomSidebar(_attempt: number) {
    roomSidebarModule ??= import('./RoomSidebar.svelte').catch((error: unknown) => {
      roomSidebarModule = null;
      throw error;
    });
    return roomSidebarModule;
  }

  let {
    presentation,
    sidebarProps
  }: {
    presentation: 'mobile' | 'desktop';
    sidebarProps: (ComponentProps<typeof RoomSidebar> & { onClose: () => void }) | null;
  } = $props();

  const maximized = $derived(sidebarProps?.maximized ?? false);
</script>

{#snippet sidebar(props: NonNullable<typeof sidebarProps>)}
  {#await loadRoomSidebar(roomSidebarLoadAttempt)}
    <div
      class="flex min-h-0 flex-1 items-center justify-center p-4 text-sm text-muted"
      aria-busy="true"
    >
      {m('common.loading')}
    </div>
  {:then { default: RoomSidebar }}
    <RoomSidebar {...props} presentation={presentation === 'mobile' ? 'overlay' : 'desktop'} />
  {:catch}
    <div class="flex min-h-0 flex-1 flex-col items-center justify-center gap-3 p-4 text-center">
      <p class="text-sm text-muted">{m('common.error.network')}</p>
      <button type="button" class="btn-secondary" onclick={() => (roomSidebarLoadAttempt += 1)}>
        {m('common.retry')}
      </button>
    </div>
  {/await}
{/snippet}

{#if presentation === 'mobile'}
  {#if sidebarProps}
    <button
      type="button"
      class="absolute inset-0 z-10 bg-transparent lg:hidden"
      aria-label={m('room.close_extras')}
      onclick={sidebarProps.onClose}
    ></button>
    <div
      class="absolute inset-y-0 end-0 z-20 flex min-h-0 w-full min-w-0 flex-col overflow-hidden border-s border-border bg-background inline-end-overlay-shadow sm:w-[90%] lg:hidden"
      data-testid="room-sidebar-mobile-pane"
      transition:fly={{ x: fromInlineEndOffset(300), ...expoOutTransition() }}
    >
      {@render sidebar(sidebarProps)}
    </div>
  {/if}
{:else if sidebarProps}
  <div
    class={['hidden min-h-0 min-w-0 lg:flex', maximized ? 'flex-1' : 'shrink-0']}
    data-testid="room-sidebar-desktop-pane"
  >
    {@render sidebar(sidebarProps)}
  </div>
{/if}
