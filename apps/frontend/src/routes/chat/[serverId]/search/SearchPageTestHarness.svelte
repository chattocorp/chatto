<script lang="ts">
  import { provideUserProfiles } from '$lib/state/userProfiles.svelte';
  import SearchPage from './+page.svelte';

  provideUserProfiles();
</script>

<SearchPage />
