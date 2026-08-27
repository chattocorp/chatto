<!--
@component

The **Server Sidebar** — wider sidebar after the Server Gutter. Owns the
secondary pane's chrome: positioning, mobile slide-in/-out, resize handle, and
the optional current-user bar pinned to the bottom. Server pages provide the
footer; app-wide preferences reuse the same shell without it. The actual
contents are passed through the `children` snippet.

See the "UI" section of `docs/GLOSSARY.md`.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import { SIDEBAR_PANEL_WIDTH_PX } from '$lib/hooks/useSidebarSwipe.svelte';
  import { sidebarNav } from '$lib/state/globals.svelte';
  import { serverSidebarWidth } from '$lib/state/serverSidebarWidth.svelte';
  import {
    SERVER_SIDEBAR_MAX_WIDTH,
    SERVER_SIDEBAR_MIN_WIDTH
  } from '$lib/storage/serverSidebarWidth';
  import { m } from '$lib/i18n/messages';
  import CurrentUserBar from './CurrentUserBar.svelte';
  import ResizeHandle from './ResizeHandle.svelte';

  let {
    children,
    width,
    mobileWidth = 'max-md:w-64',
    showCurrentUserBar = true
  }: {
    children: Snippet;
    /** Optional Tailwind class to lock the desktop width (e.g. "md:w-56"). When
     *  omitted, the sidebar uses the user's persisted resizable width and shows
     *  a drag handle. */
    width?: string;
    mobileWidth?: string;
    /** Whether to render the active server's user card in the footer. App-wide
     *  sidebars have no server scope and omit it. */
    showCurrentUserBar?: boolean;
  } = $props();

  // On mobile the panel slides as a single unit with the Server Gutter — both
  // apply the same translateX driven by `sidebarNav.progress`. On desktop the
  // sidebar toggles via `hidden`/`flex` (no overlay; layout reflows).
  const tx = $derived(sidebarNav.isMobile ? (sidebarNav.progress - 1) * SIDEBAR_PANEL_WIDTH_PX : 0);
  const dragging = $derived(sidebarNav.dragOffset !== null);
  const mobileClosed = $derived(sidebarNav.isMobile && sidebarNav.progress === 0 && !dragging);
  const resizable = $derived(!width);
</script>

<div
  data-app-sidebar="true"
  data-testid="server-sidebar"
  class={[
    'server-sidebar relative z-50 flex min-w-0 flex-col overflow-hidden border-e border-border bg-background',
    width,
    mobileWidth,
    'md:flex-initial',
    // Mobile: fixed overlay positioned after the Server Gutter (~68px); touch-pan-y so
    // vertical scroll inside the panel still works while horizontal pans go to
    // the sidebar swipe action.
    'max-md:fixed max-md:start-17 max-md:top-11 max-md:bottom-0 max-md:touch-pan-y',
    // Mobile: always rendered so the slide animation is visible.
    // Desktop: hide entirely when closed.
    sidebarNav.isMobile ? '' : sidebarNav.isOpen ? '' : 'hidden',
    // Mobile-only: become `visibility: hidden` once the slide-out animation
    // completes (see .sidebar-mobile-anim styles in MobileSidebarChrome.svelte) so
    // accessibility tools and Playwright `toBeVisible()` agree the panel is
    // hidden, not just translated off-screen.
    mobileClosed && 'sidebar-mobile-closed',
    !dragging && 'sidebar-mobile-anim',
    resizable && 'md:w-[var(--server-sidebar-width)]'
  ]}
  style:--server-sidebar-width={resizable ? `${serverSidebarWidth.value}px` : undefined}
  style:transform={sidebarNav.isMobile
    ? `translateX(calc(${tx}px * var(--inline-direction)))`
    : undefined}
>
  {@render children()}
  {#if showCurrentUserBar}
    <CurrentUserBar />
  {/if}
  {#if resizable && !sidebarNav.isMobile}
    <ResizeHandle
      width={serverSidebarWidth.value}
      min={SERVER_SIDEBAR_MIN_WIDTH}
      max={SERVER_SIDEBAR_MAX_WIDTH}
      onResize={(w) => serverSidebarWidth.set(w)}
      onReset={() => serverSidebarWidth.reset()}
      label={m('ui.resize_handle.resize_sidebar')}
      edge="end"
    />
  {/if}
</div>
