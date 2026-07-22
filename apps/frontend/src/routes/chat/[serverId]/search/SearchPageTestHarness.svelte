<script lang="ts">
  import { createPresenceCache } from '$lib/state/presenceCache.svelte';
  import { createUserProfileCache } from '$lib/state/userProfiles.svelte';
  import SearchPage from './+page.svelte';

  createPresenceCache();
  createUserProfileCache();
</script>

<SearchPage />
