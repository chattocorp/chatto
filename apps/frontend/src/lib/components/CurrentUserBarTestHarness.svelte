<!--
@component

Test-only wrapper for `CurrentUserBar`. Creates the presence-cache context
before the bar mounts so specs can exercise first-login presence fallbacks.
-->
<script lang="ts">
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';

  import { createPresenceCache } from '$lib/state/presenceCache.svelte';
  import { provideAppUiState } from '$lib/state/appUi.svelte';
  import CurrentUserBar from './CurrentUserBar.svelte';

  let { cachedPresence = PresenceStatus.ONLINE }: { cachedPresence?: PresenceStatus } = $props();

  const presenceCache = createPresenceCache();
  const appUi = provideAppUiState();
  appUi.setActiveRoomScope('origin', 'room-1');
  // svelte-ignore state_referenced_locally
  presenceCache.update({ serverId: 'origin', userId: 'user-1' }, cachedPresence);
</script>

<CurrentUserBar />
