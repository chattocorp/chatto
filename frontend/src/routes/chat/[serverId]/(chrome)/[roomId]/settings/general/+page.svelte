<script lang="ts">
  import { page } from '$app/state';
  import { getActiveServerSpaceId } from '$lib/state/activeServer.svelte';
  import RoomSettings from '$lib/RoomSettings.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';

  const spaceId = $derived(getActiveServerSpaceId()());
  const roomId = $derived(page.params.roomId!);
</script>

<PageTitle title="General | Room Settings" />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader title="General" subtitle="Room name and description" showMobileNav />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    <RoomSettings {roomId} />
  </div>
</div>
