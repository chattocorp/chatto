<script lang="ts">
  import type { Snippet } from 'svelte';
  import ServerGutter from '$lib/ServerGutter.svelte';
  import { SIDEBAR_PANEL_WIDTH_PX } from '$lib/hooks/useSidebarSwipe.svelte';
  import * as m from '$lib/i18n/messages';
  import { sidebarNav } from '$lib/state/globals.svelte';

  let { children }: { children?: Snippet } = $props();

  const progress = $derived(sidebarNav.isMobile ? sidebarNav.progress : 1);
  const dragging = $derived(sidebarNav.dragOffset !== null);
  const mobileClosed = $derived(sidebarNav.isMobile && progress === 0 && !dragging);
  const tx = $derived((progress - 1) * SIDEBAR_PANEL_WIDTH_PX);
</script>

{#if sidebarNav.isMobile}
  <button
    type="button"
    data-app-sidebar="true"
    data-testid="mobile-sidebar-backdrop"
    class={[
      'fixed inset-0 top-11 z-40 touch-none bg-black/50 md:hidden',
      !dragging && 'transition-opacity duration-200',
      mobileClosed && 'pointer-events-none'
    ]}
    style:opacity={progress}
    disabled={mobileClosed}
    tabindex={mobileClosed ? -1 : 0}
    aria-hidden={mobileClosed}
    onclick={() => sidebarNav.close()}
    aria-label={m['common.close_sidebar']()}
  ></button>
{/if}

<div class="flex min-h-0 flex-1 flex-row">
  <div
    data-app-sidebar="true"
    data-testid="mobile-sidebar-panel"
    class={[
      'z-50 min-h-0 flex-col self-stretch bg-background',
      'max-md:fixed max-md:top-11 max-md:bottom-0 max-md:left-0 max-md:w-17 max-md:touch-pan-y',
      // Mobile: always rendered so we can animate transform.
      // Desktop: hide entirely when closed (no overlay; layout reflows).
      sidebarNav.isMobile ? 'flex' : sidebarNav.isOpen ? 'flex' : 'hidden',
      // Mobile-only: hide via `visibility: hidden` after the close
      // transition, so Playwright / accessibility tooling correctly see
      // the sidebar as not-visible while the slide-out animation works.
      mobileClosed && 'sidebar-mobile-closed',
      !dragging && 'sidebar-mobile-anim'
    ]}
    style:transform={sidebarNav.isMobile ? `translateX(${tx}px)` : undefined}
  >
    <ServerGutter />
  </div>

  {@render children?.()}
</div>

<style>
  /*
		Mobile sidebar animation — slide via transform, plus a delayed visibility
		swap so the off-screen panel is reported as `visibility: hidden` (not just
		visually hidden by transform) once the close animation finishes. This
		matters for accessibility tooling and Playwright's `toBeVisible()`.

		Open  → transform animates 200ms, visibility flips to `visible` immediately.
		Close → transform animates 200ms, visibility flips to `hidden` AFTER 200ms.
	*/
  @media (max-width: 767px) {
    :global(.sidebar-mobile-anim) {
      visibility: visible;
      transition:
        transform 200ms ease-out,
        visibility 0s linear 0s;
    }
    :global(.sidebar-mobile-anim.sidebar-mobile-closed) {
      visibility: hidden;
      transition:
        transform 200ms ease-out,
        visibility 0s linear 200ms;
    }
  }
</style>
