<script lang="ts">
  import { serverIdToSegment } from '$lib/navigation';
  import { serverUi } from '$lib/state/server/serverUi';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { PageTitle } from '$lib/ui';
  import AdminRoomLayoutEditor from './AdminRoomLayoutEditor.svelte';
  import { m } from '$lib/i18n/messages';

  const serverScope = useServerScope();
  const activeServerId = serverScope.serverId;
  const serverSegment = $derived(serverIdToSegment(activeServerId));
  const stores = serverScope.store;
  const layout = $derived(serverUi(stores).adminRoomLayout);

  // The effect owns an external realtime subscription for this mounted route.
  $effect(() => serverUi(stores).activateAdminRoomLayout());
</script>

<PageTitle
  title={m('admin.common.server_admin_page_title', { title: m('admin.rooms_admin.title') })}
/>

<AdminRoomLayoutEditor {layout} {serverSegment} />
