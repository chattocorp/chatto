<script lang="ts">
  import { fullscreenVideo } from '$lib/state/globals.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { UserSettingsState, setUserSettings } from '$lib/state/userSettings.svelte';

  let { data, children } = $props();
  let authenticatedRootModule: Promise<typeof import('./AuthenticatedRoot.svelte')> | null = null;
  let fullscreenVideoOverlayModule: Promise<
    typeof import('$lib/components/chat/FullscreenVideoOverlay.svelte')
  > | null = null;

  function loadAuthenticatedRoot() {
    authenticatedRootModule ??= import('./AuthenticatedRoot.svelte');
    return authenticatedRootModule;
  }

  function loadFullscreenVideoOverlay() {
    fullscreenVideoOverlayModule ??= import('$lib/components/chat/FullscreenVideoOverlay.svelte');
    return fullscreenVideoOverlayModule;
  }

  const userSettings = new UserSettingsState();
  setUserSettings(userSettings);
</script>

{#if data.user && serverRegistry.originServer}
  {#key data.user.id}
    {#await loadAuthenticatedRoot() then { default: AuthenticatedRoot }}
      <AuthenticatedRoot user={data.user} {userSettings}>
        {@render children?.()}
      </AuthenticatedRoot>
    {/await}
  {/key}
{:else}
  {@render children?.()}
{/if}

{#if fullscreenVideo.isOpen}
  {#await loadFullscreenVideoOverlay() then { default: FullscreenVideoOverlay }}
    <FullscreenVideoOverlay />
  {/await}
{/if}
