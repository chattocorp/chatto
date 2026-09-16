<!--
@component

Test-only wrapper for `CurrentUserBar`. Creates the presence-cache context
before the bar mounts so specs can exercise first-login presence fallbacks.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';

  import { createPresenceCache } from '$lib/state/presenceCache.svelte';
  import { provideAppUiState, type AppUiState } from '$lib/state/appUi.svelte';
  import CurrentUserBar from './CurrentUserBar.svelte';

  let { cachedPresence = PresenceStatus.ONLINE, onReady }: {
    cachedPresence?: PresenceStatus;
    onReady?: (state: AppUiState) => void;
  } = $props();

  const presenceCache = createPresenceCache();
  const appUi = provideAppUiState();
  appUi.setActiveRoomScope('origin', 'room-1');
  onMount(() => onReady?.(appUi));
  // svelte-ignore state_referenced_locally
  presenceCache.update({ serverId: 'origin', userId: 'user-1' }, cachedPresence);
</script>

<CurrentUserBar />
