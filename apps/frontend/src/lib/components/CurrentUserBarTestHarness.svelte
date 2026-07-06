<!--
@component

Test-only wrapper for `CurrentUserBar`. Creates the presence-cache context
before the bar mounts so specs can exercise first-login presence fallbacks.
-->
<script lang="ts">
  import { PresenceStatus } from '$lib/render/types';
  import { createPresenceCache } from '$lib/state/presenceCache.svelte';
  import CurrentUserBar from './CurrentUserBar.svelte';

  const presenceCache = createPresenceCache();
  presenceCache.update('user-1', PresenceStatus.Online);
</script>

<CurrentUserBar />
