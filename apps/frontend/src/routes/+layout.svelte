<script lang="ts">
  import { page } from '$app/state';
  import { setAuthServerInfo } from '$lib/components/authServerInfo';
  import ConnectionProvider from '$lib/components/ConnectionProvider.svelte';
  import GlobalKeyboardShortcuts from '$lib/components/GlobalKeyboardShortcuts.svelte';
  import IdleTracker from '$lib/components/IdleTracker.svelte';
  import UpdateNotifier from '$lib/components/UpdateNotifier.svelte';
  import { usePageTitle } from '$lib/hooks';
  import { provideAppUiState } from '$lib/state/appUi.svelte';
  import { useServerRegistry } from '$lib/state/server/useServerRegistry.svelte';
  import { ToastContainer } from '$lib/ui/toast';
  import RootLayoutEffects from './RootLayoutEffects.svelte';
  import RootLayoutFrame from './RootLayoutFrame.svelte';
  import '../app.css';

  let { data, children } = $props();
  let modalContainerModule: Promise<typeof import('./chat/ModalContainer.svelte')> | null = null;

  function loadModalContainer() {
    modalContainerModule ??= import('./chat/ModalContainer.svelte');
    return modalContainerModule;
  }

  setAuthServerInfo(() => data.serverInfo);
  const appUi = provideAppUiState();
  useServerRegistry(() => data.user);

  const getFullTitle = usePageTitle();
  const fullTitle = $derived(getFullTitle());
</script>

<RootLayoutEffects {appUi} />
<GlobalKeyboardShortcuts />
<IdleTracker />
<UpdateNotifier />

<svelte:head>
  <title>{fullTitle}</title>
</svelte:head>

<ConnectionProvider>
  <RootLayoutFrame>
    {@render children?.()}
  </RootLayoutFrame>
</ConnectionProvider>

{#if page.state.modal}
  {#await loadModalContainer() then { default: ModalContainer }}
    <ModalContainer />
  {/await}
{/if}

<ToastContainer />
