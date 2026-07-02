<script lang="ts">
  import { useActiveServerScope } from '$lib/state/server/activeServerScope.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import AdminRoomLayoutEditor from './AdminRoomLayoutEditor.svelte';
  import * as m from '$lib/i18n/messages';

  const server = useActiveServerScope();
  const serverSegment = $derived(server.segment);
  const stores = $derived(server.store);
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
