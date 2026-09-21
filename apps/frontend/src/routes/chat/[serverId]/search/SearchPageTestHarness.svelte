<script lang="ts">
  import { createPresenceCache } from '$lib/state/presenceCache.svelte';
  import { provideUserProfiles } from '$lib/state/userProfiles.svelte';
  import SearchPage from './+page.svelte';

  createPresenceCache();
  provideUserProfiles();
</script>

<SearchPage />
