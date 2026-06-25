<script lang="ts">
  import { afterNavigate, goto } from '$app/navigation';
  import { page } from '$app/state';
  import * as m from '$lib/i18n/messages';
  import { onNotificationClick } from '$lib/notifications/pushNotifications';
  import ServerGutter from '$lib/ServerGutter.svelte';
  import { setAuthServerInfo } from '$lib/components/authServerInfo';
  import ConnectionIndicator from '$lib/components/ConnectionIndicator.svelte';
  import ConnectionProvider from '$lib/components/ConnectionProvider.svelte';
  import GlobalKeyboardShortcuts from '$lib/components/GlobalKeyboardShortcuts.svelte';
  import IdleTracker from '$lib/components/IdleTracker.svelte';
  import UpdateNotifier from '$lib/components/UpdateNotifier.svelte';
  import { usePageTitle, usePinchZoomPrevention, useVisualViewport } from '$lib/hooks';
  import { SIDEBAR_PANEL_WIDTH_PX, sidebarSwipe } from '$lib/hooks/useSidebarSwipe.svelte';
  import { installAssetProxyResyncHandler, syncAssetProxyServers } from '$lib/pwa/assetProxy';
  import { sidebarNav } from '$lib/state/globals.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { useServerRegistry } from '$lib/state/server/useServerRegistry.svelte';
  import { ToastContainer } from '$lib/ui/toast';
  import { AppHeader, Frame } from '$lib/ui';
  import '../app.css';

  let { data, children } = $props();
  let modalContainerModule: Promise<typeof import('./chat/ModalContainer.svelte')> | null = null;

  function loadModalContainer() {
    modalContainerModule ??= import('./chat/ModalContainer.svelte');
    return modalContainerModule;
  }

  setAuthServerInfo(() => data.serverInfo);
  useServerRegistry(() => data.user);
  useVisualViewport();
  usePinchZoomPrevention();

  $effect(() => {
    if (typeof navigator === 'undefined' || !('serviceWorker' in navigator)) {
      return;
    }

    const sync = () => syncAssetProxyServers(serverRegistry.servers);
    sync();
    const stopResync = installAssetProxyResyncHandler(() => serverRegistry.servers);
    navigator.serviceWorker.addEventListener('controllerchange', sync);
    return () => {
      navigator.serviceWorker.removeEventListener('controllerchange', sync);
      stopResync();
    };
  });

  // Route push-notification clicks via SvelteKit's client-side navigation
  // instead of letting the SW do a full document navigation. Same-URL
  // clicks become a no-op; cross-URL clicks just update the route.
  $effect(() =>
    onNotificationClick((url) => {
      try {
        const target = new URL(url);
        if (target.origin !== window.location.origin) return;
        // eslint-disable-next-line svelte/no-navigation-without-resolve -- URL comes from same-origin service worker notification data
        return goto(target.pathname + target.search + target.hash);
      } catch {
        // Ignore malformed URLs from the SW.
      }
    })
  );

  $effect(() => sidebarNav.initViewportTracking());
  afterNavigate(() => {
    if (sidebarNav.isMobile) sidebarNav.close();
  });

  const getFullTitle = usePageTitle();
  const fullTitle = $derived(getFullTitle());
</script>

<GlobalKeyboardShortcuts />
<IdleTracker />
<UpdateNotifier />

<svelte:head>
  <title>{fullTitle}</title>
</svelte:head>

<ConnectionProvider>
  {@render frame()}
</ConnectionProvider>

{#snippet frame()}
  {@const progress = sidebarNav.isMobile ? sidebarNav.progress : 1}
  {@const dragging = sidebarNav.dragOffset !== null}
  {@const tx = (progress - 1) * SIDEBAR_PANEL_WIDTH_PX}
  <div
    class="flex h-full w-full flex-col overscroll-y-contain bg-surface-100 pt-[env(safe-area-inset-top,0px)] md:p-3 md:pt-0"
  >
    <ConnectionIndicator />

    <AppHeader />

    <Frame class="relative flex-col">
      {#if sidebarNav.isMobile}
        <!--
          Edge gesture zone (swipe-to-open). `touch-action: none` is essential:
          without it, Chrome / iOS Safari fire pointercancel ~8px into a
          horizontal drag (text-selection / back-navigation gesture detection).
          Hidden when sidebar is open (the backdrop takes over).
        -->
        {#if !sidebarNav.isOpen || dragging}
          <div
            use:sidebarSwipe
            class="fixed top-11 bottom-0 left-0 z-40 w-6 touch-none md:hidden"
            aria-hidden="true"
          ></div>
        {/if}

        {#if progress > 0}
          <button
            type="button"
            use:sidebarSwipe
            class={[
              'fixed inset-0 top-11 z-40 touch-none bg-black/50 md:hidden',
              !dragging && 'transition-opacity duration-200'
            ]}
            style="opacity: {progress}"
            onclick={() => sidebarNav.close()}
            aria-label={m['common.close_sidebar']()}
          ></button>
        {/if}
      {/if}

      <div class="flex min-h-0 flex-1 flex-row">
        <div
          use:sidebarSwipe
          class={[
            'z-50 min-h-0 flex-col self-stretch bg-background',
            'max-md:fixed max-md:top-11 max-md:bottom-0 max-md:left-0 max-md:w-17 max-md:touch-pan-y',
            // Mobile: always rendered so we can animate transform.
            // Desktop: hide entirely when closed (no overlay; layout reflows).
            sidebarNav.isMobile ? 'flex' : sidebarNav.isOpen ? 'flex' : 'hidden',
            // Mobile-only: hide via `visibility: hidden` (with transition-delay
            // applied via the `sidebar-mobile-anim` class below) when fully
            // closed, so Playwright / accessibility tooling correctly see the
            // sidebar as not-visible while the slide-in animation still works.
            sidebarNav.isMobile && progress === 0 && !dragging && 'max-md:invisible',
            !dragging && 'sidebar-mobile-anim'
          ]}
          style:transform={sidebarNav.isMobile ? `translateX(${tx}px)` : undefined}
        >
          <ServerGutter />
        </div>

        {@render children?.()}
      </div>
    </Frame>
  </div>
{/snippet}

{#if page.state.modal}
  {#await loadModalContainer() then { default: ModalContainer }}
    <ModalContainer />
  {/await}
{/if}

<ToastContainer />

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
      transition:
        transform 200ms ease-out,
        visibility 0s linear 200ms;
    }
    :global(.sidebar-mobile-anim:not(.invisible)) {
      transition:
        transform 200ms ease-out,
        visibility 0s linear 0s;
    }
  }
</style>
