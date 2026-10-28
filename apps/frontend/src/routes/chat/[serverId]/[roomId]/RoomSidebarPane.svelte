<script lang="ts">
  import type { ComponentProps } from 'svelte';
  import type RoomSidebar from './RoomSidebar.svelte';
  import { m } from '$lib/i18n/messages';
  import { LoadingFog, LoadRetry } from '$lib/ui';

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
    <LoadingFog class="m-4 min-h-0 flex-1" />
  {:then { default: RoomSidebar }}
    <RoomSidebar {...props} presentation={presentation === 'mobile' ? 'overlay' : 'desktop'} />
  {:catch}
    <LoadRetry class="min-h-0 flex-1" onretry={() => (roomSidebarLoadAttempt += 1)} />
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
