<script lang="ts">
  import type { ComponentProps } from 'svelte';
  import { fly } from 'svelte/transition';
  import * as m from '$lib/i18n/messages';
  import RoomSidebar from './RoomSidebar.svelte';

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
  <RoomSidebar {...props} presentation={presentation === 'mobile' ? 'overlay' : 'desktop'} />
{/snippet}

{#if presentation === 'mobile'}
  {#if sidebarProps}
    <button
      type="button"
      class="absolute inset-0 z-10 bg-transparent lg:hidden"
      aria-label={m['room.close_extras']()}
      onclick={sidebarProps.onClose}
    ></button>
    <div
      class="absolute inset-y-0 right-0 z-20 flex min-h-0 w-full min-w-0 flex-col overflow-hidden border-l border-border bg-background shadow-[-4px_0_12px_rgba(0,0,0,0.15)] sm:w-[90%] lg:hidden"
      data-testid="room-sidebar-mobile-pane"
      transition:fly={{ x: 300, duration: 200 }}
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
