<script lang="ts">
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import AdminRoomLayoutEditor from './AdminRoomLayoutEditor.svelte';
  import { m } from '$lib/i18n/messages';

  const serverScope = useServerScope();
  const activeServerId = $derived(serverScope.serverId);
  const serverSegment = $derived(serverIdToSegment(activeServerId));
  const stores = $derived(serverScope.store);
  const layout = $derived(stores.adminRoomLayout);

  // The effect owns an external realtime subscription for this mounted route.
  $effect(() => stores.activateAdminRoomLayout());
</script>

<PageTitle
  title={m('admin.common.server_admin_page_title', { title: m('admin.rooms_admin.title') })}
/>

<AdminRoomLayoutEditor {layout} {serverSegment} />
