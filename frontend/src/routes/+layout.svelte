<script lang="ts">
  import { afterNavigate, goto } from '$app/navigation';
  import { page } from '$app/state';
  import { onNotificationClick } from '$lib/notifications/pushNotifications';
  import SpaceList from '$lib/SpaceList.svelte';
  import ConnectionIndicator from '$lib/components/ConnectionIndicator.svelte';
  import ConnectionProvider from '$lib/components/ConnectionProvider.svelte';
  import GlobalKeyboardShortcuts from '$lib/components/GlobalKeyboardShortcuts.svelte';
  import NotificationSync from '$lib/components/NotificationSync.svelte';
  import UpdateNotifier from '$lib/components/UpdateNotifier.svelte';
  import FullscreenVideoOverlay from '$lib/components/chat/FullscreenVideoOverlay.svelte';
  import { usePageTitle, usePinchZoomPrevention, useVisualViewport } from '$lib/hooks';
  import { SIDEBAR_PANEL_WIDTH_PX, sidebarSwipe } from '$lib/hooks/useSidebarSwipe.svelte';
  import { sidebarNav } from '$lib/state/globals.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { useServerRegistry } from '$lib/state/server/useServerRegistry.svelte';
  import { graphqlClientManager } from '$lib/state/server/graphqlClient.svelte';
  import { eventBusManager } from '$lib/state/server/eventBus.svelte';
  import { createPresenceCache } from '$lib/state/presenceCache.svelte';
  import { createUserProfileCache } from '$lib/state/userProfiles.svelte';
  import { UserSettingsState, setUserSettings } from '$lib/state/userSettings.svelte';
  import { AppHeader, Frame } from '$lib/ui';
  import { ToastContainer } from '$lib/ui/toast';
  import '../app.css';
  import AuthenticatedChatProvider from './chat/AuthenticatedChatProvider.svelte';
  import ModalContainer from './chat/ModalContainer.svelte';

  let { data, children } = $props();

  // Global initialization
  useServerRegistry(() => data.user);
  useVisualViewport();
  usePinchZoomPrevention();

  // Mark the origin store's currentUser as not-loading at app init.
  // SvelteKit's load function already resolved auth state by the time this
  // script runs — any further changes flow through `currentUser.user`. The
  // registry is the single source of truth for `CurrentUserState`;
  // consumers read it via `serverRegistry.getStore(serverId).currentUser`.
  const originId = serverRegistry.originServer?.id;
  if (originId) {
    serverRegistry.getStore(originId).currentUser.loading = false;
  }

  const userSettings = new UserSettingsState();
  setUserSettings(userSettings);

  const profileCache = createUserProfileCache();
  const presenceCache = createPresenceCache();

  // Start event buses for every authenticated instance (origin or remote).
  // startBus is idempotent; cleanup is handled by removeInstance.
  //
  // We do this synchronously during script init AND in a $effect, because
  // child route layouts (e.g. /chat/[serverId]/+layout.svelte) call
  // `provideEventBus(serverId)` at their own script init time —
  // which runs after THIS script but before any $effect on this component.
  // Without the sync pass, the bus isn't available when those children try
  // to expose it via Svelte context, and any descendant calling
  // `useEvent` ends up subscribing to nothing (real-time updates
  // for cross-instance unread tracking get silently dropped).
  for (const instance of serverRegistry.instances) {
    const store = serverRegistry.tryGetStore(instance.id);
    if (store?.isAuthenticated) {
      eventBusManager.startBus(
        instance.id,
        graphqlClientManager.getClient(instance.id).client
      );
    }
  }
  $effect(() => {
    for (const instance of serverRegistry.instances) {
      const store = serverRegistry.tryGetStore(instance.id);
      if (store?.isAuthenticated) {
        // startBus is idempotent — no-op if already started above.
        eventBusManager.startBus(
          instance.id,
          graphqlClientManager.getClient(instance.id).client
        );
      }
    }
  });

  // Route push-notification clicks via SvelteKit's client-side navigation
  // instead of letting the SW do a full document navigation. Same-URL
  // clicks become a no-op; cross-URL clicks just update the route.
  $effect(() =>
    onNotificationClick((url) => {
      try {
        const target = new URL(url);
        if (target.origin !== window.location.origin) return;
        void goto(target.pathname + target.search + target.hash);
      } catch {
        // Ignore malformed URLs from the SW.
      }
    })
  );

  // Sidebar
  $effect(() => sidebarNav.initViewportTracking());
  afterNavigate(() => {
    if (sidebarNav.isMobile) sidebarNav.close();
  });

  // Page title
  const getFullTitle = usePageTitle();
  const fullTitle = $derived(getFullTitle());

  // Route detection
  const isSetupRoute = $derived(page.url.pathname.startsWith('/setup'));
</script>

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

<GlobalKeyboardShortcuts />
<UpdateNotifier />
<NotificationSync />

<svelte:head>
  <title>{fullTitle}</title>
</svelte:head>

{#if isSetupRoute}
  <div class="flex h-full flex-col overscroll-y-contain pt-[env(safe-area-inset-top,0px)]">
    {@render children?.()}
  </div>
{:else}
  <ConnectionProvider>
    {#if data.user && serverRegistry.originServer}
      <AuthenticatedChatProvider
        user={data.user}
        {userSettings}
        {profileCache}
        {presenceCache}
      >
        {@render frame()}
      </AuthenticatedChatProvider>
    {:else}
      {@render frame()}
    {/if}
  </ConnectionProvider>
{/if}

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
            aria-label="Close sidebar"
          ></button>
        {/if}
      {/if}

      <div class="flex min-h-0 flex-1 flex-row">
        <div
          use:sidebarSwipe
          class={[
            'z-50 min-h-0 flex-col self-stretch bg-background',
            'max-md:fixed max-md:top-11 max-md:bottom-0 max-md:left-0 max-md:touch-pan-y',
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
          <SpaceList />
        </div>

        {@render children?.()}
      </div>
    </Frame>
  </div>

  <ModalContainer />
  <FullscreenVideoOverlay />
{/snippet}

<ToastContainer />
