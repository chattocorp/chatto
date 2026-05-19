<!--
@component

The **Server Sidebar** — wider sidebar to the right of the Server Gutter,
scoped to a single server. Owns the per-server pane's chrome: positioning,
mobile slide-in/-out, resize handle, and the current-user bar pinned to the
bottom. The actual contents (server banner + room list, settings nav, admin
nav, …) are passed in via the `children` snippet by `Chrome.svelte`.

See the "UI" section of `docs/GLOSSARY.md`.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import { SIDEBAR_PANEL_WIDTH_PX, sidebarSwipe } from '$lib/hooks/useSidebarSwipe.svelte';
  import { sidebarNav } from '$lib/state/globals.svelte';
  import { serverSidebarWidth } from '$lib/state/serverSidebarWidth.svelte';
  import {
    SERVER_SIDEBAR_MAX_WIDTH,
    SERVER_SIDEBAR_MIN_WIDTH
  } from '$lib/storage/serverSidebarWidth';
  import CurrentUserBar from './CurrentUserBar.svelte';
  import ResizeHandle from './ResizeHandle.svelte';

  let {
    children,
    width,
    mobileWidth = 'max-md:w-64'
  }: {
    children: Snippet;
    /** Optional Tailwind class to lock the desktop width (e.g. "md:w-56"). When
     *  omitted, the sidebar uses the user's persisted resizable width and shows
     *  a drag handle. */
    width?: string;
    mobileWidth?: string;
  } = $props();

  // On mobile the panel slides as a single unit with the Server Gutter — both
  // apply the same translateX driven by `sidebarNav.progress`. On desktop the
  // sidebar toggles via `hidden`/`flex` (no overlay; layout reflows).
  const tx = $derived(
    sidebarNav.isMobile ? (sidebarNav.progress - 1) * SIDEBAR_PANEL_WIDTH_PX : 0
  );
  const dragging = $derived(sidebarNav.dragOffset !== null);
  const resizable = $derived(!width);
</script>

<div
  use:sidebarSwipe
  class={[
    'server-sidebar relative z-50 flex min-w-0 flex-col overflow-hidden border-r border-border bg-background',
    width,
    mobileWidth,
    'md:flex-initial',
    // Mobile: fixed overlay positioned after the Server Gutter (~68px); touch-pan-y so
    // vertical scroll inside the panel still works while horizontal pans go to
    // the sidebar swipe action.
    'max-md:fixed max-md:top-11 max-md:bottom-0 max-md:left-17 max-md:touch-pan-y',
    // Mobile: always rendered so the slide animation is visible.
    // Desktop: hide entirely when closed.
    sidebarNav.isMobile ? '' : sidebarNav.isOpen ? '' : 'hidden',
    // Mobile-only: become `visibility: hidden` once the slide-out animation
    // completes (see .sidebar-mobile-anim styles in routes/+layout.svelte) so
    // accessibility tools and Playwright `toBeVisible()` agree the panel is
    // hidden, not just translated off-screen.
    sidebarNav.isMobile && sidebarNav.progress === 0 && !dragging && 'max-md:invisible',
    !dragging && 'sidebar-mobile-anim',
    resizable && 'server-sidebar--resizable'
  ]}
  style:--server-sidebar-width={resizable ? `${serverSidebarWidth.value}px` : undefined}
  style:transform={sidebarNav.isMobile ? `translateX(${tx}px)` : undefined}
>
  {@render children()}
  <CurrentUserBar />
  {#if resizable && !sidebarNav.isMobile}
    <ResizeHandle
      width={serverSidebarWidth.value}
      min={SERVER_SIDEBAR_MIN_WIDTH}
      max={SERVER_SIDEBAR_MAX_WIDTH}
      onResize={(w) => serverSidebarWidth.set(w)}
      onReset={() => serverSidebarWidth.reset()}
      label="Resize sidebar"
    />
  {/if}
</div>

<style>
  @media (min-width: 768px) {
    .server-sidebar--resizable {
      width: var(--server-sidebar-width);
    }
  }
</style>
