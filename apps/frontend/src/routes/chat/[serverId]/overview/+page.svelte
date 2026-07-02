<script lang="ts">
  import { useActiveServerScope } from '$lib/state/server/activeServerScope.svelte';
  import * as m from '$lib/i18n/messages';
  import RoomDirectory from '$lib/RoomDirectory.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';

  // Active-server stores. Both substores self-manage refresh and
  // live-event ingestion from inside `ServerStateStore`, so this page
  // just reads them. Re-derives reactively when the URL `[serverId]`
  // changes.
  const server = useActiveServerScope();
  const directory = $derived(server.store.roomDirectory);
  const roomsStore = $derived(server.rooms);
  const serverSegment = $derived(server.segment);
</script>

<PageTitle title={m['chat.overview.title']()} />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader title={m['chat.overview.title']()} showMobileNav />

  <div class="flex-1 overflow-auto">
    <div class="mx-auto flex max-w-6xl flex-col gap-8 p-6">
      <section class="flex flex-col gap-3">
        <h2 class="text-lg font-semibold">{m['common.rooms']()}</h2>
        <RoomDirectory {directory} {roomsStore} {serverSegment} />
      </section>
    </div>
  </div>
</div>
