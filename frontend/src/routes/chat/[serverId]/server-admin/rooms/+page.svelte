<script lang="ts">
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import AdminRoomLayoutEditor from './AdminRoomLayoutEditor.svelte';

  const activeServerId = $derived(getActiveServer());
  const serverSegment = $derived(serverIdToSegment(activeServerId));
  const layout = $derived(serverRegistry.getStore(activeServerId).adminRoomLayout);
</script>

<PageTitle title="Rooms | Space Admin" />

<AdminRoomLayoutEditor {layout} {serverSegment} />
