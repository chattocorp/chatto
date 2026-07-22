<script lang="ts">
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import * as m from '$lib/i18n/messages';
  import RoomDirectory from '$lib/RoomDirectory.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';

  // Active-server stores. Both substores self-manage refresh and
  // live-event ingestion from inside `ServerStateStore`, so this page
  // just reads them. Re-derives reactively when the URL `[serverId]`
  // changes.
  const stores = $derived(serverRegistry.getStore(getActiveServer()));
  const directory = $derived(stores.roomDirectory);
  const roomsStore = $derived(stores.rooms);
  const serverSegment = $derived(serverIdToSegment(getActiveServer()));
</script>

<PageTitle title={m['chat.overview.title']()} />

<div class="pane-page">
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
