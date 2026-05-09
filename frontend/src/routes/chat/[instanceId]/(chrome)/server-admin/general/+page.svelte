<script lang="ts">
  import { page } from '$app/state';
  import { getActiveInstanceSpaceId } from '$lib/state/activeInstance.svelte';
  import SpaceSettings from '$lib/SpaceSettings.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';

  const spaceId = $derived(getActiveInstanceSpaceId()());
</script>

<PageTitle title="General | Space Admin" />

<PaneHeader title="General" subtitle="Space name, description, and branding" showMobileNav />

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  <SpaceSettings {spaceId} />
</div>
