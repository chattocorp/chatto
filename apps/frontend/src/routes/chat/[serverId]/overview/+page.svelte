<script lang="ts">
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import { m } from '$lib/i18n/messages';
  import RoomDirectory from '$lib/RoomDirectory.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';

  // Re-derives reactively when the URL `[serverId]` changes. Directory rows
  // and membership are selected directly from that server's projection.
  const serverScope = useServerScope();

  const stores = $derived(serverScope.store);
  const directory = $derived(stores.roomDirectory);
  const serverSegment = $derived(serverIdToSegment(serverScope.serverId));
</script>

<PageTitle title={m('chat.overview.title')} />

<div class="pane-page">
  <PaneHeader title={m('chat.overview.title')} showMobileNav />

  <div class="flex-1 overflow-auto">
    <div class="mx-auto flex max-w-6xl flex-col gap-8 p-6">
      <section class="flex flex-col gap-3">
        <h2 class="text-lg font-semibold">{m('common.rooms')}</h2>
        <RoomDirectory {directory} {serverSegment} />
      </section>
    </div>
  </div>
</div>
