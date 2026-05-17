<script lang="ts">
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
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
  const serverInfo = $derived(stores.serverInfo);

  const serverName = $derived(serverInfo.name);
  const serverDescription = $derived(serverInfo.description);
  const joinedCount = $derived(roomsStore.rooms.length);
</script>

<PageTitle title={`${serverName} | Overview`} />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader title="Overview" showMobileNav />

  <div class="flex-1 overflow-auto">
    <div class="mx-auto flex max-w-6xl flex-col gap-8 p-6">
      <!-- Welcome / hero -->
      <section
        class="rounded-2xl border border-border bg-gradient-to-br from-primary/10 via-surface to-surface p-6 shadow-sm"
      >
        <div class="flex flex-col gap-2">
          <h1 class="text-2xl font-bold text-text">Welcome to {serverName}</h1>
          {#if serverDescription}
            <p class="max-w-2xl text-sm text-muted">{serverDescription}</p>
          {:else}
            <p class="max-w-2xl text-sm text-muted">
              Find rooms to join below. You'll only see what you've explicitly joined in your
              sidebar — pick the ones that look interesting.
            </p>
          {/if}
          <div class="mt-2 flex flex-wrap gap-2 text-xs text-muted">
            <span class="rounded-full border border-border bg-surface px-2 py-1">
              <span class="iconify uil--check-circle inline-block align-[-2px]"></span>
              You're in {joinedCount} {joinedCount === 1 ? 'room' : 'rooms'}
            </span>
          </div>
        </div>
      </section>

      <!-- Room directory (cards) -->
      <section class="flex flex-col gap-3">
        <h2 class="text-lg font-semibold">Rooms</h2>
        <RoomDirectory {directory} {roomsStore} />
      </section>
    </div>
  </div>
</div>
