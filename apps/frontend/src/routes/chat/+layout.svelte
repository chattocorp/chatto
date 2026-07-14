<script lang="ts">
  import { fullscreenVideo } from '$lib/state/globals.svelte';
  import { createPresenceCache } from '$lib/state/presenceCache.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { UserSettingsState, setUserSettings } from '$lib/state/userSettings.svelte';
  import { createUserProfileCache } from '$lib/state/userProfiles.svelte';

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

  // Remote servers can be authenticated while the origin remains anonymous,
  // so chat-wide caches must exist independently of the origin auth wrapper.
  const profileCache = createUserProfileCache();
  const presenceCache = createPresenceCache();
  const userSettings = new UserSettingsState();
  setUserSettings(userSettings);
</script>

{#if data.user && serverRegistry.originServer}
  {#key data.user.id}
    {#await loadAuthenticatedRoot() then { default: AuthenticatedRoot }}
      <AuthenticatedRoot user={data.user} {userSettings} {profileCache} {presenceCache}>
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
