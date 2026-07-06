<script lang="ts">
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import AdminRoomLayoutEditor from './AdminRoomLayoutEditor.svelte';
  import * as m from '$lib/i18n/messages';

  const activeServerId = $derived(getActiveServer());
  const serverSegment = $derived(serverIdToSegment(activeServerId));
  const stores = $derived(serverRegistry.getStore(activeServerId));
  const layout = $derived(stores.adminRoomLayout);

  $effect(() => stores.activateAdminRoomLayout());

  function refreshServerRoomState() {
    void stores.rooms.refresh();
    void stores.roomDirectory.refresh();
  }
</script>

<PageTitle
  title={m['admin.common.server_admin_page_title']({ title: m['admin.rooms_admin.title']() })}
/>

<AdminRoomLayoutEditor {layout} {serverSegment} onroomcreated={refreshServerRoomState} />
