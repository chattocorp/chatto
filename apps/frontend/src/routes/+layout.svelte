<script lang="ts">
  import { initialPageReveal } from '$lib/attachments/initialPageReveal';
  import { afterNavigate, beforeNavigate, goto } from '$app/navigation';
  import { navigationVisits } from '$lib/navigation/mutationCompletion';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { onNotificationClick } from '$lib/notifications/pushNotifications';
  import { prepareUiForNotificationPath } from '$lib/notifications/notificationNavigationUi';
  import { setAuthServerInfo } from '$lib/components/authServerInfo';
  import GlobalKeyboardShortcuts from '$lib/components/GlobalKeyboardShortcuts.svelte';
  import MobileSidebarChrome from '$lib/components/MobileSidebarChrome.svelte';
  import NotificationSync from '$lib/components/NotificationSync.svelte';
  import UpdateNotifier from '$lib/components/UpdateNotifier.svelte';
  import { usePageTitle } from '$lib/hooks/usePageTitle.svelte';
  import { usePinchZoomPrevention } from '$lib/hooks/usePinchZoomPrevention.svelte';
  import { sidebarSwipe } from '$lib/hooks/useSidebarSwipe.svelte';
  import { useVisualViewport } from '$lib/hooks/useVisualViewport.svelte';
  import { chatRoomIdFromRoute } from '$lib/navigation/chatRoomRoute';
  import { isSafeInternalPath } from '$lib/navigation/safeInternalPath';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { sidebarNav } from '$lib/state/globals.svelte';
  import { provideAppUiState } from '$lib/state/appUi.svelte';
  import ServerRuntimeCoordinator from '$lib/state/server/ServerRuntimeCoordinator.svelte';
  import { ToastContainer } from '$lib/ui/toast';
  import AppHeader from '$lib/ui/AppHeader.svelte';
  import Frame from '$lib/ui/Frame.svelte';
  import '../app.css';

  let { data, children } = $props();
  let modalContainerModule: Promise<typeof import('./chat/ModalContainer.svelte')> | null = null;

  function loadModalContainer() {
    modalContainerModule ??= import('./chat/ModalContainer.svelte');
    return modalContainerModule;
  }

  setAuthServerInfo(() => data.serverInfo);
  const appUi = provideAppUiState();
  beforeNavigate(() => navigationVisits.leave());
  useVisualViewport();
  usePinchZoomPrevention();

  const activeServerId = $derived(getActiveServer());
  const activeRoomId = $derived(chatRoomIdFromRoute(page.route.id, page.params.roomId));

  // OAuth windows keep their page content and branding without app navigation.
  const standaloneOAuth = $derived.by(() => {
    if (page.route.id === '/oauth/consent') return true;
    if (page.route.id === '/servers/callback') {
      return ['popup', 'provider'].includes(page.url.searchParams.get('mode') ?? '');
    }
    if (page.route.id !== '/login') return false;
    const redirect = page.url.searchParams.get('redirect');
    if (!isSafeInternalPath(redirect)) return false;
    const target = new URL(redirect, page.url).pathname;
    return target === '/oauth/authorize' || target === '/oauth/consent';
  });

  $effect(() => {
    if (typeof activeRoomId === 'string' && activeRoomId) {
      appUi.setActiveRoomScope(activeServerId, activeRoomId);
      return;
    }
    appUi.setActiveServer(activeServerId);
  });

  // Route push-notification clicks via SvelteKit's client-side navigation
  // instead of letting the SW do a full document navigation. Same-URL
  // clicks become a no-op; cross-URL clicks just update the route.
  $effect(() =>
    onNotificationClick((url) => {
      try {
        const target = new URL(url);
        if (target.origin !== window.location.origin) return;
        prepareUiForNotificationPath(appUi, target.pathname);
        return goto(resolve((target.pathname + target.search + target.hash) as '/'));
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

{#if !standaloneOAuth}
  <GlobalKeyboardShortcuts />
{/if}
{#key data.user?.id}
  <ServerRuntimeCoordinator user={data.user} />
{/key}
<NotificationSync />
<UpdateNotifier />

<svelte:head>
  <title>{fullTitle}</title>
</svelte:head>

{#if standaloneOAuth}
  <div
    {@attach initialPageReveal}
    class="flex h-full w-full flex-col overflow-hidden bg-background pt-[env(safe-area-inset-top,0px)] pb-[env(safe-area-inset-bottom,0px)]"
  >
    {@render children?.()}
  </div>
{:else}
  <div
    {@attach initialPageReveal}
    use:sidebarSwipe
    class="flex h-full w-full flex-col overscroll-y-contain bg-surface desktop-presentation:app-frame-shell desktop-presentation:px-3 desktop-presentation:pb-3 pt-[env(safe-area-inset-top,0px)]"
  >
    <AppHeader />

    <Frame class="relative flex-col">
      <MobileSidebarChrome>
        {@render children?.()}
      </MobileSidebarChrome>
    </Frame>
  </div>
{/if}

<!-- Give WebKit a fixed, opaque bottom edge to extend behind Safari's toolbar.
     The native iOS shell hides it because it paints its own keyboard backdrop.
     This surface must not intercept input or change the app's available height. -->
<div
  data-safari-bottom-edge
  aria-hidden="true"
  class={[
    'pointer-events-none fixed inset-x-0 bottom-0 hidden h-px mobile-presentation:block',
    standaloneOAuth ? 'bg-background' : 'bg-surface'
  ]}
></div>

{#if page.state.modal}
  {#await loadModalContainer() then { default: ModalContainer }}
    <ModalContainer />
  {/await}
{/if}

<ToastContainer />
