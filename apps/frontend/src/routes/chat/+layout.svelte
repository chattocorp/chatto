<script lang="ts">
  import { fullscreenVideo } from '$lib/state/globals.svelte';
  import { createPresenceCache } from '$lib/state/presenceCache.svelte';
  import { provideUserProfiles } from '$lib/state/userProfiles.svelte';
  import ChatRoot from './ChatRoot.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';

  let { children } = $props();
  let fullscreenVideoOverlayModule: Promise<
    typeof import('$lib/components/chat/FullscreenVideoOverlay.svelte')
  > | null = null;

  function loadFullscreenVideoOverlay() {
    fullscreenVideoOverlayModule ??= import('$lib/components/chat/FullscreenVideoOverlay.svelte');
    return fullscreenVideoOverlayModule;
  }

  provideUserProfiles(() => {
    const id = serverRegistry.originServer?.id;
    return id ? serverRegistry.tryGetStore(id)?.projection.users : undefined;
  });
  const presenceCache = createPresenceCache();
</script>

<!-- Keep the saved viewer's tree through verification and route loads. Only
     an actual identity change resets origin-scoped effects and local UI state. -->
{#key serverRegistry.originServer?.userId}
  <ChatRoot {presenceCache}>
    {@render children?.()}
  </ChatRoot>
{/key}

{#if fullscreenVideo.isOpen}
  {#await loadFullscreenVideoOverlay() then { default: FullscreenVideoOverlay }}
    <FullscreenVideoOverlay />
  {/await}
{/if}
