<script lang="ts">
  import { fullscreenVideo } from '$lib/state/globals.svelte';
  import { createPresenceCache } from '$lib/state/presenceCache.svelte';
  import { createUserProfileCache } from '$lib/state/userProfiles.svelte';
  import ChatRoot from './ChatRoot.svelte';

  let { data, children } = $props();
  let fullscreenVideoOverlayModule: Promise<
    typeof import('$lib/components/chat/FullscreenVideoOverlay.svelte')
  > | null = null;

  function loadFullscreenVideoOverlay() {
    fullscreenVideoOverlayModule ??= import('$lib/components/chat/FullscreenVideoOverlay.svelte');
    return fullscreenVideoOverlayModule;
  }

  const profileCache = createUserProfileCache();
  const presenceCache = createPresenceCache();
</script>

<!-- Origin login/logout changes replace the origin-scoped effects while the
     chat-wide coordinator remains available to remote-only sessions. -->
{#key data.user?.id}
  <ChatRoot user={data.user} {profileCache} {presenceCache}>
    {@render children?.()}
  </ChatRoot>
{/key}

{#if fullscreenVideo.isOpen}
  {#await loadFullscreenVideoOverlay() then { default: FullscreenVideoOverlay }}
    <FullscreenVideoOverlay />
  {/await}
{/if}
