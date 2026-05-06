<script lang="ts">
  import type { Snippet } from 'svelte';
  import { SIDEBAR_PANEL_WIDTH_PX, sidebarSwipe } from '$lib/hooks/useSidebarSwipe.svelte';
  import { sidebarNav } from '$lib/state/globals.svelte';

  let {
    children,
    width = 'md:w-64',
    mobileWidth = 'max-md:w-64'
  }: {
    children: Snippet;
    width?: string;
    mobileWidth?: string;
  } = $props();

  // On mobile the panel slides as a single unit with the SpaceList — both apply
  // the same translateX driven by `sidebarNav.progress`. On desktop the sidebar
  // toggles via `hidden`/`flex` (no overlay; layout reflows).
  const tx = $derived(
    sidebarNav.isMobile ? (sidebarNav.progress - 1) * SIDEBAR_PANEL_WIDTH_PX : 0
  );
  const dragging = $derived(sidebarNav.dragOffset !== null);
</script>

<!--
	Secondary sidebar (room list, DM conversations, etc.)
	- Desktop: shown in normal flow with fixed width
	- Mobile: fixed overlay positioned after SpaceList; slides in/out with the panel
-->
<div
  use:sidebarSwipe
  class={[
    'z-50 flex min-w-0 flex-col overflow-hidden border-r border-border bg-background',
    width,
    mobileWidth,
    'md:flex-initial',
    // Mobile: fixed overlay positioned after SpaceList (~68px); touch-pan-y so
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
    !dragging && 'sidebar-mobile-anim'
  ]}
  style:transform={sidebarNav.isMobile ? `translateX(${tx}px)` : undefined}
>
  {@render children()}
</div>
