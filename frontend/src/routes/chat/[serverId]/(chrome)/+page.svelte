<script lang="ts">
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Hint } from '$lib/ui';

  const getServerId = getActiveServer;
  const serverId = $derived(getServerId());
  const serverSegment = $derived(serverIdToSegment(serverId));
  const stores = $derived(serverRegistry.tryGetStore(serverId));
  const serverInfo = $derived(stores?.serverInfo);
  const serverName = $derived(serverInfo?.name ?? 'Server');
</script>

<PageTitle title={`${serverName} | Home`} />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader title="Home" subtitle={serverName} showMobileNav />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    <Hint tone="info">
      Welcome to <strong>{serverName}</strong>. This is a stub — the full home page (welcome
      message, featured rooms / groups, recent activity) is on the way.
    </Hint>

    <section class="flex flex-col gap-2">
      <h2 class="text-lg font-semibold">Get started</h2>
      <p class="text-sm text-muted">
        Membership is explicit: you only see what you've joined. Use Browse Rooms to find rooms
        you want to join.
      </p>
      <div>
        <a
          href={resolve('/chat/[serverId]/(chrome)/rooms', { serverId: serverSegment })}
          class="btn btn-primary inline-flex items-center gap-2"
        >
          <span class="iconify uil--search-alt"></span>
          Browse Rooms
        </a>
      </div>
    </section>
  </div>
</div>
