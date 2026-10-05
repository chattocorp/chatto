<script lang="ts">
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { serverUi } from '$lib/state/server/serverUi';
  import { serverIdToSegment } from '$lib/navigation';
  import { m } from '$lib/i18n/messages';
  import RoomDirectory from '$lib/RoomDirectory.svelte';
  import { PaneContent, PaneHeader, PageTitle } from '$lib/ui';

  // Re-derives reactively when the URL `[serverId]` changes. Directory rows
  // and membership are selected directly from that server's projection.
  const serverScope = useServerScope();

  const stores = serverScope.store;
  const directory = $derived(serverUi(stores).roomDirectory);
  const serverSegment = $derived(serverIdToSegment(serverScope.serverId));
</script>

<PageTitle title={m('chat.overview.title')} />

<div class="pane-page">
  <PaneHeader title={m('chat.overview.title')} />

  <!-- The directory's room groups are titled panels; the masonry uses the wide width. -->
  <PaneContent wide>
    <RoomDirectory {directory} {serverSegment} />
  </PaneContent>
</div>
