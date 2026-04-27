<script lang="ts">
  import { afterNavigate } from '$app/navigation';
  import { page } from '$app/state';
  import SpaceList from '$lib/SpaceList.svelte';
  import { CurrentUserState, setCurrentUser } from '$lib/auth/currentUser.svelte';
  import ConnectionIndicator from '$lib/components/ConnectionIndicator.svelte';
  import ConnectionProvider from '$lib/components/ConnectionProvider.svelte';
  import GlobalKeyboardShortcuts from '$lib/components/GlobalKeyboardShortcuts.svelte';
  import NotificationSync from '$lib/components/NotificationSync.svelte';
  import UpdateNotifier from '$lib/components/UpdateNotifier.svelte';
  import FullscreenVideoOverlay from '$lib/components/chat/FullscreenVideoOverlay.svelte';
  import { usePageTitle, usePinchZoomPrevention, useVisualViewport } from '$lib/hooks';
  import { sidebarNav } from '$lib/state/globals.svelte';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import { useInstanceRegistry } from '$lib/state/instance/useInstanceRegistry.svelte';
  import { graphqlClientManager } from '$lib/state/instance/graphqlClient.svelte';
  import { instanceEventBusManager } from '$lib/state/instance/eventBus.svelte';
  import { createInstancePermissions } from '$lib/state/instance/permissions.svelte';
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
  useInstanceRegistry(() => data.user);
  useVisualViewport();
  usePinchZoomPrevention();

  // Contexts
  const updateInstancePermissions = createInstancePermissions();

  // Provide a CurrentUserState via context so components that render outside
  // the chat tree (SpaceList, /setup, etc.) can still call getCurrentUser().
  // Components that need to *write* to the user state (AuthenticatedChatProvider)
  // look up the registry directly — see the comment there for why.
  const originId = instanceRegistry.originInstance?.id;
  const currentUserState = originId
    ? instanceRegistry.getStore(originId).currentUser
    : new CurrentUserState(graphqlClientManager.originClient.client, true);
  currentUserState.loading = false;
  setCurrentUser(currentUserState);

  const userSettings = new UserSettingsState();
  setUserSettings(userSettings);

  const profileCache = createUserProfileCache();
  const presenceCache = createPresenceCache();

  // Start event buses for token-authenticated (remote) instances.
  // startBus is idempotent; cleanup is handled by removeInstance.
  $effect(() => {
    for (const instance of instanceRegistry.instances) {
      if (instance.token) {
        instanceEventBusManager.startBus(
          instance.id,
          graphqlClientManager.getClient(instance.id).client
        );
      }
    }
  });

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
    {#if data.user && instanceRegistry.originInstance}
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
  <div
    class="flex h-full w-full flex-col overscroll-y-contain bg-surface-100 pt-[env(safe-area-inset-top,0px)] md:p-3 md:pt-0"
  >
    <ConnectionIndicator />

    <AppHeader />

    <Frame class="relative flex-col">
      {#if sidebarNav.isOpen}
        <button
          type="button"
          class="fixed inset-0 top-11 z-40 bg-black/50 md:hidden"
          onclick={() => sidebarNav.close()}
          aria-label="Close sidebar"
        ></button>
      {/if}

      <div class="flex min-h-0 flex-1 flex-row">
        <div
          class={[
            'z-50 min-h-0 flex-col self-stretch bg-background',
            'max-md:fixed max-md:top-11 max-md:bottom-0 max-md:left-0',
            sidebarNav.isOpen ? 'flex' : 'hidden'
          ]}
        >
          <SpaceList
            activeSpaceId={page.params.spaceId}
            onPermissionsLoaded={updateInstancePermissions}
          />
        </div>

        {@render children?.()}
      </div>
    </Frame>
  </div>

  <ModalContainer />
  <FullscreenVideoOverlay />
{/snippet}

<ToastContainer />
